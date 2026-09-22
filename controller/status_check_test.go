package controller

import (
	"testing"

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
