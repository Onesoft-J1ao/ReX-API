package perf_metrics_setting

import (
	"time"

	"github.com/QuantumNous/new-api/setting/config"
)

type PerfMetricsSetting struct {
	Enabled                  bool   `json:"enabled"`
	ExcludeUpstream400Errors bool   `json:"exclude_upstream_400_errors"`
	FlushInterval            int    `json:"flush_interval"`
	BucketTime               string `json:"bucket_time"`
	RetentionDays            int    `json:"retention_days"`

	// UpstreamSummaryUrl mirrors another new-api instance's public perf-metrics
	// summary. Pay-per-request models cost real money to probe, so an upstream
	// that already samples them is a cheaper source of the same numbers. The
	// upstream wins for models it knows about; our own data fills the rest.
	UpstreamSummaryUrl string `json:"upstream_summary_url"`
	// UpstreamSummaryUrls mirrors model health from each configured upstream.
	// Keys are stable source names used by channel base-url mapping.
	UpstreamSummaryUrls map[string]string `json:"upstream_summary_urls"`
	// UpstreamEnabled gates the fetch. The upstream is a third party, so this is
	// on by default but can be turned off without losing the URL.
	UpstreamEnabled bool `json:"upstream_enabled"`
	// UpstreamTimeoutSeconds bounds one fetch. The upstream answers in 7-19s
	// (measured against the live instance), and the refresh runs in the
	// background, so this is set to comfortably outlast a slow answer rather
	// than to keep a page load responsive.
	UpstreamTimeoutSeconds int `json:"upstream_timeout_seconds"`
}

var perfMetricsSetting = PerfMetricsSetting{
	Enabled:                  true,
	ExcludeUpstream400Errors: false,
	FlushInterval:            5,
	BucketTime:               "hour",
	RetentionDays:            0,

	UpstreamSummaryUrl: "https://ai.furry.vg/api/perf-metrics/summary",
	UpstreamSummaryUrls: map[string]string{
		"furry":   "https://api.furry.vg/api/perf-metrics/summary",
		"sheapi":  "https://www.sheapi.cc/api/perf-metrics/summary",
		"mcapple": "https://api.mcapple.top/api/perf-metrics/summary",
	},
	UpstreamEnabled:        true,
	UpstreamTimeoutSeconds: 30,
}

func init() {
	config.GlobalConfig.Register("perf_metrics_setting", &perfMetricsSetting)
}

func GetSetting() PerfMetricsSetting {
	return perfMetricsSetting
}

func GetBucketSeconds() int64 {
	switch perfMetricsSetting.BucketTime {
	case "minute":
		return 60
	case "5min":
		return 300
	case "hour":
		return 3600
	default:
		return 3600
	}
}

func GetFlushIntervalMinutes() int {
	if perfMetricsSetting.FlushInterval < 1 {
		return 1
	}
	return perfMetricsSetting.FlushInterval
}

func GetUpstreamTimeout() time.Duration {
	seconds := perfMetricsSetting.UpstreamTimeoutSeconds
	if seconds < 1 {
		seconds = 30
	}
	return time.Duration(seconds) * time.Second
}
