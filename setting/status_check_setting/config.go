package status_check_setting

import (
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

// StatusCheckSetting governs the /api/status-check group-health page and its
// background probe job. Mirrors the perf_metrics_setting wiring so the option
// store loads/saves it under the status_check_setting.<field> key namespace.
type StatusCheckSetting struct {
	Enabled              bool   `json:"enabled"`                // default false — admin flips on after wiring groups
	ProbeIntervalMinutes int    `json:"probe_interval_minutes"` // default 10 (keep宽松 to dodge upstream rate-limit)
	Groups               string `json:"groups"`                 // comma-separated group names probed
	ExcludedModels       string `json:"excluded_models"`        // comma-separated model slugs to skip
	Announcement         string `json:"announcement"`           // markdown body shown at top of /status
	PassiveWindowHours   int    `json:"passive_window_hours"`   // default 24, perf_metrics merge window
	// ProbeFreeModelsOnly skips probing any group whose ratio is not zero. A
	// probe is a real upstream request, so probing a pay-per-request model
	// bills us every cycle for a signal the mirrored upstream already reports.
	ProbeFreeModelsOnly bool `json:"probe_free_models_only"`

	// Flexible probing paces the automatic group probe by real traffic. A group
	// that is already serving requests has just proven it works, so the probe
	// waits for it to go quiet rather than paying for a scheduled test on top.
	// Off by default: the probe cadence only changes when an operator asks.
	FlexibleProbingEnabled       bool `json:"flexible_probing_enabled"`
	FlexibleIdleMinutes          int  `json:"flexible_idle_minutes"`           // probe a group only after this much quiet
	FlexibleMaxConsecutiveProbes int  `json:"flexible_max_consecutive_probes"` // cap on probes while nothing needs the group
}

// Bounds for the flexible pacing options. They exist to cap accidental spend
// from a malformed setting: too small an idle window probes constantly, and
// too large a cap lets an idle group be probed forever.
const (
	DefaultFlexibleIdleMinutes          = 15
	DefaultFlexibleMaxConsecutiveProbes = 40
	maxFlexibleIdleMinutes              = 24 * 60
	maxFlexibleConsecutiveProbes        = 1000
)

var statusCheckSetting = StatusCheckSetting{
	Enabled:                      false,
	ProbeIntervalMinutes:         10,
	Groups:                       "default",
	ExcludedModels:               "",
	Announcement:                 "",
	PassiveWindowHours:           24,
	ProbeFreeModelsOnly:          true,
	FlexibleProbingEnabled:       false,
	FlexibleIdleMinutes:          DefaultFlexibleIdleMinutes,
	FlexibleMaxConsecutiveProbes: DefaultFlexibleMaxConsecutiveProbes,
}

func init() {
	config.GlobalConfig.Register("status_check_setting", &statusCheckSetting)
}

func GetSetting() StatusCheckSetting { return statusCheckSetting }

// GetGroups returns the configured probe groups with empty entries trimmed.
// Empty result yields {"default"} so a freshly-installed instance still has a
// sane target instead of probing zero groups.
func GetGroups() []string {
	raw := strings.TrimSpace(statusCheckSetting.Groups)
	if raw == "" {
		return []string{"default"}
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return []string{"default"}
	}
	return out
}

// GetExcludedModelsMap builds a set lookup; nil/empty map means "no exclusions".
func GetExcludedModelsMap() map[string]struct{} {
	raw := strings.TrimSpace(statusCheckSetting.ExcludedModels)
	if raw == "" {
		return nil
	}
	m := map[string]struct{}{}
	for _, p := range strings.Split(raw, ",") {
		if t := strings.TrimSpace(p); t != "" {
			m[t] = struct{}{}
		}
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

func GetIntervalMinutes() int {
	if statusCheckSetting.ProbeIntervalMinutes < 1 {
		return 10
	}
	return statusCheckSetting.ProbeIntervalMinutes
}

func GetPassiveWindowHours() int {
	if statusCheckSetting.PassiveWindowHours < 1 {
		return 24
	}
	return statusCheckSetting.PassiveWindowHours
}

// FlexibleProbingEnabled reports whether the automatic probe should wait for a
// group to go quiet instead of probing on every interval.
func FlexibleProbingEnabled() bool { return statusCheckSetting.FlexibleProbingEnabled }

// GetFlexibleIdleMinutes returns the quiet period a group must show before the
// automatic probe runs again. Out-of-range values fall back to the default
// rather than being clamped to a bound, so a typo cannot silently turn probing
// off for a day.
func GetFlexibleIdleMinutes() int {
	minutes := statusCheckSetting.FlexibleIdleMinutes
	if minutes < 1 || minutes > maxFlexibleIdleMinutes {
		return DefaultFlexibleIdleMinutes
	}
	return minutes
}

// GetFlexibleMaxConsecutiveProbes returns how many automatic probes may run in
// a row while real traffic never arrives.
func GetFlexibleMaxConsecutiveProbes() int {
	probes := statusCheckSetting.FlexibleMaxConsecutiveProbes
	if probes < 1 || probes > maxFlexibleConsecutiveProbes {
		return DefaultFlexibleMaxConsecutiveProbes
	}
	return probes
}

// ShouldProbeOnlyFreeModels reports whether paid groups are skipped during a
// probe cycle.
func ShouldProbeOnlyFreeModels() bool {
	return statusCheckSetting.ProbeFreeModelsOnly
}
