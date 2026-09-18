package controller

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/thanhpk/randstr"

	"github.com/gin-gonic/gin"
)

// Redemption code purchase.
//
// A buyer pays real money through a payment provider and receives redemption
// codes, so this deliberately never touches wallet balance: buying codes with
// balance would be a transfer between two of the site's own ledgers, not a
// purchase. The order is stored as an ordinary paid order carrying the codes'
// terms, and settlement mints the codes inside the same transaction that marks
// the order paid (see model.settleTopUp), so a paid order always has its codes
// and a failed mint leaves the order pending.
const maxRedemptionPurchaseInt64 = int64(^uint64(0) >> 1)

type RedemptionPurchaseRequest struct {
	UnitAmount    int64  `json:"unit_amount"`
	Amount        int64  `json:"amount"` // compatibility alias for the top-up payload shape
	Quantity      int    `json:"quantity"`
	PaymentMethod string `json:"payment_method"`
}

type redemptionPurchaseContext struct {
	Request   RedemptionPurchaseRequest
	User      *model.User
	Group     string
	Total     int64
	UnitQuota int
	PayMoney  float64
}

func redemptionPurchaseEnabled() bool {
	return operation_setting.IsPaymentComplianceConfirmed() &&
		operation_setting.GetPaymentSetting().RedemptionPurchaseEnabled
}

// redemptionPurchasePaymentMethods lists the methods a code purchase may use.
//
// Balance is excluded on purpose (see the package comment). Only providers that
// are both configured and implemented here are offered, so the console cannot
// present a checkout that would fail after the buyer committed to it.
func redemptionPurchasePaymentMethods() []string {
	if !redemptionPurchaseEnabled() {
		return []string{}
	}
	methods := make([]string, 0, len(operation_setting.PayMethods)+1)
	seen := make(map[string]struct{}, len(operation_setting.PayMethods)+1)
	add := func(method string) {
		if method == "" || method == model.PaymentMethodBalance {
			return
		}
		if _, ok := seen[method]; ok {
			return
		}
		seen[method] = struct{}{}
		methods = append(methods, method)
	}
	if isEpayTopUpEnabled() {
		for _, method := range operation_setting.PayMethods {
			add(method["type"])
		}
	}
	if isStripeTopUpEnabled() {
		add(model.PaymentMethodStripe)
	}
	return methods
}

func isRedemptionPurchasePaymentMethodAvailable(method string) bool {
	for _, available := range redemptionPurchasePaymentMethods() {
		if available == method {
			return true
		}
	}
	return false
}

// redemptionPurchaseMinAmount is the provider's own floor for a single payment,
// so a code purchase is rejected for the same reason a top-up would be.
func redemptionPurchaseMinAmount(method string) int64 {
	if method == model.PaymentMethodStripe {
		return getStripeMinTopup()
	}
	return getMinTopup()
}

func normalizeRedemptionPurchaseAmount(req RedemptionPurchaseRequest) (int64, error) {
	amount := req.UnitAmount
	if amount <= 0 {
		amount = req.Amount
	}
	if amount <= 0 {
		return 0, errors.New("兑换码面值必须大于 0")
	}
	if req.Quantity <= 0 || req.Quantity > model.MaxRedemptionPurchaseCount {
		return 0, fmt.Errorf("兑换码数量必须在 1 到 %d 之间", model.MaxRedemptionPurchaseCount)
	}
	if amount > maxRedemptionPurchaseInt64/int64(req.Quantity) {
		return 0, errors.New("兑换码购买总额超出范围")
	}
	return amount, nil
}

func validateRedemptionPurchase(c *gin.Context, req RedemptionPurchaseRequest) (*redemptionPurchaseContext, error) {
	if !redemptionPurchaseEnabled() {
		return nil, errors.New("兑换码购买功能未启用")
	}
	if req.PaymentMethod == model.PaymentMethodBalance {
		return nil, errors.New("兑换码购买不能使用余额")
	}
	if !isRedemptionPurchasePaymentMethodAvailable(req.PaymentMethod) {
		return nil, errors.New("支付方式未配置或不可用于购买兑换码")
	}

	unitAmount, err := normalizeRedemptionPurchaseAmount(req)
	if err != nil {
		return nil, err
	}
	total := unitAmount * int64(req.Quantity)

	if minAmount := redemptionPurchaseMinAmount(req.PaymentMethod); minAmount > 0 && total < minAmount {
		return nil, fmt.Errorf("兑换码购买总额不能小于 %d", minAmount)
	}
	if maxAmount := getMaxTopUpAmount(); maxAmount > 0 && total > maxAmount {
		return nil, fmt.Errorf("兑换码购买总额不能大于 %d", maxAmount)
	}

	// The face value must convert to a creditable quota, otherwise the code the
	// buyer receives could not be redeemed for anything.
	unitQuota, err := getTopUpQuota(unitAmount)
	if err != nil || unitQuota <= 0 {
		return nil, errors.New("兑换码面值无效")
	}

	user, err := model.GetUserById(c.GetInt("id"), true)
	if err != nil || user == nil {
		return nil, errors.New("用户不存在")
	}

	ctx := &redemptionPurchaseContext{
		Request:   req,
		User:      user,
		Group:     user.Group,
		Total:     total,
		UnitQuota: unitQuota,
	}
	ctx.Request.UnitAmount = unitAmount

	var payMoney float64
	switch req.PaymentMethod {
	case model.PaymentMethodStripe:
		payMoney = GetChargedAmount(float64(total), *user)
	default:
		payMoney = getPayMoney(total, user.Group)
	}
	if payMoney < 0.01 || math.IsNaN(payMoney) || math.IsInf(payMoney, 0) {
		return nil, errors.New("支付金额过低或无效")
	}
	ctx.PayMoney = payMoney
	return ctx, nil
}

