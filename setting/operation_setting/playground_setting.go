package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

// PlaygroundSetting covers the playground workbench: the free allowance a
// visitor gets, where that allowance is accounted, and which origins may call
// the playground endpoints at all.
type PlaygroundSetting struct {
	Enabled bool `json:"enabled"`

	// Free allowance. A request is served from it only while both budgets still
	// have room, so whichever budget runs out first ends the free tier. Zero
	// disables that budget; both zero disables the free tier.
	FreeGenerations int   `json:"free_generations"`
	FreeTokens      int64 `json:"free_tokens"`

	// Account the allowance per person rather than per account: one browser
	// opening several accounts would otherwise claim the allowance once each.
	// The fingerprint comes from the client and falls back to the caller IP.
	DedupeByIP bool `json:"dedupe_by_ip"`

	// Playground requests must come from a browser on this site. A third-party
	// page embedding the playground is rejected, which is what stops a reverse
	// proxy from spending other people's allowances.
	RequireBrowserOrigin bool     `json:"require_browser_origin"`
	AllowedOrigins       []string `json:"allowed_origins"`
}

var playgroundSetting = PlaygroundSetting{
	Enabled:              true,
	FreeGenerations:      5,
	FreeTokens:           1000,
	DedupeByIP:           true,
	RequireBrowserOrigin: true,
	AllowedOrigins:       []string{},
}

func init() {
	config.GlobalConfig.Register("playground_setting", &playgroundSetting)
}

func GetPlaygroundSetting() *PlaygroundSetting {
	return &playgroundSetting
}
