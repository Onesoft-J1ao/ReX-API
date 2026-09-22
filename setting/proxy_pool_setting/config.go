package proxy_pool_setting

import (
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

// ProxyPoolSetting configures the outbound-proxy pool this gateway can draw
// channel proxies from. The pool is an external HTTP service (jhao104/proxy_pool
// and compatible implementations); its answers are untrusted, so every value
// here is either a bound on what we accept or a switch an operator can turn off.
type ProxyPoolSetting struct {
	Enabled             bool   `json:"enabled"`
	PoolURL             string `json:"pool_url"`               // e.g. http://139.224.75.133:5555
	ProxyType           string `json:"proxy_type"`             // optional ?type= forwarded to the pool (https/http/socks5)
	RequestTimeoutSeconds int  `json:"request_timeout_seconds"` // per-request timeout against the pool
	CacheTTLSeconds     int    `json:"cache_ttl_seconds"`      // how long one drawn proxy stays usable for auto-assignment
	AutoAssignChannels  bool   `json:"auto_assign_channels"`   // give channels with no explicit proxy a pool proxy at relay time
	MaxAssignBatch      int    `json:"max_assign_batch"`       // upper bound on channels one assign request may touch
}

var proxyPoolSetting = ProxyPoolSetting{
	Enabled:               false,
	PoolURL:               "http://139.224.75.133:5555",
	ProxyType:             "",
	RequestTimeoutSeconds: 5,
	CacheTTLSeconds:       300,
	AutoAssignChannels:    false,
	MaxAssignBatch:        50,
}

func init() {
	config.GlobalConfig.Register("proxy_pool_setting", &proxyPoolSetting)
}

func GetSetting() ProxyPoolSetting { return proxyPoolSetting }

func GetPoolURL() string { return strings.TrimRight(strings.TrimSpace(proxyPoolSetting.PoolURL), "/") }

func GetRequestTimeoutSeconds() int {
	if proxyPoolSetting.RequestTimeoutSeconds < 1 {
		return 5
	}
	return proxyPoolSetting.RequestTimeoutSeconds
}

func GetCacheTTLSeconds() int {
	if proxyPoolSetting.CacheTTLSeconds < 1 {
		return 300
	}
	return proxyPoolSetting.CacheTTLSeconds
}

// GetMaxAssignBatch clamps the batch bound into a sane range: a zero/negative
// value would allow nothing, and an unbounded value lets one request walk the
// whole pool.
func GetMaxAssignBatch() int {
	return min(max(proxyPoolSetting.MaxAssignBatch, 1), 500)
}