// redemptionPurchaseName is the order/code label. Kept short because the code
// list shows it, and characters such as the buyer id keep orders distinguishable
// in the provider's dashboard.
func redemptionPurchaseName(unitAmount int64, userID int) string {
	name := fmt.Sprintf("Code %d U%d", unitAmount, userID)
	if len(name) > 20 {
		return "Purchased code"
	}
	return name
}

func newRedemptionTopUp(
	ctx *redemptionPurchaseContext,
	tradeNo string,
	provider string,
	amount int64,
	money float64,
) *model.TopUp {
	return &model.TopUp{
		UserId:           ctx.User.Id,
		Amount:           amount,
		Money:            money,
		TradeNo:          tradeNo,
		PaymentMethod:    ctx.Request.PaymentMethod,
		PaymentProvider:  provider,
		CreateTime:       time.Now().Unix(),
		Status:           common.TopUpStatusPending,
		OrderType:        model.OrderTypeRedemption,
		RedemptionQuota:  ctx.UnitQuota,
		RedemptionCount:  ctx.Request.Quantity,
		RedemptionAmount: ctx.Request.UnitAmount,
		RedemptionName:   redemptionPurchaseName(ctx.Request.UnitAmount, ctx.User.Id),
	}
}

// GetRedemptionPurchaseOptions tells the console what it may offer: whether the
// feature is on, and which payment methods this deployment can actually take.
//
// The list is served rather than derived in the browser so the console cannot
// offer a method the server would reject, and so adding a provider later needs
// no frontend change.
func GetRedemptionPurchaseOptions(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"enabled":    redemptionPurchaseEnabled(),
			"methods":    redemptionPurchasePaymentMethods(),
			"max_count":  model.MaxRedemptionPurchaseCount,
			"max_amount": getMaxTopUpAmount(),
			"min_amount": getMinTopup(),
		},
	})
}

// RequestRedemptionPurchaseAmount returns the money one order would cost, so the
// console can show the price before the buyer commits to it.
func RequestRedemptionPurchaseAmount(c *gin.Context) {
	var req RedemptionPurchaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	ctx, err := validateRedemptionPurchase(c, req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": strconv.FormatFloat(ctx.PayMoney, 'f', 2, 64)})
}

// RequestRedemptionPurchase creates the order and hands back whatever the
// provider needs to collect the payment.
func RequestRedemptionPurchase(c *gin.Context) {
	var req RedemptionPurchaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	ctx, err := validateRedemptionPurchase(c, req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return
	}

	switch req.PaymentMethod {
	case model.PaymentMethodStripe:
		requestRedemptionStripePurchase(c, ctx)
	default:
		requestRedemptionEpayPurchase(c, ctx)
	}
}

func requestRedemptionEpayPurchase(c *gin.Context, ctx *redemptionPurchaseContext) {
	client := GetEpayClient()
	if client == nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "当前管理员未配置支付信息"})
		return
	}
	tradeNo := fmt.Sprintf("USR%dRC%s", ctx.User.Id, common.GetRandomString(18))
	callbackAddress := service.GetCallbackAddress()
	returnURL, _ := url.Parse(paymentReturnPath("/wallet"))
	notifyURL, _ := url.Parse(callbackAddress + "/api/user/epay/notify")

	uri, params, err := client.Purchase(&epay.PurchaseArgs{
		Type:           ctx.Request.PaymentMethod,
		ServiceTradeNo: tradeNo,
		Name:           fmt.Sprintf("RC%dX%d", ctx.Request.UnitAmount, ctx.Request.Quantity),
		Money:          strconv.FormatFloat(ctx.PayMoney, 'f', 2, 64),
		Device:         epay.PC,
		NotifyUrl:      notifyURL,
		ReturnUrl:      returnURL,
	})
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("创建兑换码支付失败 user_id=%d trade_no=%s error=%q", ctx.User.Id, tradeNo, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}
	if err := newRedemptionTopUp(ctx, tradeNo, model.PaymentProviderEpay, ctx.Total, ctx.PayMoney).Insert(); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("创建兑换码订单失败 user_id=%d trade_no=%s error=%q", ctx.User.Id, tradeNo, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("兑换码订单创建成功 user_id=%d trade_no=%s unit=%d qty=%d money=%.2f", ctx.User.Id, tradeNo, ctx.Request.UnitAmount, ctx.Request.Quantity, ctx.PayMoney))
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": params, "url": uri})
}

func requestRedemptionStripePurchase(c *gin.Context, ctx *redemptionPurchaseContext) {
	reference := fmt.Sprintf("redemption-ref-%d-%d-%s", ctx.User.Id, time.Now().UnixMilli(), randstr.String(4))
	referenceID := "ref_" + common.Sha1([]byte(reference))

	payLink, err := genStripeLink(referenceID, ctx.User.StripeCustomer, ctx.User.Email, ctx.Total, "", "")
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("创建兑换码 Stripe 支付失败 user_id=%d trade_no=%s error=%q", ctx.User.Id, referenceID, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}
	if err := newRedemptionTopUp(ctx, referenceID, model.PaymentProviderStripe, ctx.Total, ctx.PayMoney).Insert(); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("创建兑换码订单失败 user_id=%d trade_no=%s error=%q", ctx.User.Id, referenceID, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": gin.H{"pay_link": payLink}})
}
