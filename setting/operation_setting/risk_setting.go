package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

// RiskControlSetting detects tokens whose traffic looks automated or abusive.
// Every threshold is a per-token sliding-window budget describing what normal
// traffic looks like, so a signal means the window went past that budget. A
// zero threshold disables the signal it belongs to.
type RiskControlSetting struct {
	Enabled bool `json:"enabled"`

	WindowSeconds int `json:"window_seconds"` // sliding window every signal is measured over

	MaxRequestsPerWindow int `json:"max_requests_per_window"` // request burst budget
	MaxQuotaPerWindow    int `json:"max_quota_per_window"`    // quota burn budget

	MinRequestsForPattern int     `json:"min_requests_for_pattern"` // requests needed before interval regularity is judged
	IntervalCVThreshold   float64 `json:"interval_cv_threshold"`    // coefficient of variation below this counts as machine-regular

	AutoDisableToken     bool `json:"auto_disable_token"`
	AlertCooldownSeconds int  `json:"alert_cooldown_seconds"`
}

var riskControlSetting = RiskControlSetting{
	Enabled:               true,
	WindowSeconds:         60,
	MaxRequestsPerWindow:  300,
	MaxQuotaPerWindow:     0,
	MinRequestsForPattern: 30,
	IntervalCVThreshold:   0.05,
	AutoDisableToken:      true,
	AlertCooldownSeconds:  600,
}

func init() {
	config.GlobalConfig.Register("risk_control_setting", &riskControlSetting)
}

func GetRiskControlSetting() *RiskControlSetting {
	return &riskControlSetting
}
