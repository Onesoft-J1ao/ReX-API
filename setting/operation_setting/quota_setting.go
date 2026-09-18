package operation_setting

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

// DefaultMinBillablePrice is the smallest 元 amount a request may cost.
// A model configured with a zero price or zero ratio used to be billed nothing
// and was usable by an account with an empty wallet. Billing it this tiny
// non-zero amount instead keeps the model effectively free for anyone who has
// ever topped up (the platform's minimum top-up is 1 元) while making the
// natural insufficient-quota path — not a separate allow-list — the thing that
// gates brand-new accounts.
const DefaultMinBillablePrice = 0.0002

type QuotaSetting struct {
	EnableFreeModelPreConsume bool `json:"enable_free_model_pre_consume"` // 是否对免费模型启用预消耗
	// MinBillablePrice is the 元 floor applied when a model's resolved price or
	// ratio is zero. Set it to 0 to restore the legacy "zero price = free" behavior.
	MinBillablePrice float64 `json:"min_billable_price"`
}

// 默认配置
var quotaSetting = QuotaSetting{
	EnableFreeModelPreConsume: true,
	MinBillablePrice:          DefaultMinBillablePrice,
}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("quota_setting", &quotaSetting)
}

func GetQuotaSetting() *QuotaSetting {
	return &quotaSetting
}

// MinBillableQuota converts MinBillablePrice into quota units. The result is at
// least 1 so the floor can never round back down to a free request; it is
// computed through common.QuotaFromFloat, which saturates instead of wrapping.
func (s *QuotaSetting) MinBillableQuota() int {
	if s == nil || s.MinBillablePrice <= 0 {
		return 0
	}
	quota := common.QuotaFromFloat(s.MinBillablePrice * common.QuotaPerUnit)
	if quota < 1 {
		return 1
	}
	return quota
}

// ModelPricedAtZero reports whether the resolved model pricing is zero, i.e.
// whether the model would have been free before the minimum-price floor
// existed. usePrice selects the per-call price field over the token ratio.
func ModelPricedAtZero(usePrice bool, modelRatio float64, modelPrice float64) bool {
	if usePrice {
		return modelPrice <= 0
	}
	return modelRatio <= 0
}

// MinBillableQuota is the package-level convenience accessor for the configured
// per-request floor.
func MinBillableQuota() int {
	return quotaSetting.MinBillableQuota()
}
