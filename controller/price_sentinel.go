package controller

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	pricesentinel "github.com/QuantumNous/new-api/pkg/price_sentinel"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/price_sentinel_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
)

// priceSentinelHandler runs the hourly upstream price check.
//
// It only reports. The digest names each model whose price differs and by how
// much, and mails an expiring link to an edit page; changing a price stays the
// operator's call, because whether to follow an upstream move depends on which
// side of the trade that model sits on.
type priceSentinelHandler struct{}

func (priceSentinelHandler) Type() string { return model.SystemTaskTypePriceSentinel }

func (priceSentinelHandler) Enabled() bool {
	return price_sentinel_setting.GetSetting().Enabled
}

func (priceSentinelHandler) Interval() time.Duration {
	return time.Duration(price_sentinel_setting.GetIntervalMinutes()) * time.Minute
}

func (priceSentinelHandler) NewPayload() any { return nil }

func (priceSentinelHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	report, err := runPriceSentinelCheck(ctx)
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, report, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, report, nil)
}

// priceSentinelRunSummary is what a run reports back to the task row.
type priceSentinelRunSummary struct {
	Compared    int    `json:"compared"`
	Changed     int    `json:"changed"`
	EmailSent   bool   `json:"email_sent"`
	EmailTarget string `json:"email_target,omitempty"`
	FetchError  string `json:"fetch_error,omitempty"`
}

// runPriceSentinelCheck performs one check end to end: compare, persist, mail.
//
// A failed fetch still writes a row and still mails — an operator who stops
// receiving the digest cannot tell "no changes" from "the check is broken", so
// the failure is reported rather than swallowed.
func runPriceSentinelCheck(ctx context.Context) (*priceSentinelRunSummary, error) {
	setting := price_sentinel_setting.GetSetting()
	checkedAt := common.GetTimestamp()
	summary := &priceSentinelRunSummary{}

	comparison, compareErr := pricesentinel.RunOnce(ctx)
	report := &model.PriceSentinelReport{
		CheckedAt:   checkedAt,
		UpstreamUrl: strings.TrimSpace(setting.UpstreamPricingUrl),
	}
	if compareErr != nil {
		report.FetchError = compareErr.Error()
		summary.FetchError = compareErr.Error()
	} else {
		encoded, err := common.Marshal(comparison.Changes)
		if err != nil {
			return summary, err
		}
		report.Compared = comparison.Compared
		report.Changed = len(comparison.Changes)
		report.Changes = string(encoded)
		higher, lower := 0, 0
		for _, change := range comparison.Changes {
			if change.Percent > 0 {
				lower++
			} else {
				higher++
			}
		}
		report.HigherPrice = higher
		report.LowerPrice = lower
		summary.Compared = comparison.Compared
		summary.Changed = len(comparison.Changes)
	}

	if err := model.InsertPriceSentinelReport(report); err != nil {
		return summary, err
	}

	target := resolvePriceSentinelRecipient()
	if target == "" {
		common.SysLog("price sentinel: no report recipient configured, skipping mail")
		return summary, nil
	}
	summary.EmailTarget = target

	editUrl := ""
	if compareErr == nil && len(comparison.Changes) > 0 {
		token, err := model.CreatePriceSentinelEditToken(report.Id, setting.EditTokenTTLHours)
		if err != nil {
			common.SysLog(fmt.Sprintf("price sentinel: failed to mint edit token: %s", err.Error()))
		} else {
			editUrl = buildPriceSentinelEditUrl(token)
		}
	}

	var body string
	if compareErr != nil {
		rendered, err := pricesentinel.BuildFailureReport(checkedAt, compareErr.Error())
		if err != nil {
			common.SysLog("price sentinel: failed to render failure report: " + err.Error())
			return summary, nil
		}
		body = rendered
	} else {
		body = pricesentinel.BuildReport(comparison, checkedAt, editUrl)
	}

	subject := pricesentinel.ReportSubject(checkedAt)
	if err := common.SendEmail(subject, target, body); err != nil {
		common.SysLog(fmt.Sprintf("price sentinel: failed to send digest to %s: %s", target, err.Error()))
		return summary, nil
	}
	summary.EmailSent = true
	if err := model.MarkPriceSentinelReportEmailed(report.Id, common.GetTimestamp()); err != nil {
		common.SysLog(fmt.Sprintf("price sentinel: failed to mark report emailed: %s", err.Error()))
	}
	return summary, nil
}

