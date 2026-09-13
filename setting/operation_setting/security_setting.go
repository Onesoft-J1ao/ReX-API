package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

// SecuritySetting holds deployment-level access rules that are not tied to one
// feature: addresses that must not reach the instance at all.
type SecuritySetting struct {
	// BlockedIPs are single addresses or CIDR ranges. Matching is on the client
	// address the request came from, which behind a proxy is the forwarded one.
	BlockedIPEnabled bool     `json:"blocked_ip_enabled"`
	BlockedIPs       []string `json:"blocked_ips"`
}

var securitySetting = SecuritySetting{
	BlockedIPEnabled: false,
	BlockedIPs:       []string{},
}

func init() {
	config.GlobalConfig.Register("security_setting", &securitySetting)
}

func GetSecuritySetting() *SecuritySetting {
	return &securitySetting
}
