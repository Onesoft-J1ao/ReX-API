package controller

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/status_check_setting"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// flexibleProbeSetting exposes the registered settings struct so a test can set
// pacing values directly. Get returns the pointer that was registered, and the
// copy is restored on cleanup.
func flexibleProbeSetting(t *testing.T) *status_check_setting.StatusCheckSetting {
	t.Helper()
	setting, ok := config.GlobalConfig.Get("status_check_setting").(*status_check_setting.StatusCheckSetting)
	require.True(t, ok, "status_check_setting must be registered as a pointer")
	previous := *setting
	t.Cleanup(func() { *setting = previous })
	return setting
}

func setupFlexibleProbeTest(t *testing.T) {
	t.Helper()
	// Through the shared helper so the dialect quoting for the `group` column is
	// initialised, exactly as it is at startup.
	db := modelManagementDB(t, "sqlite", "")
	require.NoError(t, db.AutoMigrate(&model.StatusCheckProbeState{}))

	// The durable row is what the assertions read; keep Redis out of the path so
	// the test does not depend on a server being reachable.
	previousRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previousRedis })
}

func probeStateFor(t *testing.T, group string) (model.StatusCheckProbeState, bool) {
	t.Helper()
	states, err := model.GetStatusCheckProbeStates([]string{group})
	require.NoError(t, err)
	state, ok := states[group]
	return state, ok
}

func TestSelectFlexibleProbeGroupsLeavesEverythingAloneWhenDisabled(t *testing.T) {
	setupFlexibleProbeTest(t)
	setting := flexibleProbeSetting(t)
	setting.FlexibleProbingEnabled = false

	groups, err := selectFlexibleProbeGroups([]string{"default", "go-plan"}, time.Now().Unix())

	require.NoError(t, err)
	assert.Equal(t, []string{"default", "go-plan"}, groups)
	// Nothing may be written when the feature is off, so a later switch to it
	// cannot find stale state.
	states, err := model.GetStatusCheckProbeStates([]string{"default", "go-plan"})
	require.NoError(t, err)
	assert.Empty(t, states)
}

func TestSelectFlexibleProbeGroupsOpensTheIdleWindowOnFirstSight(t *testing.T) {
	setupFlexibleProbeTest(t)
	setting := flexibleProbeSetting(t)
	setting.FlexibleProbingEnabled = true
	setting.FlexibleIdleMinutes = 15
	setting.FlexibleMaxConsecutiveProbes = 3

	now := time.Now().Unix()
	groups, err := selectFlexibleProbeGroups([]string{"default"}, now)

	require.NoError(t, err)
	// The first cycle only opens the window: switching the feature on must not
	// spend a probe immediately.
	assert.Empty(t, groups)

	state, ok := probeStateFor(t, "default")
	require.True(t, ok)
	assert.Equal(t, now, state.LastProbeAt)
}

func TestSelectFlexibleProbeGroupsSkipsAGroupThatJustServedOrWasProbed(t *testing.T) {
	setupFlexibleProbeTest(t)
	setting := flexibleProbeSetting(t)
	setting.FlexibleProbingEnabled = true
	setting.FlexibleIdleMinutes = 15
	setting.FlexibleMaxConsecutiveProbes = 3

	now := time.Now().Unix()
	require.NoError(t, model.InitializeStatusCheckProbeState("probed", now-5*60))
	require.NoError(t, model.InitializeStatusCheckProbeState("serving", now-45*60))
	require.NoError(t, model.TouchStatusCheckProbePassiveActivity("serving", now-5*60))

	groups, err := selectFlexibleProbeGroups([]string{"probed", "serving"}, now)

	require.NoError(t, err)
	assert.Empty(t, groups, "a recently probed or recently serving group needs no probe")
}

func TestSelectFlexibleProbeGroupsProbesAQuietGroup(t *testing.T) {
	setupFlexibleProbeTest(t)
	setting := flexibleProbeSetting(t)
	setting.FlexibleProbingEnabled = true
	setting.FlexibleIdleMinutes = 15
	setting.FlexibleMaxConsecutiveProbes = 3

	now := time.Now().Unix()
	require.NoError(t, model.InitializeStatusCheckProbeState("default", now-16*60))

	groups, err := selectFlexibleProbeGroups([]string{"default"}, now)

	require.NoError(t, err)
	assert.Equal(t, []string{"default"}, groups)
}

