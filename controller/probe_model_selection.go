package controller

import (
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// probeModelRank sorts a model into a cost tier for probing. Lower is
// preferred. The tiers separate classes of cost, not exact amounts, because
// within a class the deciding factor is speed, applied by the caller.
const (
	probeRankFree       = 0 // costs nothing to probe
	probeRankFlatPrice  = 1 // a fixed price per request, so the cost is known
	probeRankCheapRatio = 2 // token-billed, but cheap
	probeRankPaidRatio  = 3 // token-billed and expensive
	probeRankUnknown    = 4 // nothing known; probe only as a last resort
)

// pickProbeModel chooses which model a health probe should test on a channel.
//
// A probe is a real upstream request: it costs whatever the model costs and it
// occupies the channel for as long as the model takes to answer. So the order
// is free first, then cheap, then fast — a free model costs nothing, and among
// paid ones the cheapest is all a liveness check needs. Speed only breaks ties,
// because the signal is "did it answer", not "how well".
//
// A channel's configured test model still wins when set: the operator chose it
// deliberately, and the scheduled channel test already uses it.
func pickProbeModel(channel *model.Channel) string {
	if channel == nil {
		return ""
	}
	if channel.TestModel != nil {
		if configured := strings.TrimSpace(*channel.TestModel); configured != "" {
			return configured
		}
	}

	groups := channel.GetGroups()
	candidates := make([]string, 0, 8)
	for _, name := range channel.GetModels() {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			candidates = append(candidates, trimmed)
		}
	}
	if len(candidates) == 0 {
		return ""
	}

	// A stable order first, so two runs on the same channel agree when prices
	// and speeds tie.
	sort.Strings(candidates)
	sort.SliceStable(candidates, func(i, j int) bool {
		return probeModelRank(candidates[i], groups) < probeModelRank(candidates[j], groups)
	})
	return candidates[0]
}

// uncoveredProbeModels lists every model on a channel that the upstream mirror
// does not report, cheapest first. Those models have no mirrored health, so a
// probe is the only way to know whether they answer. covered reports whether
// the mirror already has the model; the caller supplies it so this package
// stays free of the metrics cache.
func uncoveredProbeModels(channel *model.Channel, covered func(string) bool) []string {
	if channel == nil || covered == nil {
		return nil
	}
	groups := channel.GetGroups()
	candidates := make([]string, 0, 8)
	for _, name := range channel.GetModels() {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" || covered(trimmed) {
			continue
		}
		candidates = append(candidates, trimmed)
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.Strings(candidates)
	sort.SliceStable(candidates, func(i, j int) bool {
		return probeModelRank(candidates[i], groups) < probeModelRank(candidates[j], groups)
	})
	return candidates
}
func probeModelRank(modelName string, groups []string) int {
	// Free in any of the channel's groups is free enough to prefer: the probe
	// would be billed to that group.
	for _, group := range groups {
		if ratio_setting.IsModelFreeInGroup(modelName, group) {
			return probeRankFree
		}
	}
	// A per-request price is a flat cost per probe; a token ratio scales with
	// the answer, which a liveness check cannot predict. Prefer the flat one.
	if _, hasPrice := ratio_setting.GetModelPrice(modelName, false); hasPrice {
		return probeRankFlatPrice
	}
	if ratio, exist, _ := ratio_setting.GetModelRatio(modelName); exist {
		if ratio <= 1 {
			return probeRankCheapRatio
		}
		return probeRankPaidRatio
	}
	return probeRankUnknown
}
