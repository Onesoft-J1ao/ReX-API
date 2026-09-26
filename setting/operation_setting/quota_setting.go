package operation_setting

import (
	"fmt"
	"math"
	"strconv"

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
	// TrustQuotaUSD is the 元-equivalent wallet balance at or above which a
	// request skips pre-consume and is charged only at settle. 0 disables that
	// fast path. See the default below for why this fork ships it off.
	TrustQuotaUSD float64 `json:"trust_quota_usd"` // 钱包免预扣门槛，0 表示禁用
	// PreConsumeMultiplier scales the estimated input cost when reserving
	// quota. It only affects the reservation, never the final charge.
	PreConsumeMultiplier float64 `json:"pre_consume_multiplier"` // 预计输入费用的预扣倍率，仅影响预留
}

// 默认配置
var quotaSetting = QuotaSetting{
	EnableFreeModelPreConsume: true,
	MinBillablePrice:          DefaultMinBillablePrice,
	// 0 rather than upstream's 10. The fast path skipped pre-consume for any
	// wallet above this line and charged only at settle, so concurrent bursts
	// could overspend before the balance moved. Operators who want the old
	// behaviour can raise it deliberately from the settings page.
	TrustQuotaUSD:        0,
	PreConsumeMultiplier: 1,
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

// ValidateQuotaOption validates reservation settings before they are persisted.
func ValidateQuotaOption(key, value string) error {
	if key != "quota_setting.trust_quota_usd" && key != "quota_setting.pre_consume_multiplier" {
		return nil
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) || number < 0 {
		return fmt.Errorf("%s must be a finite non-negative number", key)
	}
	if key == "quota_setting.pre_consume_multiplier" && number == 0 {
		return fmt.Errorf("%s must be greater than zero", key)
	}
	return nil
}

// InputPreConsumeMultiplier rejects invalid runtime settings as well as invalid saves.
func InputPreConsumeMultiplier() (float64, error) {
	multiplier := quotaSetting.PreConsumeMultiplier
	if multiplier <= 0 || math.IsNaN(multiplier) || math.IsInf(multiplier, 0) {
		return 0, fmt.Errorf("pre-consume multiplier must be a finite number greater than zero")
	}
	return multiplier, nil
}
