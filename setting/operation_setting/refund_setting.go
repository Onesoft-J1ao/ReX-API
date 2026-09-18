package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

// DefaultCashRefundRatio is the share of a user's remaining unused cash balance
// that a general refund pays out. The remainder (25%) is retained as a handling
// fee, which is why a general refund never returns the full balance.
const DefaultCashRefundRatio = 0.75

// RefundSetting configures the two refund flows.
//
// A Go Plan refund converts the plan's remaining allowance back into wallet
// balance at principal proportion — no fee, because the money never leaves the
// platform. A general refund pays cash out of the platform, so it charges a
// fee and is administrator-only.
type RefundSetting struct {
	CashRefundEnabled bool    `json:"cash_refund_enabled"`
	CashRefundRatio   float64 `json:"cash_refund_ratio"`
	PlanRefundEnabled bool    `json:"plan_refund_enabled"`
	// PlanRefundToGiftQuota credits a plan refund as non-withdrawable wallet
	// balance. The owner's rule is "不退现金，退回网页钱包", so the refunded
	// principal stays spendable on the site but cannot be withdrawn again.
	PlanRefundToGiftQuota bool `json:"plan_refund_to_gift_quota"`
}

var refundSetting = RefundSetting{
	CashRefundEnabled:     true,
	CashRefundRatio:       DefaultCashRefundRatio,
	PlanRefundEnabled:     true,
	PlanRefundToGiftQuota: true,
}

func init() {
	config.GlobalConfig.Register("refund_setting", &refundSetting)
}

func GetRefundSetting() *RefundSetting {
	return &refundSetting
}

// CashRefundRatioOrDefault rejects a ratio outside (0, 1]; anything else would
// either refund nothing or refund more than the balance.
func (s *RefundSetting) CashRefundRatioOrDefault() float64 {
	if s == nil || s.CashRefundRatio <= 0 || s.CashRefundRatio > 1 {
		return DefaultCashRefundRatio
	}
	return s.CashRefundRatio
}
