package controller

import (
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

// refundAdvanceStatuses are the states an administrator may move a request into.
// The model owns the legal transitions between them; this list only rejects a
// target that is not part of the workflow at all.
var refundAdvanceStatuses = []string{
	model.RefundStatusApproved,
	model.RefundStatusCredited,
	model.RefundStatusRejected,
}

// refundErrorMessage turns the model's refund errors into a message an operator
// can act on. Anything unrecognised keeps its original text so an unexpected
// database failure is still visible.
func refundErrorMessage(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, model.ErrRefundRequestNotFound):
		return "退款申请不存在"
	case errors.Is(err, model.ErrRefundRequestStatus):
		return "退款申请当前状态不允许该操作"
	case errors.Is(err, model.ErrRefundRequestTerminal):
		return "退款申请已结算，无法再次变更"
	case errors.Is(err, model.ErrRefundRequestNotOwned):
		return "退款申请不属于该用户"
	case errors.Is(err, model.ErrRefundSubscriptionNotActive):
		return "订阅已失效，无法退款"
	case errors.Is(err, model.ErrNothingToRefund):
		return "没有可退款的额度"
	case errors.Is(err, model.ErrRefundNotAllowed):
		return "当前不允许退款"
	case errors.Is(err, model.ErrRefundDisabled):
		return "退款功能已关闭"
	}
	return err.Error()
}

// GetUserRefundRequests returns the caller's own refund requests, newest first,
// so the user can watch their progress on the wallet page.
func GetUserRefundRequests(c *gin.Context) {
	userId := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	requests, total, err := model.ListUserRefundRequests(userId, pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(requests)
	common.ApiSuccess(c, pageInfo)
}

// AdminListRefundRequests returns every refund request, optionally narrowed to
// one status.
func AdminListRefundRequests(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	status := strings.TrimSpace(c.Query("status"))
	if status != "" && !slices.Contains(refundAdvanceStatuses, status) && status != model.RefundStatusPending {
		common.ApiErrorMsg(c, "无效的退款状态")
		return
	}
	requests, total, err := model.ListRefundRequests(status, pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(requests)
	common.ApiSuccess(c, pageInfo)
}

type AdminCreatePlanRefundRequestPayload struct {
	UserId             int    `json:"user_id"`
	UserSubscriptionId int    `json:"user_subscription_id"`
	Remark             string `json:"remark"`
}

// AdminCreatePlanRefundRequest records a plan refund for the remaining allowance
// of one subscription. The allowance is only converted when the request is
// approved, so a pending request does not strand the user's plan.
func AdminCreatePlanRefundRequest(c *gin.Context) {
	if !operation_setting.GetRefundSetting().PlanRefundEnabled {
		common.ApiErrorMsg(c, "套餐退款功能已关闭")
		return
	}
	var payload AdminCreatePlanRefundRequestPayload
	if err := c.ShouldBindJSON(&payload); err != nil || payload.UserId <= 0 || payload.UserSubscriptionId <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	request, downgradeGroup, err := model.CreatePlanRefundRequest(
		payload.UserId, payload.UserSubscriptionId, payload.Remark, c.GetInt("id"))
	if err != nil {
		common.ApiErrorMsg(c, refundErrorMessage(err))
		return
	}
	model.RecordRefundRequestLog(request, "申请")
	recordManageAuditFor(c, payload.UserId, "refund.plan_create", map[string]any{
		"refund_id":            request.Id,
		"user_subscription_id": request.UserSubscriptionId,
		"principal_quota":      request.PrincipalQuota,
		"quota":                request.Quota,
		"amount":               request.Amount,
		"downgrade_group":      downgradeGroup,
	})
	common.ApiSuccess(c, request)
}

type AdminCreateCashRefundRequestPayload struct {
	UserId int     `json:"user_id"`
	Ratio  float64 `json:"ratio"`
	Remark string  `json:"remark"`
}

// AdminCreateCashRefundRequest pays out a share of the user's withdrawable
// balance. The balance is frozen immediately so it cannot be spent while the
// payout is in flight; rejecting the request restores it.
//
// An omitted ratio (zero or absent) uses the operator's configured payout ratio
// rather than refunding nothing.
func AdminCreateCashRefundRequest(c *gin.Context) {
	refundSetting := operation_setting.GetRefundSetting()
	if !refundSetting.CashRefundEnabled {
		common.ApiErrorMsg(c, "现金退款功能已关闭")
		return
	}
	var payload AdminCreateCashRefundRequestPayload
	if err := c.ShouldBindJSON(&payload); err != nil || payload.UserId <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	ratio := payload.Ratio
	if ratio <= 0 {
		ratio = refundSetting.CashRefundRatioOrDefault()
	}
	request, err := model.CreateCashRefundRequest(payload.UserId, ratio, payload.Remark, c.GetInt("id"))
	if err != nil {
		common.ApiErrorMsg(c, refundErrorMessage(err))
		return
	}
	model.RecordRefundRequestLog(request, "申请")
	recordManageAuditFor(c, payload.UserId, "refund.cash_create", map[string]any{
		"refund_id":       request.Id,
		"principal_quota": request.PrincipalQuota,
		"deducted_quota":  request.DeductedQuota,
		"quota":           request.Quota,
		"amount":          request.Amount,
		"ratio":           ratio,
	})
	common.ApiSuccess(c, request)
}

type AdminAdvanceRefundRequestPayload struct {
	Status string `json:"status"`
}

// AdminAdvanceRefundRequest moves a request through the refund workflow:
// pending -> approved (settles a plan refund), approved -> credited (confirms the
// payout landed), and pending|approved -> rejected (restores a frozen balance).
func AdminAdvanceRefundRequest(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if id <= 0 {
		common.ApiErrorMsg(c, "无效的ID")
		return
	}
	var payload AdminAdvanceRefundRequestPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	targetStatus := strings.TrimSpace(payload.Status)
	if !slices.Contains(refundAdvanceStatuses, targetStatus) {
		common.ApiErrorMsg(c, "无效的目标状态")
		return
	}
	request, downgradeGroup, err := model.AdvanceRefundRequest(id, targetStatus, c.GetInt("id"))
	if err != nil {
		common.ApiErrorMsg(c, refundErrorMessage(err))
		return
	}
	model.RecordRefundRequestLog(request, "状态更新为 "+targetStatus)
	recordManageAuditFor(c, request.UserId, "refund.advance", map[string]any{
		"refund_id":       request.Id,
		"kind":            request.Kind,
		"status":          request.Status,
		"quota":           request.Quota,
		"amount":          request.Amount,
		"downgrade_group": downgradeGroup,
	})
	common.ApiSuccess(c, request)
}