// resolvePriceSentinelRecipient picks where the digest goes: the configured
// address, else the root user's, which is where the upstream-model digest
// already lands.
func resolvePriceSentinelRecipient() string {
	if configured := strings.TrimSpace(price_sentinel_setting.GetSetting().ReportEmail); configured != "" {
		return configured
	}
	email, err := model.GetRootUserEmail()
	if err != nil {
		common.SysLog(fmt.Sprintf("price sentinel: failed to resolve root email: %s", err.Error()))
		return ""
	}
	return strings.TrimSpace(email)
}

// buildPriceSentinelEditUrl points at the console's edit page. The base comes
// from the configured server address so the link works from a mail client.
func buildPriceSentinelEditUrl(token string) string {
	base := strings.TrimRight(strings.TrimSpace(system_setting.ServerAddress), "/")
	if base == "" {
		return ""
	}
	return fmt.Sprintf("%s/price-sentinel?token=%s", base, token)
}

// ------------------- HTTP surface -------------------

// GetPriceSentinelReport returns the latest check plus its change list. Admin
// only: the report names our prices and the upstream's side by side.
func GetPriceSentinelReport(c *gin.Context) {
	report, err := model.GetLatestPriceSentinelReport()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if report == nil {
		common.ApiSuccess(c, gin.H{"report": nil, "changes": []model.PriceChange{}})
		return
	}
	changes := []model.PriceChange{}
	if strings.TrimSpace(report.Changes) != "" {
		if err := common.UnmarshalJsonStr(report.Changes, &changes); err != nil {
			common.SysLog("price sentinel: stored change list is unreadable: " + err.Error())
			changes = []model.PriceChange{}
		}
	}
	common.ApiSuccess(c, gin.H{"report": priceSentinelReportForClient(report), "changes": changes})
}

// RunPriceSentinelNow triggers a check on demand.
func RunPriceSentinelNow(c *gin.Context) {
	summary, err := runPriceSentinelCheck(c.Request.Context())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, summary)
}

// priceSentinelEditRequest carries one price change applied from the edit page.
type priceSentinelEditRequest struct {
	Token     string  `json:"token"`
	ModelName string  `json:"model_name"`
	Field     string  `json:"field"`
	Value     float64 `json:"value"`
}

// ApplyPriceSentinelEdit verifies the mailed token and writes one price.
//
// The token only proves the request came from a link we mailed; it is not an
// admin credential. This route therefore sits behind RootAuth as well, so the
// operator still authenticates before anything is written.
func ApplyPriceSentinelEdit(c *gin.Context) {
	var req priceSentinelEditRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "无效的参数")
		return
	}
	modelName := strings.TrimSpace(req.ModelName)
	if modelName == "" {
		common.ApiErrorMsg(c, "模型名不能为空")
		return
	}
	if req.Value < 0 {
		common.ApiErrorMsg(c, "价格不能为负数")
		return
	}

	record, err := model.VerifyPriceSentinelEditToken(req.Token)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	changes, err := loadPriceSentinelChanges(record.ReportId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !pricesentinel.ChangeIsReported(changes, modelName, req.Field) {
		common.ApiErrorMsg(c, "该模型不在这份报告里")
		return
	}

	if err := pricesentinel.ApplyPriceEdit(modelName, req.Field, req.Value); err != nil {
		common.ApiError(c, err)
		return
	}

	recordManageAudit(c, "price_sentinel.apply", map[string]any{
		"model_name": modelName,
		"field":      req.Field,
		"value":      req.Value,
	})
	common.ApiSuccess(c, gin.H{"model_name": modelName, "field": req.Field, "value": req.Value})
}

func loadPriceSentinelChanges(reportId int) ([]model.PriceChange, error) {
	report, err := model.GetPriceSentinelReport(reportId)
	if err != nil {
		return nil, err
	}
	var changes []model.PriceChange
	if strings.TrimSpace(report.Changes) == "" {
		return changes, nil
	}
	if err := common.UnmarshalJsonStr(report.Changes, &changes); err != nil {
		return nil, err
	}
	return changes, nil
}