func TestSelectFlexibleProbeGroupsStopsAtTheConsecutiveCap(t *testing.T) {
	setupFlexibleProbeTest(t)
	setting := flexibleProbeSetting(t)
	setting.FlexibleProbingEnabled = true
	setting.FlexibleIdleMinutes = 15
	setting.FlexibleMaxConsecutiveProbes = 3

	now := time.Now().Unix()
	require.NoError(t, model.InitializeStatusCheckProbeState("default", now-60*60))
	// Three probe cycles with no traffic in between: the cap is reached.
	for range 3 {
		require.NoError(t, model.RecordStatusCheckProbeResult("default", now-16*60))
	}
	state, ok := probeStateFor(t, "default")
	require.True(t, ok)
	require.Equal(t, 3, state.ConsecutiveProbeCount)

	groups, err := selectFlexibleProbeGroups([]string{"default"}, now)
	require.NoError(t, err)
	assert.Empty(t, groups, "nothing has needed this group; stop paying to check it")
}

func TestSelectFlexibleProbeGroupsResetsTheStreakAfterRealTraffic(t *testing.T) {
	setupFlexibleProbeTest(t)
	setting := flexibleProbeSetting(t)
	setting.FlexibleProbingEnabled = true
	setting.FlexibleIdleMinutes = 15
	setting.FlexibleMaxConsecutiveProbes = 3

	now := time.Now().Unix()
	// Five consecutive probes put the group over the cap...
	require.NoError(t, model.InitializeStatusCheckProbeState("default", now-60*60))
	for range 5 {
		require.NoError(t, model.RecordStatusCheckProbeResult("default", now-60*60))
	}
	state, ok := probeStateFor(t, "default")
	require.True(t, ok)
	require.GreaterOrEqual(t, state.ConsecutiveProbeCount, 3)

	// ...but traffic 20 minutes ago is newer than the last probe (an hour ago),
	// so the streak resets and the group is probed again once it goes quiet.
	require.NoError(t, model.TouchStatusCheckProbePassiveActivity("default", now-20*60))

	groups, err := selectFlexibleProbeGroups([]string{"default"}, now)

	require.NoError(t, err)
	assert.Equal(t, []string{"default"}, groups,
		"traffic since the last probe restarts the streak, so the cap cannot mute a group that is in use")
}

func TestSelectFlexibleProbeGroupsFallsBackForOutOfRangeSettings(t *testing.T) {
	setupFlexibleProbeTest(t)
	setting := flexibleProbeSetting(t)
	setting.FlexibleProbingEnabled = true
	setting.FlexibleIdleMinutes = -5
	setting.FlexibleMaxConsecutiveProbes = 0

	assert.Equal(t, status_check_setting.DefaultFlexibleIdleMinutes, status_check_setting.GetFlexibleIdleMinutes())
	assert.Equal(t, status_check_setting.DefaultFlexibleMaxConsecutiveProbes, status_check_setting.GetFlexibleMaxConsecutiveProbes())
}

// The relay path reports activity on every successful request, so the reporting
// has to be a no-op unless flexible probing actually needs it.
func TestNoteStatusCheckPassiveActivityRecordsOnlyTrackedGroups(t *testing.T) {
	setupFlexibleProbeTest(t)
	setting := flexibleProbeSetting(t)
	setting.FlexibleProbingEnabled = false
	setting.Groups = "default"

	perfmetrics.NoteStatusCheckPassiveActivity("default")
	_, recorded := probeStateFor(t, "default")
	assert.False(t, recorded, "a disabled feature must not write pacing state")

	setting.FlexibleProbingEnabled = true
	setting.Groups = "go-plan"
	perfmetrics.NoteStatusCheckPassiveActivity("default")
	_, recorded = probeStateFor(t, "default")
	assert.False(t, recorded, "an unprobed group has nothing to pace")

	setting.Groups = "default"
	perfmetrics.NoteStatusCheckPassiveActivity("default")
	state, recorded := probeStateFor(t, "default")
	require.True(t, recorded, "a tracked group's traffic is recorded")
	assert.Greater(t, state.LastPassiveAt, int64(0))
	assert.Greater(t, perfmetrics.LatestStatusCheckPassiveActivity("default", 0), int64(0))
}
