package model_catalog_setting

import (
	"strings"
	"time"

	"github.com/QuantumNous/new-api/setting/config"
)

// ModelCatalogSetting controls the model-catalog metadata overlay.
//
// The pricing endpoint can only republish what the gateway stores, and the
// gateway stores billing data — context window, modalities and capability flags
// have never had a column. Rather than add schema for display-only fields, the
// catalog is read from models.dev at request time and merged into the pricing
// response, the same way the upstream perf-metrics mirror is layered on top of
// local numbers.
type ModelCatalogSetting struct {
	Enabled bool `json:"enabled"`
	// SourceURL is the models.dev catalog document. Operators may point it at a
	// mirror when the public host is unreachable from their region.
	SourceURL string `json:"source_url"`
	// TimeoutSeconds bounds one fetch. The document is ~4.7MB, so this is
	// generous enough for a slow link without stalling a page load behind it.
	TimeoutSeconds int `json:"timeout_seconds"`
	// CacheMinutes is how long a fetched catalog is reused. The upstream
	// publishes daily, so a long TTL costs nothing and keeps the page fast.
	CacheMinutes int `json:"cache_minutes"`
}

var modelCatalogSetting = ModelCatalogSetting{
	Enabled:        true,
	SourceURL:      "https://models.dev/api.json",
	TimeoutSeconds: 15,
	CacheMinutes:   360,
}

func init() {
	config.GlobalConfig.Register("model_catalog_setting", &modelCatalogSetting)
}

func GetSetting() ModelCatalogSetting { return modelCatalogSetting }

// GetSourceURL returns the configured catalog URL, or the public default when
// an operator has cleared it.
func GetSourceURL() string {
	if url := strings.TrimSpace(modelCatalogSetting.SourceURL); url != "" {
		return url
	}
	return "https://models.dev/api.json"
}

func GetTimeout() time.Duration {
	seconds := modelCatalogSetting.TimeoutSeconds
	if seconds < 1 {
		seconds = 15
	}
	return time.Duration(seconds) * time.Second
}

func GetCacheTTL() time.Duration {
	minutes := modelCatalogSetting.CacheMinutes
	if minutes < 1 {
		minutes = 360
	}
	return time.Duration(minutes) * time.Minute
}
