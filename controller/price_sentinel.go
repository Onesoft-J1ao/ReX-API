package controller

import (
	"context"
	"errors"
	"fmt"
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
// much, and mails a single-use link to an edit page; changing a price stays the
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

	body := pricesentinel.BuildReport(comparison, checkedAt, editUrl)
	if compareErr != nil {
		body = fmt.Sprintf(
			"上游价格巡检报告（%s）\n\n本次检查未能读取上游价格：\n  %s\n\n下一轮会自动重试。\n",
			time.Unix(checkedAt, 0).Format("2006-01-02 15:04"),
			compareErr.Error(),
		)
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
	common.ApiSuccess(c, gin.H{"report": report, "changes": changes})
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
	Token     string   `json:"token"`
	ModelName string   `json:"model_name"`
	Field     string   `json:"field"`
	Value     float64  `json:"value"`
}

// ApplyPriceSentinelEdit consumes the mailed token and writes one price.
//
// The token only proves the request came from a link we mailed; it is not an
// admin credential. This route therefore sits behind AdminAuth as well, so the
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

	if _, err := model.ConsumePriceSentinelEditToken(req.Token); err != nil {
		common.ApiErrorMsg(c, err.Error())
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

// ValidatePriceSentinelToken lets the edit page check a link before the
// operator types their access token, so an expired link fails fast.
func ValidatePriceSentinelToken(c *gin.Context) {
	token := strings.TrimSpace(c.Query("token"))
	if token == "" {
		common.ApiErrorMsg(c, "缺少链接令牌")
		return
	}
	record, err := model.FindPriceSentinelEditToken(token)
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
		"report":  report,
		"changes": changes,
		"expires_at": record.ExpiresAt,
	})
}

// errPriceSentinelDisabled is returned when the sentinel is switched off.
var errPriceSentinelDisabled = errors.New("price sentinel is disabled")

// RegisterPriceSentinelTask wires the scheduled check into the task runner.
func RegisterPriceSentinelTask() {
	service.RegisterSystemTaskHandler(priceSentinelHandler{})
}
