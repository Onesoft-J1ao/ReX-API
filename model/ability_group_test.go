package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A group whose channels are all disabled must not be reported as servable, or
// the model square keeps advertising a group that can only fail.
func TestGetGroupsWithEnabledChannelSkipsFullyDisabledGroups(t *testing.T) {
	truncateTables(t)

	require.NoError(t, DB.Create(&Ability{Group: "open", Model: "m1", ChannelId: 1, Enabled: true}).Error)
	require.NoError(t, DB.Create(&Ability{Group: "open", Model: "m2", ChannelId: 2, Enabled: true}).Error)
	require.NoError(t, DB.Create(&Ability{Group: "closed", Model: "m3", ChannelId: 3, Enabled: false}).Error)
	// One disabled channel does not close a group that another channel serves.
	require.NoError(t, DB.Create(&Ability{Group: "mixed", Model: "m4", ChannelId: 4, Enabled: false}).Error)
	require.NoError(t, DB.Create(&Ability{Group: "mixed", Model: "m5", ChannelId: 5, Enabled: true}).Error)

	groups, err := GetGroupsWithEnabledChannel()

	require.NoError(t, err)
	assert.Contains(t, groups, "open")
	assert.Contains(t, groups, "mixed")
	assert.NotContains(t, groups, "closed")
}

func TestGetGroupsWithEnabledChannelIsEmptyWithoutAbilities(t *testing.T) {
	truncateTables(t)

	groups, err := GetGroupsWithEnabledChannel()

	require.NoError(t, err)
	assert.Empty(t, groups)
}
