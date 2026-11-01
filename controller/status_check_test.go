package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A probe is a real upstream request. Probing a paid group therefore bills us on
// every cycle for a signal the mirrored upstream already publishes, so the group
// filter is the mechanism that keeps the probe budget off pay-per-request models.
func TestProbeGroupsForSkipsPaidGroups(t *testing.T) {
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"group_ratio_setting.group_ratio":             `{"free-group":0,"paid-group":1}`,
		"status_check_setting.probe_free_models_only": "true",
		"ModelPrice": `{"priced-model":0.01}`,
		"ModelRatio": `{"paid-ratio-model":2}`,
	}))

	groups := []string{"free-group", "paid-group"}

	assert.Equal(t, []string{"free-group"},
		probeGroupsFor(groups, "paid-ratio-model"),
		"a channel serving both groups is still probed for its free one")

	assert.Equal(t, []string{"free-group"},
		probeGroupsFor(groups, "priced-model"),
		"the zero-ratio group stays probeable even for a priced model, since that probe is free")
}

// With no free group in play, a priced model is not probed at all.
func TestProbeGroupsForDropsEveryGroupForPaidModel(t *testing.T) {
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"group_ratio_setting.group_ratio":             `{"paid-group":1}`,
		"status_check_setting.probe_free_models_only": "true",
		"ModelPrice": `{"priced-model":0.01}`,
	}))

	assert.Empty(t, probeGroupsFor([]string{"paid-group"}, "priced-model"),
		"a paid group with a priced model costs money on every probe and must be skipped")
}

// Turning the switch off restores the previous behaviour, so the change can be
// rolled back from the settings page without a redeploy.
func TestProbeGroupsForKeepsAllGroupsWhenDisabled(t *testing.T) {
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"group_ratio_setting.group_ratio":             `{"paid-group":1}`,
		"status_check_setting.probe_free_models_only": "false",
	}))

	groups := []string{"paid-group"}
	assert.Equal(t, groups, probeGroupsFor(groups, "priced-model"),
		"with the switch off every configured group is probed as before")
}

// A group ratio of zero makes the probe free regardless of the model, which is
// the cheapest case and must never be filtered away.
func TestProbeGroupsForKeepsFreeGroupForAnyModel(t *testing.T) {
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"group_ratio_setting.group_ratio":             `{"free-group":0}`,
		"status_check_setting.probe_free_models_only": "true",
		"ModelPrice": `{"expensive-model":99}`,
	}))

	assert.Equal(t, []string{"free-group"},
		probeGroupsFor([]string{"free-group"}, "expensive-model"))
}

// The number on a card and the strip under it come from the same requests, so
// they must agree. Averaging the probe 50/50 against the passive rollup broke
// that: one failed probe turned 17 successful requests out of 17 into "50%",
// with an all-green strip directly above it. The probe is one sample, so it
// weighs one sample.
func TestMergeAvailabilityWeightsOneProbeAgainstObservedTraffic(t *testing.T) {
	// 17 requests, all successful, and the probe failed.
	assert.InDelta(t, 94.44, mergeAvailability(0, 100, 17), 0.01,
		"a single failed probe must not halve a group that served 17 requests successfully")

	// Same traffic, probe succeeded: the probe can only pull the figure up a
	// little, never to 100% over real failures.
	assert.InDelta(t, 52.78, mergeAvailability(100, 50, 17), 0.01,
		"a healthy probe does not erase failures the group actually served")

	// No traffic at all leaves the probe as the only evidence.
	assert.Equal(t, 0.0, mergeAvailability(0, -1, 0),
		"with no passive traffic the probe result is the whole answer")
	assert.Equal(t, 100.0, mergeAvailability(100, -1, 0))

	// Traffic with no probe still reports the traffic.
	assert.InDelta(t, 81.74, mergeAvailability(0, 81.96, 377), 0.01)
}

// A failed probe measures how fast the failure came back, not the model's
// response time: a request rejected before it reaches the upstream (an
// unconfigured price, a disabled channel) answers in a couple of milliseconds.
// Reporting that duration put "1ms" and "45ms" on cards for models that never
// answered at all.
func TestMergeHealthRowDoesNotReportAFailedProbesDuration(t *testing.T) {
	noTraffic := map[string]model.PerfMetricSummaryByGroup{}

	failed := mergeHealthRow(model.ModelHealth{
		ModelName: "slow-model",
		Ok:        false,
		LatencyMs: 45,
	}, noTraffic, "g")

	assert.Equal(t, int64(0), failed.LatencyMs,
		"a probe that failed measured the failure, not the model")

	ok := mergeHealthRow(model.ModelHealth{
		ModelName: "slow-model",
		Ok:        true,
		LatencyMs: 45,
	}, noTraffic, "g")

	assert.Equal(t, int64(45), ok.LatencyMs,
		"a successful probe is a real round trip and stays")

	// Once the group carries traffic, that traffic describes what a caller in
	// this group actually waits for better than one probe does.
	withTraffic := map[string]model.PerfMetricSummaryByGroup{
		"g|slow-model": {
			ModelName: "slow-model", Group: "g",
			RequestCount: 4, SuccessCount: 4, TotalLatencyMs: 8000,
		},
	}
	traffic := mergeHealthRow(model.ModelHealth{
		ModelName: "slow-model", Ok: true, LatencyMs: 45,
	}, withTraffic, "g")

	assert.Equal(t, int64(2000), traffic.LatencyMs,
		"the group's own requests describe the group's latency better than one probe")
}

// The status page reads probe rows and passive metrics, and both of those
func TestModelIsAdvertisedFollowsThePlazaAndTheExclusionList(t *testing.T) {
	served := map[string]bool{"served-model": true, "excluded-model": true}
	excluded := map[string]struct{}{"excluded-model": {}}

	assert.True(t, modelIsAdvertised(served, "served-model", nil))

	assert.False(t, modelIsAdvertised(served, "retired-model", nil),
		"a model no enabled channel serves is not on the plaza, so the page drops it")

	assert.False(t, modelIsAdvertised(served, "excluded-model", excluded),
		"the admin exclusion list still hides a model that is served")

	assert.True(t, modelIsAdvertised(nil, "retired-model", nil),
		"an empty ability set is unknown rather than a verdict, so nothing is dropped")
}
