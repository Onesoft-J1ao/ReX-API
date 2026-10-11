package price_sentinel_setting

import (
	"strings"
	"time"

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
	// SendTime pins that cadence to a wall-clock time of day, "HH:MM". The digest
	// then goes out at that time and every IntervalMinutes after it, so the mail
	// lands in a working hour instead of drifting with each run. Empty keeps the
	// plain "every IntervalMinutes since the last run" behaviour.
	SendTime string `json:"send_time"`
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
	// MarginPercent is the gross margin a synchronisation keeps over the
	// upstream price: ours = upstream / (1 - MarginPercent). The upstream figure
	// is what we pay, so following it at its own number would sell at cost, and
	// the digest exists to be applied in bulk. 0.2 means 20% of revenue kept,
	// i.e. x1.25 — not a 20% markup, which would keep only 16.67%.
	MarginPercent float64 `json:"margin_percent"`
}

var priceSentinelSetting = PriceSentinelSetting{
	Enabled:            true,
	UpstreamPricingUrl: "https://ai.furry.vg/api/pricing",
	IntervalMinutes:    60,
	SendTime:           "",
	TimeoutSeconds:     20,
	MinChangePercent:   0.5,
	ReportEmail:        "",
	EditTokenTTLHours:  24,
	IgnoredModels:      "",
	MarginPercent:      0.2,
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

// GetSendTime returns the configured anchor as an hour and minute, or ok=false
// when it is unset or malformed. A malformed value falls back to the plain
// interval rather than stalling the digest, since nothing validates the stored
// string on the way in.
func GetSendTime() (hour int, minute int, ok bool) {
	raw := strings.TrimSpace(priceSentinelSetting.SendTime)
	if raw == "" {
		return 0, 0, false
	}
	parsed, err := time.Parse("15:04", raw)
	if err != nil {
		return 0, 0, false
	}
	return parsed.Hour(), parsed.Minute(), true
}

// GetMinChangePercent returns the noise floor. A negative value would report
// every float wobble, so it is clamped to zero.
func GetMinChangePercent() float64 {
	if priceSentinelSetting.MinChangePercent < 0 {
		return 0
	}
	return priceSentinelSetting.MinChangePercent
}

// MaxMarginPercent bounds the markup. A value far past this is a typo rather
// than an intended price, and the apply path multiplies money by it.
const MaxMarginPercent = 10.0

// ClampMarginPercent bounds a markup to the range the apply path will accept.
// It is exported because the edit page may send its own margin for one sync, and
// that value has to be bounded by the same rule as the stored one.
func ClampMarginPercent(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > MaxMarginPercent {
		return MaxMarginPercent
	}
	return value
}

// GetMarginPercent returns the configured markup, clamped to a sane range so a
// malformed value cannot multiply every synced price into nonsense.
func GetMarginPercent() float64 {
	return ClampMarginPercent(priceSentinelSetting.MarginPercent)
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
