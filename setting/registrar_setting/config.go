package registrar_setting

import (
	"strings"
	"time"

	"github.com/QuantumNous/new-api/setting/config"
)

// Provider is one registration source and the channel its keys feed. The
// framework is multi-site: each provider names its own runner script, its own
// destination channel, and its own replenishment policy, so adding a site means
// adding a script and an entry here rather than new code.
type Provider struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// Enabled gates both the scheduled replenishment and the manual run button.
	Enabled bool `json:"enabled"`
	// ScriptPath is an absolute path inside the gateway container. It comes from
	// server-side settings, never from a request, so it cannot be pointed at an
	// arbitrary binary by an API caller.
	ScriptPath string `json:"script_path"`
	// ChannelID is the multi-key channel that receives the registered keys.
	ChannelID int `json:"channel_id"`
	// MinKeys is the usable-key floor: below it, replenishment registers more.
	MinKeys int `json:"min_keys"`
	// TargetKeys is how many usable keys a replenishment run aims for.
	TargetKeys int `json:"target_keys"`
	// MaxBatch caps how many accounts one run may register, so a bad threshold
	// cannot turn into an unbounded registration burst.
	MaxBatch int `json:"max_batch"`
	// Workers is the runner's concurrency.
	Workers int `json:"workers"`
	// UseProxy asks the runner to route through its proxy pool. The gateway
	// registers from its own address, so this is off by default.
	UseProxy bool `json:"use_proxy"`
}

type RegistrarSetting struct {
	Enabled bool `json:"enabled"`
	// IntervalMinutes is how often the replenishment check wakes. Each provider's
	// floor and target decide whether a wake actually registers anything.
	IntervalMinutes int        `json:"interval_minutes"`
	Providers       []Provider `json:"providers"`
}

var registrarSetting = RegistrarSetting{
	Enabled:         true,
	IntervalMinutes: 10,
	Providers: []Provider{
		{
			ID:         "dahl",
			Label:      "Dahl Inference",
			Enabled:    true,
			ScriptPath: "/opt/registrar/dahl/runner.py",
			ChannelID:  30,
			MinKeys:    50,
			TargetKeys: 200,
			MaxBatch:   100,
			Workers:    8,
			UseProxy:   false,
		},
	},
}

func init() {
	config.GlobalConfig.Register("registrar_setting", &registrarSetting)
}

func GetSetting() RegistrarSetting {
	return registrarSetting
}

func GetProvider(id string) (Provider, bool) {
	id = strings.TrimSpace(id)
	for _, provider := range registrarSetting.Providers {
		if provider.ID == id {
			return provider, true
		}
	}
	return Provider{}, false
}

func GetInterval() time.Duration {
	minutes := registrarSetting.IntervalMinutes
	if minutes < 1 {
		minutes = 10
	}
	return time.Duration(minutes) * time.Minute
}

// MissingFor returns how many keys a provider is short of its target, clamped to
// MaxBatch. Zero means the provider is at or above its floor and needs nothing.
func (p Provider) MissingFor(usableKeys int) int {
	if !p.Enabled || p.TargetKeys <= 0 || usableKeys >= p.MinKeys {
		return 0
	}
	missing := p.TargetKeys - usableKeys
	if missing <= 0 {
		return 0
	}
	if p.MaxBatch > 0 && missing > p.MaxBatch {
		missing = p.MaxBatch
	}
	return missing
}
