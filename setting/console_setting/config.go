package console_setting

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

type ConsoleSetting struct {
	ApiInfo                  string `json:"api_info"`              // 控制台 API 信息 (JSON 数组字符串)
	UptimeKumaGroups         string `json:"uptime_kuma_groups"`    // Uptime Kuma 分组配置 (JSON 数组字符串)
	Announcements            string `json:"announcements"`         // 系统公告 (JSON 数组字符串)
	FAQ                      string `json:"faq"`                   // 常见问题 (JSON 数组字符串)
	ApiInfoEnabled           bool   `json:"api_info_enabled"`      // 是否启用 API 信息面板
	UptimeKumaEnabled        bool   `json:"uptime_kuma_enabled"`   // 是否启用 Uptime Kuma 面板
	AnnouncementsEnabled     bool   `json:"announcements_enabled"` // 是否启用系统公告面板
	FAQEnabled               bool   `json:"faq_enabled"`           // 是否启用常见问答面板
	BackgroundImage          string `json:"background_image"`
	BackgroundRandomCategory string `json:"background_random_category"`
	BackgroundUnsplashQuery  string `json:"background_unsplash_query"`
	// Never returned to the SPA: it is only read server-side when calling
	// Unsplash on the visitor's behalf.
	BackgroundUnsplashAccessKey string `json:"background_unsplash_access_key"`
	BackgroundBlurOpacity      int    `json:"background_blur_opacity"`
	LegalBackgroundEnabled     bool   `json:"legal_background_enabled"`
	DefaultTheme               string `json:"default_theme"`
	DefaultThemeOverride     string `json:"default_theme_override"`
	DefaultThemePreset       string `json:"default_theme_preset"`
	DefaultThemeFont         string `json:"default_theme_font"`
	DefaultThemeRadius       string `json:"default_theme_radius"`
	DefaultThemeScale        string `json:"default_theme_scale"`
	DefaultSidebarVariant    string `json:"default_sidebar_variant"`
	DefaultSidebarLayout     string `json:"default_sidebar_layout"`
	DefaultContentLayout     string `json:"default_content_layout"`
	DefaultDirection         string `json:"default_direction"`
	ModelSquareDefaultView   string `json:"model_square_default_view"`
	ModelSquareCardPageSize  int    `json:"model_square_card_page_size"`
	ModelSquareTablePageSize int    `json:"model_square_table_page_size"`
	ModelSquareVisibleGroups string `json:"model_square_visible_groups"`
	GroupAccessRules         string `json:"group_access_rules"`
	HomepageStyle            string `json:"homepage_style"`
	HomepagePresetTitleMode  string `json:"homepage_preset_title_mode"`
	HomepagePresetSLAEnabled bool   `json:"homepage_preset_sla_enabled"`
	HomepagePresetSLAText    string `json:"homepage_preset_sla_text"`
	SPAMetaDescription       string `json:"spa_meta_description"`
	SPAMetaOGType            string `json:"spa_meta_og_type"`
	SPAMetaOGDescription     string `json:"spa_meta_og_description"`
	HideUpstreamRequestID    bool   `json:"hide_upstream_request_id"`

	// HideModelsWithoutEnabledChannel drops a group from the model square when
	// none of its channels can serve it, so the picker and the list never
	// advertise a group that is entirely disabled. On by default: a disabled
	// group is a configuration state, not something a visitor should have to
	// interpret.
	HideModelsWithoutEnabledChannel bool `json:"hide_models_without_enabled_channel"`

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
	ApiInfo:                         "",
	UptimeKumaGroups:                "",
	Announcements:                   "",
	FAQ:                             "",
	ApiInfoEnabled:                  true,
	UptimeKumaEnabled:               true,
	AnnouncementsEnabled:            true,
	FAQEnabled:                      true,
	BackgroundBlurOpacity:           40,
	LegalBackgroundEnabled:          true,
	DefaultTheme:                    "system",
	DefaultThemeOverride:            "none",
	DefaultThemePreset:              "default",
	DefaultThemeFont:                "default",
	DefaultThemeRadius:              "default",
	DefaultThemeScale:               "default",
	DefaultSidebarVariant:           "inset",
	DefaultSidebarLayout:            "expanded",
	DefaultContentLayout:            "full",
	DefaultDirection:                "ltr",
	ModelSquareDefaultView:          "card",
	ModelSquareCardPageSize:         18,
	ModelSquareTablePageSize:        20,
	ModelSquareVisibleGroups:        "[]",
	GroupAccessRules:                "[]",
	HomepageStyle:                   "default",
	HomepagePresetTitleMode:         "i18n",
	HomepagePresetSLAEnabled:        true,
	HomepagePresetSLAText:           "99% SLA guarantee",
	SPAMetaDescription:              "Unified AI API gateway and admin dashboard.",
	SPAMetaOGType:                   "website",
	SPAMetaOGDescription:            "Unified AI API gateway and admin dashboard.",
	HideUpstreamRequestID:           false,
	HideModelsWithoutEnabledChannel: true,
	// 默认镜像上游 S3AI 的公开状态页
	UpstreamStatusUrl:     "https://status.s3.fan/status/s3ai-api",
	UpstreamStatusName:    "上游状态",
	UpstreamStatusEnabled: true,

	SupportQqUrl:     "",
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

type AppearanceSetting struct {
	BackgroundImage          string `json:"background_image"`
	BackgroundRandomCategory string `json:"background_random_category"`
	BackgroundUnsplashQuery  string `json:"background_unsplash_query"`
	BackgroundBlurOpacity    int    `json:"background_blur_opacity"`
	LegalBackgroundEnabled   bool   `json:"legal_background_enabled"`
	DefaultTheme             string `json:"default_theme"`
	DefaultThemeOverride     string `json:"default_theme_override"`
	DefaultThemePreset       string `json:"default_theme_preset"`
	DefaultThemeFont         string `json:"default_theme_font"`
	DefaultThemeRadius       string `json:"default_theme_radius"`
	DefaultThemeScale        string `json:"default_theme_scale"`
	DefaultSidebarVariant    string `json:"default_sidebar_variant"`
	DefaultSidebarLayout     string `json:"default_sidebar_layout"`
	DefaultContentLayout     string `json:"default_content_layout"`
	DefaultDirection         string `json:"default_direction"`
	ModelSquareDefaultView   string `json:"model_square_default_view"`
	ModelSquareCardPageSize  int    `json:"model_square_card_page_size"`
	ModelSquareTablePageSize int    `json:"model_square_table_page_size"`
}

type SPAMetaSetting struct {
	Description   string `json:"description"`
	OGType        string `json:"og_type"`
	OGDescription string `json:"og_description"`
}

type HomepageSetting struct {
	Style            string `json:"style"`
	PresetTitleMode  string `json:"preset_title_mode"`
	PresetSLAEnabled bool   `json:"preset_sla_enabled"`
	PresetSLAText    string `json:"preset_sla_text"`
}

func GetHomepageSetting() HomepageSetting {
	return HomepageSetting{
		Style:            consoleSetting.HomepageStyle,
		PresetTitleMode:  consoleSetting.HomepagePresetTitleMode,
		PresetSLAEnabled: consoleSetting.HomepagePresetSLAEnabled,
		PresetSLAText:    consoleSetting.HomepagePresetSLAText,
	}
}

func GetAppearanceSetting() AppearanceSetting {
	return AppearanceSetting{
		BackgroundImage:          consoleSetting.BackgroundImage,
		BackgroundRandomCategory: consoleSetting.BackgroundRandomCategory,
		BackgroundUnsplashQuery:  consoleSetting.BackgroundUnsplashQuery,
		BackgroundBlurOpacity:    consoleSetting.BackgroundBlurOpacity,
		LegalBackgroundEnabled:   consoleSetting.LegalBackgroundEnabled,
		DefaultTheme:             consoleSetting.DefaultTheme,
		DefaultThemeOverride:     consoleSetting.DefaultThemeOverride,
		DefaultThemePreset:       consoleSetting.DefaultThemePreset,
		DefaultThemeFont:         consoleSetting.DefaultThemeFont,
		DefaultThemeRadius:       consoleSetting.DefaultThemeRadius,
		DefaultThemeScale:        consoleSetting.DefaultThemeScale,
		DefaultSidebarVariant:    consoleSetting.DefaultSidebarVariant,
		DefaultSidebarLayout:     consoleSetting.DefaultSidebarLayout,
		DefaultContentLayout:     consoleSetting.DefaultContentLayout,
		DefaultDirection:         consoleSetting.DefaultDirection,
		ModelSquareDefaultView:   consoleSetting.ModelSquareDefaultView,
		ModelSquareCardPageSize:  consoleSetting.ModelSquareCardPageSize,
		ModelSquareTablePageSize: consoleSetting.ModelSquareTablePageSize,
	}
}

func GetSPAMetaSetting() SPAMetaSetting {
	return SPAMetaSetting{
		Description:   consoleSetting.SPAMetaDescription,
		OGType:        consoleSetting.SPAMetaOGType,
		OGDescription: consoleSetting.SPAMetaOGDescription,
	}
}

var ogTypePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._:-]{0,63}$`)

func validatePublicImageURL(label, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if len(value) > 2048 {
		return fmt.Errorf("%s URL is too long", label)
	}
	if strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//") {
		if _, err := url.ParseRequestURI(value); err == nil {
			return nil
		}
		return fmt.Errorf("%s contains invalid URL characters", label)
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%s must use an absolute HTTP(S) URL or a root-relative path", label)
	}
	return nil
}

// validateUnsplashQuery accepts a free-text search term for Unsplash. Empty
// disables the remote lookup.
func validateUnsplashQuery(value string) error {
	value = strings.TrimSpace(value)
	if len(value) > 120 {
		return fmt.Errorf("unsplash query is too long")
	}
	return nil
}

// validateUnsplashAccessKey accepts an Unsplash application access key. It is a
// URL-safe token, so the charset is restricted rather than free-form.
func validateUnsplashAccessKey(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if len(value) > 128 {
		return fmt.Errorf("unsplash access key is too long")
	}
	for _, r := range value {
		isAllowed := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_'
		if !isAllowed {
			return fmt.Errorf("unsplash access key contains invalid characters")
		}
	}
	return nil
}

// GetUnsplashAccessKey returns the configured access key. It is deliberately
// absent from AppearanceSetting so the value never reaches the browser.
func GetUnsplashAccessKey() string {
	return strings.TrimSpace(consoleSetting.BackgroundUnsplashAccessKey)
}

// GetUnsplashQuery returns the configured Unsplash search term, if any.
func GetUnsplashQuery() string {
	return strings.TrimSpace(consoleSetting.BackgroundUnsplashQuery)
}

// validateBackgroundRandomCategory accepts a Nekosia image category slug. Empty
// disables the random backdrop, so the bundled pool is used instead.
func validateBackgroundRandomCategory(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if len(value) > 64 {
		return fmt.Errorf("background random category is too long")
	}
	for _, r := range value {
		isAllowed := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-'
		if !isAllowed {
			return fmt.Errorf("background random category may only contain lowercase letters, digits and dashes")
		}
	}
	return nil
}

func validateEnum(value string, allowed ...string) error {
	for _, item := range allowed {
		if value == item {
			return nil
		}
	}
	return fmt.Errorf("invalid value %q", value)
}

// ValidatePublicOption validates options that are consumed by the public SPA.
// Unknown keys are intentionally ignored so older deployments can add fields
// without making this validator a compatibility bottleneck.
func ValidatePublicOption(key, value string) error {
	if key == "Logo" {
		return validatePublicImageURL("logo", value)
	}
	if !strings.HasPrefix(key, "console_setting.") {
		return nil
	}
	name := strings.TrimPrefix(key, "console_setting.")
	switch name {
	case "background_image":
		return validatePublicImageURL("background image", value)
	case "background_random_category":
		return validateBackgroundRandomCategory(value)
	case "background_unsplash_query":
		return validateUnsplashQuery(value)
	case "background_unsplash_access_key":
		return validateUnsplashAccessKey(value)
	case "background_blur_opacity":
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || parsed < 0 || parsed > 100 {
			return fmt.Errorf("background blur opacity must be between 0 and 100")
		}
	case "legal_background_enabled":
		return validateEnum(value, "true", "false")
	case "default_theme":
		return validateEnum(value, "system", "light", "dark")
	case "default_theme_override":
		return validateEnum(value, "none", "light", "dark")
	case "default_theme_preset":
		return validateEnum(value, "default", "anthropic", "simple-large", "underground", "rose-garden", "lake-view", "sunset-glow", "forest-whisper", "ocean-breeze", "lavender-dream")
	case "default_theme_font":
		return validateEnum(value, "default", "sans", "serif")
	case "default_theme_radius":
		return validateEnum(value, "default", "none", "sm", "md", "lg", "xl")
	case "default_theme_scale":
		return validateEnum(value, "default", "sm", "lg", "xl")
	case "default_sidebar_variant":
		return validateEnum(value, "inset", "floating", "sidebar")
	case "default_sidebar_layout":
		return validateEnum(value, "expanded", "icon", "offcanvas")
	case "default_content_layout":
		return validateEnum(value, "full", "centered")
	case "default_direction":
		return validateEnum(value, "ltr", "rtl")
	case "model_square_default_view":
		return validateEnum(value, "card", "table")
	case "model_square_card_page_size":
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || parsed < 6 || parsed > 96 || parsed%6 != 0 {
			return fmt.Errorf("model square card page size must be a multiple of 6 between 6 and 96")
		}
	case "model_square_table_page_size":
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || parsed < 5 || parsed > 100 {
			return fmt.Errorf("model square table page size must be between 5 and 100")
		}
	case "homepage_style":
		return validateEnum(value, "default", "custom", "preset-1")
	case "homepage_preset_title_mode":
		return validateEnum(value, "i18n", "english")
	case "homepage_preset_sla_text":
		if len(value) > 120 {
			return fmt.Errorf("homepage SLA text is too long")
		}
	case "spa_meta_description", "spa_meta_og_description":
		if len(value) > 500 {
			return fmt.Errorf("SPA meta description is too long")
		}
	case "spa_meta_og_type":
		if !ogTypePattern.MatchString(value) {
			return fmt.Errorf("invalid Open Graph type")
		}
	}
	return nil
}