// priceSentinelSyncRequest asks to apply a whole report at once.
type priceSentinelSyncRequest struct {
	Token string `json:"token"`
	// MarginPercent overrides the configured markup for this one sync. Absent
	// means "use the configured default", which is why it is a pointer: a
	// deliberate 0 (follow the upstream exactly) must be distinguishable from
	// an omitted field.
	MarginPercent *float64 `json:"margin_percent"`
}

// SyncPriceSentinelPrices applies every change in the link's report at once,
// marking each upstream price up by the requested margin.
//
// This is the page's primary action. A report routinely lists dozens of models,
// and writing them one row at a time is slow and easy to leave half finished —
// a half-applied report is worse than an untouched one, because the next
// comparison reports the models that were skipped as though nothing happened.
//
// Which changes become writes is decided by BuildSyncEdits, including leaving
// models we never priced alone. The margin matters because a sync that landed
// exactly on the upstream's own number would sell at cost.
func SyncPriceSentinelPrices(c *gin.Context) {
	var req priceSentinelSyncRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "无效的参数")
		return
	}
	record, err := model.VerifyPriceSentinelEditToken(req.Token)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}

	margin := price_sentinel_setting.GetMarginPercent()
	if req.MarginPercent != nil {
		margin = price_sentinel_setting.ClampMarginPercent(*req.MarginPercent)
	}

	report, err := model.GetPriceSentinelReport(record.ReportId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var changes []model.PriceChange
	if strings.TrimSpace(report.Changes) != "" {
		if err := common.UnmarshalJsonStr(report.Changes, &changes); err != nil {
			common.ApiErrorMsg(c, "报告内容无法解析")
			return
		}
	}

	edits, skipped := pricesentinel.BuildSyncEdits(changes, margin)

	applied := 0
	if len(edits) > 0 {
		applied, err = pricesentinel.ApplyPriceEdits(edits)
		if err != nil {
			common.ApiError(c, err)
			return
		}
	}

	recordManageAudit(c, "price_sentinel.sync", map[string]any{
		"report_id":      report.Id,
		"applied":        applied,
		"skipped":        skipped,
		"margin_percent": margin,
	})
	common.ApiSuccess(c, gin.H{
		"applied":        applied,
		"skipped":        skipped,
		"margin_percent": margin,
	})
}

// ValidatePriceSentinelToken lets the edit page check a link before the
// operator types their access token, so an expired link fails fast.
func ValidatePriceSentinelToken(c *gin.Context) {
	var body struct {
		Token string `json:"token"`
	}
	// The token is a capability. It has to travel in the body: a query parameter
	// is written into access logs and Referer headers, which is enough to redeem
	// the link without ever seeing the mail.
	if err := common.DecodeJson(c.Request.Body, &body); err != nil || strings.TrimSpace(body.Token) == "" {
		common.ApiErrorMsg(c, "缺少链接令牌")
		return
	}
	record, err := model.VerifyPriceSentinelEditToken(body.Token)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	report, err := model.GetPriceSentinelReport(record.ReportId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	changes := []model.PriceChange{}
	if strings.TrimSpace(report.Changes) != "" {
		_ = common.UnmarshalJsonStr(report.Changes, &changes)
	}
	common.ApiSuccess(c, gin.H{
		"report":     priceSentinelReportForClient(report),
		"changes":    changes,
		"expires_at": record.ExpiresAt,
	})
}

var upstreamAddress = regexp.MustCompile(`https?://\S+`)

func priceSentinelReportForClient(report *model.PriceSentinelReport) *model.PriceSentinelReport {
	if report == nil {
		return nil
	}
	visible := *report
	visible.UpstreamUrl = ""
	visible.FetchError = strings.TrimSpace(upstreamAddress.ReplaceAllString(visible.FetchError, "upstream"))
	return &visible
}

// errPriceSentinelDisabled is returned when the sentinel is switched off.
var errPriceSentinelDisabled = errors.New("price sentinel is disabled")

// RegisterPriceSentinelTask wires the scheduled check into the task runner.
func RegisterPriceSentinelTask() {
	service.RegisterSystemTaskHandler(priceSentinelHandler{})
}
