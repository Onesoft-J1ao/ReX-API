package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

// WallpaperSetting covers the paid custom console wallpaper: whether users may
// buy one at all, what one change costs, and how many changes a single
// calendar month allows.
type WallpaperSetting struct {
	Enabled bool `json:"enabled"`

	// PriceYuan is the price of one change in 元 (CNY). It is converted to
	// quota with the site's USD exchange rate, so it stays meaningful on
	// deployments that do not bill in CNY.
	PriceYuan float64 `json:"price_yuan"`

	// MonthlyChangeLimit caps paid changes per calendar month. Zero or a
	// negative value means unlimited, which the API reports as -1 remaining.
	MonthlyChangeLimit int `json:"monthly_change_limit"`
}

var wallpaperSetting = WallpaperSetting{
	Enabled:            true,
	PriceYuan:          0.5,
	MonthlyChangeLimit: 3,
}

func init() {
	config.GlobalConfig.Register("wallpaper_setting", &wallpaperSetting)
}

func GetWallpaperSetting() *WallpaperSetting {
	return &wallpaperSetting
}
