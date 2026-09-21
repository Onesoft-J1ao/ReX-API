package controller

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// flexibleProbeMode installs the StatusCheckFlexibleMode document, which is
// what the console writes and what the probe paths read. It returns the map so
// a test can enable groups or change a group's pacing in place, and restores
// the previous document on cleanup.
func flexibleProbeMode(t *testing.T) map[string]any {
	t.Helper()
	previous := common.StatusCheckFlexibleMode
	t.Cleanup(func() { common.StatusCheckFlexibleMode = previous })

	groups := map[string]any{}
	encoded, err := common.Marshal(map[string]any{"groups": groups})
	require.NoError(t, err)
	common.StatusCheckFlexibleMode = string(encoded)
	return groups
}

// enableFlexibleProbe turns flexible pacing on for one group with the given
// pacing values, writing through the same document the console edits.
func enableFlexibleProbe(t *testing.T, groups map[string]any, group string, idleMinutes, maxProbes int) {
	t.Helper()
	groups[group] = map[string]any{
		"enabled":                true,
		"idle_minutes":           idleMinutes,
		"max_consecutive_probes": maxProbes,
	}
	encoded, err := common.Marshal(map[string]any{"groups": groups})
	require.NoError(t, err)
	common.StatusCheckFlexibleMode = string(encoded)
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
	// The activity map is process-wide, so clear whatever a previous test left
	// behind and clear what this one writes.
	perfmetrics.ResetStatusCheckPassiveActivityForTest()
	t.Cleanup(func() {
		common.RedisEnabled = previousRedis
		perfmetrics.ResetStatusCheckPassiveActivityForTest()
	})
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
	flexibleProbeMode(t) // no groups enabled

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
	enableFlexibleProbe(t, flexibleProbeMode(t), "default", 15, 3)

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
	groups := flexibleProbeMode(t)
	enableFlexibleProbe(t, groups, "probed", 15, 3)
	enableFlexibleProbe(t, groups, "serving", 15, 3)

	now := time.Now().Unix()
	require.NoError(t, model.InitializeStatusCheckProbeState("probed", now-5*60))
	require.NoError(t, model.InitializeStatusCheckProbeState("serving", now-45*60))
	require.NoError(t, model.TouchStatusCheckProbePassiveActivity("serving", now-5*60))

	selected, err := selectFlexibleProbeGroups([]string{"probed", "serving"}, now)

	require.NoError(t, err)
	assert.Empty(t, selected, "a recently probed or recently serving group needs no probe")
}

func TestSelectFlexibleProbeGroupsProbesAQuietGroup(t *testing.T) {
	setupFlexibleProbeTest(t)
	enableFlexibleProbe(t, flexibleProbeMode(t), "default", 15, 3)

	now := time.Now().Unix()
	require.NoError(t, model.InitializeStatusCheckProbeState("default", now-16*60))

	groups, err := selectFlexibleProbeGroups([]string{"default"}, now)

	require.NoError(t, err)
	assert.Equal(t, []string{"default"}, groups)
}

func TestSelectFlexibleProbeGroupsStopsAtTheConsecutiveCap(t *testing.T) {
	setupFlexibleProbeTest(t)
	enableFlexibleProbe(t, flexibleProbeMode(t), "default", 15, 3)

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
	enableFlexibleProbe(t, flexibleProbeMode(t), "default", 15, 3)

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

// An out-of-range document is rejected outright rather than silently clamped:
// a typo must not quietly change the probe cadence, and a rejected document
// leaves the feature off, which is the safe direction to fail.
func TestSelectFlexibleProbeGroupsRejectsOutOfRangeSettings(t *testing.T) {
	setupFlexibleProbeTest(t)
	groups := flexibleProbeMode(t)
	groups["default"] = map[string]any{
		"enabled":                true,
		"idle_minutes":           -5,
		"max_consecutive_probes": 0,
	}
	encoded, err := common.Marshal(map[string]any{"groups": groups})
	require.NoError(t, err)
	common.StatusCheckFlexibleMode = string(encoded)

	_, err = perfmetrics.ParseStatusCheckFlexibleConfig(common.StatusCheckFlexibleMode)
	require.Error(t, err, "an out-of-range idle_minutes must be rejected")

	// GetStatusCheckFlexibleConfig swallows the error and yields the empty
	// default, so nothing is paced and every group keeps the ordinary cadence.
	now := time.Now().Unix()
	selected, err := selectFlexibleProbeGroups([]string{"default"}, now)
	require.NoError(t, err)
	assert.Equal(t, []string{"default"}, selected)
}

// The relay path reports activity on every successful request, so the reporting
// has to be a no-op unless flexible probing actually needs it.
func TestNoteStatusCheckPassiveActivityRecordsOnlyTrackedGroups(t *testing.T) {
	setupFlexibleProbeTest(t)
	groups := flexibleProbeMode(t) // nothing enabled yet

	perfmetrics.NoteStatusCheckPassiveActivity("default")
	_, recorded := probeStateFor(t, "default")
	assert.False(t, recorded, "a disabled feature must not write pacing state")

	// Enabled for a different group: this one is still untracked.
	enableFlexibleProbe(t, groups, "go-plan", 15, 3)
	perfmetrics.NoteStatusCheckPassiveActivity("default")
	_, recorded = probeStateFor(t, "default")
	assert.False(t, recorded, "an untracked group's traffic is not recorded")

	enableFlexibleProbe(t, groups, "default", 15, 3)
	perfmetrics.NoteStatusCheckPassiveActivity("default")
	state, recorded := probeStateFor(t, "default")
	require.True(t, recorded, "a tracked group's traffic is recorded")
	assert.Greater(t, state.LastPassiveAt, int64(0))
	assert.Greater(t, perfmetrics.LatestStatusCheckPassiveActivity("default", 0), int64(0))
}
