package price_sentinel_setting

import (
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

// PriceSentinelSetting governs the hourly upstream price check.
//
// The sentinel only reports. It compares our price table against the upstream
// reference site and mails the operator a digest; changing a price stays a
// human decision, because the right answer differs per model — following an
// upstream increase protects margin, following a decrease protects customers,
// and only the operator knows which side of that trade each model sits on.
type PriceSentinelSetting struct {
	Enabled bool `json:"enabled"`
	// UpstreamPricingUrl is the reference site's pricing endpoint. It is public
	// and unauthenticated, so no credential is stored here.
	UpstreamPricingUrl string `json:"upstream_pricing_url"`
	// IntervalMinutes is how often the check runs. The upstream publishes price
	// changes by hand, so hourly is already generous.
	IntervalMinutes int `json:"interval_minutes"`
	// TimeoutSeconds bounds one fetch of the upstream pricing document.
	TimeoutSeconds int `json:"timeout_seconds"`
	// MinChangePercent filters noise: a model whose ratio moved less than this
	// is left out of the report. The upstream stores ratios as floats, so a
	// re-serialization can shift the last digit without any real price change.
	MinChangePercent float64 `json:"min_change_percent"`
	// ReportEmail receives the digest. Empty falls back to the root user's
	// address, which is where the existing upstream-model digest already goes.
	ReportEmail string `json:"report_email"`
	// EditTokenTTLHours bounds how long a mailed edit link stays usable.
	EditTokenTTLHours int `json:"edit_token_ttl_hours"`
	// IgnoredModels are never reported. Operators use it for models whose price
	// is deliberately kept above or below the upstream's.
	IgnoredModels string `json:"ignored_models"`
}

var priceSentinelSetting = PriceSentinelSetting{
	Enabled:            true,
	UpstreamPricingUrl: "https://ai.furry.vg/api/pricing",
	IntervalMinutes:    60,
	TimeoutSeconds:     20,
	MinChangePercent:   0.5,
	ReportEmail:        "",
	EditTokenTTLHours:  24,
	IgnoredModels:      "",
}

func init() {
	config.GlobalConfig.Register("price_sentinel_setting", &priceSentinelSetting)
}

func GetSetting() PriceSentinelSetting { return priceSentinelSetting }

// GetIntervalMinutes returns the configured cadence, floored at one minute so a
// malformed value cannot spin the scheduler.
func GetIntervalMinutes() int {
	if priceSentinelSetting.IntervalMinutes < 1 {
		return 60
	}
	return priceSentinelSetting.IntervalMinutes
}

// GetMinChangePercent returns the noise floor. A negative value would report
// every float wobble, so it is clamped to zero.
func GetMinChangePercent() float64 {
	if priceSentinelSetting.MinChangePercent < 0 {
		return 0
	}
	return priceSentinelSetting.MinChangePercent
}

// GetIgnoredModels returns the never-report list.
func GetIgnoredModels() []string {
	raw := strings.TrimSpace(priceSentinelSetting.IgnoredModels)
	if raw == "" {
		return nil
	}
	out := make([]string, 0, 8)
	for _, part := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
