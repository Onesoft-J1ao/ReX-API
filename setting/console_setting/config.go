package console_setting

import "github.com/QuantumNous/new-api/setting/config"

type ConsoleSetting struct {
	ApiInfo              string `json:"api_info"`              // 控制台 API 信息 (JSON 数组字符串)
	UptimeKumaGroups     string `json:"uptime_kuma_groups"`    // Uptime Kuma 分组配置 (JSON 数组字符串)
	Announcements        string `json:"announcements"`         // 系统公告 (JSON 数组字符串)
	FAQ                  string `json:"faq"`                   // 常见问题 (JSON 数组字符串)
	ApiInfoEnabled       bool   `json:"api_info_enabled"`      // 是否启用 API 信息面板
	UptimeKumaEnabled    bool   `json:"uptime_kuma_enabled"`   // 是否启用 Uptime Kuma 面板
	AnnouncementsEnabled bool   `json:"announcements_enabled"` // 是否启用系统公告面板
	FAQEnabled           bool   `json:"faq_enabled"`           // 是否启用常见问答面板

	// UpstreamStatusUrl mirrors an upstream provider's public status page into
	// the console, next to our own monitors. It holds a complete status page URL
	// rather than a base URL plus slug because operators copy it straight out of
	// the provider's address bar.
	UpstreamStatusUrl  string `json:"upstream_status_url"`
	UpstreamStatusName string `json:"upstream_status_name"`
	// UpstreamStatusEnabled gates the fetch. The provider is a third party, so
	// this is on by default but can be turned off without losing the URL.
	UpstreamStatusEnabled bool `json:"upstream_status_enabled"`

	// SupportQqUrl is the QQ group join link shown on the user dashboard. The
	// value is the link the group's "join" widget points at, so it carries the
	// group key and auth key and must not be reconstructed from a group number.
	SupportQqUrl string `json:"support_qq_url"`
	// SupportQqEnabled gates the card. On by default so a fresh install that
	// pastes a link immediately shows it.
	SupportQqEnabled bool `json:"support_qq_enabled"`
}

// 默认配置
var defaultConsoleSetting = ConsoleSetting{
	ApiInfo:              "",
	UptimeKumaGroups:     "",
	Announcements:        "",
	FAQ:                  "",
	ApiInfoEnabled:       true,
	UptimeKumaEnabled:    true,
	AnnouncementsEnabled: true,
	FAQEnabled:           true,
	// 默认镜像上游 S3AI 的公开状态页
	UpstreamStatusUrl:     "https://status.s3.fan/status/s3ai-api",
	UpstreamStatusName:    "上游状态",
	UpstreamStatusEnabled: true,

	SupportQqUrl:     "https://qm.qq.com/cgi-bin/qm/qr?k=AVppaqUacwRihy7NYrc9ir8u7XH0eAVk&jump_from=webapi&authKey=7+bvl3k1aDPsSLDH9Owx57JOP97QO6/zm7+tvArSwI+re5/GEI1n2skM9JjbuARA",
	SupportQqEnabled: true,
}

// 全局实例
var consoleSetting = defaultConsoleSetting

func init() {
	// 注册到全局配置管理器，键名为 console_setting
	config.GlobalConfig.Register("console_setting", &consoleSetting)
}

// GetConsoleSetting 获取 ConsoleSetting 配置实例
func GetConsoleSetting() *ConsoleSetting {
	return &consoleSetting
}
