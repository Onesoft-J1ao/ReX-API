package channel_status_sync_setting

import (
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

// ChannelStatusSyncSetting configures the job that pulls an external status page
// (an Uptime Kuma instance) and reflects remote monitor state onto local channels.
//
// The remote page is an untrusted third party: every field here exists to keep a
// remote answer from mass-disabling the gateway. Aliases make matching explicit,
// the staleness window refuses to act on a frozen page, and MaxDisableFraction
// caps how much of the fleet one response may disable.
type ChannelStatusSyncSetting struct {
	Enabled             bool    `json:"enabled"`
	SourceURL           string  `json:"source_url"`            // status page URL, e.g. https://status.example.com/status/default
	SyncIntervalMinutes int     `json:"sync_interval_minutes"` // scheduled job cadence
	Aliases             string  `json:"aliases"`               // "remote monitor name=local channel name", comma/newline separated
	EnableOnUp          bool    `json:"enable_on_up"`          // re-enable channels this job auto-disabled once the monitor recovers
	DisableOnDown       bool    `json:"disable_on_down"`       // auto-disable channels whose monitor reports down
	StaleAfterMinutes   int     `json:"stale_after_minutes"`   // 0 disables the freshness check
	MaxDisableFraction  float64 `json:"max_disable_fraction"`  // refuse a change set disabling more than this fraction of channels
}

var channelStatusSyncSetting = ChannelStatusSyncSetting{
	Enabled:             false,
	SourceURL:           "https://status.example.com/status/default",
	SyncIntervalMinutes: 5,
	Aliases:             "",
	EnableOnUp:          true,
	DisableOnDown:       true,
	StaleAfterMinutes:   30,
	MaxDisableFraction:  0.5,
}

func init() {
	config.GlobalConfig.Register("channel_status_sync_setting", &channelStatusSyncSetting)
}

func GetSetting() ChannelStatusSyncSetting { return channelStatusSyncSetting }

func GetSyncIntervalMinutes() int {
	if channelStatusSyncSetting.SyncIntervalMinutes < 1 {
		return 5
	}
	return channelStatusSyncSetting.SyncIntervalMinutes
}

func GetStaleAfterMinutes() int {
	if channelStatusSyncSetting.StaleAfterMinutes < 0 {
		return 0
	}
	return channelStatusSyncSetting.StaleAfterMinutes
}

// GetMaxDisableFraction clamps to (0, 1]. A zero/negative/huge value would either
// block every sync or remove the guard entirely, so it falls back to the default.
func GetMaxDisableFraction() float64 {
	f := channelStatusSyncSetting.MaxDisableFraction
	if f <= 0 || f > 1 {
		return 0.5
	}
	return f
}

// GetAliases parses the alias list into a lowercase remote-name -> local-name map.
// Entries without a "=" are ignored rather than guessed at: an unparsable alias
// must not silently become a match rule.
func GetAliases() map[string]string {
	raw := strings.TrimSpace(channelStatusSyncSetting.Aliases)
	if raw == "" {
		return nil
	}
	aliases := map[string]string{}
	for _, entry := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r'
	}) {
		remote, local, found := strings.Cut(entry, "=")
		if !found {
			continue
		}
		remote = strings.ToLower(strings.TrimSpace(remote))
		local = strings.TrimSpace(local)
		if remote == "" || local == "" {
			continue
		}
		aliases[remote] = local
	}
	if len(aliases) == 0 {
		return nil
	}
	return aliases
}
