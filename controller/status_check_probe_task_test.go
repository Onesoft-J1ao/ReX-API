package controller

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunStatusCheckProbeTaskRecordsOneProbeAndHonorsBackoff(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.StatusCheckProbeState{}))

	previousGroups := common.StatusCheckGroups
	previousFlexibleMode := common.StatusCheckFlexibleMode
	previousProbeTest := runStatusCheckProbeChannelTest
	previousRatios := ratio_setting.GetGroupRatioCopy()
	// A group is only probed when it is also a known group ratio, and other
	// tests in this package replace the ratio table, so set it here rather than
	// relying on whatever the previous test left behind.
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	// A group is only probed when it is both listed as a visible status group
	// and enabled in the pacing document, so the test must set both.
	common.StatusCheckGroups = `["default"]`
	common.StatusCheckFlexibleMode = `{"groups":{"default":{"enabled":true,"idle_minutes":1,"max_consecutive_probes":1}}}`
	runStatusCheckProbeChannelTest = func(context.Context, *model.Channel, int, string, string, bool) testResult {
		return testResult{}
	}
	t.Cleanup(func() {
		common.StatusCheckGroups = previousGroups
		common.StatusCheckFlexibleMode = previousFlexibleMode
		runStatusCheckProbeChannelTest = previousProbeTest
		encodedRatios, err := common.Marshal(previousRatios)
		if err == nil {
			_ = ratio_setting.UpdateGroupRatioByJSONString(string(encodedRatios))
		}
	})

	rootUser := &model.User{
		Username: "status-probe-root",
		Password: "unused",
		Role:     common.RoleRootUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}
	require.NoError(t, db.Create(rootUser).Error)
	channel := &model.Channel{
		Name:   "status-probe-channel",
		Key:    "status-probe-key",
		Group:  "default",
		Status: common.ChannelStatusEnabled,
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.StatusCheckProbeState{
		Group:       "default",
		LastProbeAt: time.Now().Add(-2 * time.Minute).Unix(),
	}).Error)

	summary, err := runStatusCheckProbeTask(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, summary.Tested)
	assert.Equal(t, 1, summary.Succeeded)

	var state model.StatusCheckProbeState
	require.NoError(t, db.First(&state, "`group` = ?", "default").Error)
	assert.Equal(t, 1, state.ConsecutiveProbeCount)

	state.LastProbeAt = time.Now().Add(-2 * time.Minute).Unix()
	require.NoError(t, db.Save(&state).Error)
	summary, err = runStatusCheckProbeTask(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, summary.SkippedBackoff)
	assert.Zero(t, summary.Tested)
}

func TestStatusCheckFlexibleProbeGroupsUsesPerGroupSettings(t *testing.T) {
	previousGroups := common.StatusCheckGroups
	previousFlexibleMode := common.StatusCheckFlexibleMode
	t.Cleanup(func() {
		common.StatusCheckGroups = previousGroups
		common.StatusCheckFlexibleMode = previousFlexibleMode
	})

	common.StatusCheckGroups = `["default"]`
	common.StatusCheckFlexibleMode = `{"groups":{"default":{"enabled":false,"idle_minutes":5,"max_consecutive_probes":3}}}`
	assert.Empty(t, statusCheckFlexibleProbeGroups())

	common.StatusCheckFlexibleMode = `{"groups":{"default":{"enabled":true,"idle_minutes":5,"max_consecutive_probes":3}}}`
	groups := statusCheckFlexibleProbeGroups()
	require.Len(t, groups, 1)
	assert.Equal(t, "default", groups[0].Group)
	assert.Equal(t, 5, groups[0].Config.IdleMinutes)
	assert.Equal(t, 3, groups[0].Config.MaxConsecutiveProbes)
}
