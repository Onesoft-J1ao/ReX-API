package registrar

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/registrar_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestProviderMissingForHonoursFloorTargetAndBatch(t *testing.T) {
	base := registrar_setting.Provider{Enabled: true, MinKeys: 50, TargetKeys: 200, MaxBatch: 100}

	cases := []struct {
		name   string
		usable int
		want   int
	}{
		{name: "at the floor needs nothing", usable: 50, want: 0},
		{name: "above the floor needs nothing", usable: 220, want: 0},
		{name: "just below the floor tops up to target", usable: 49, want: 100},
		{name: "empty channel cannot exceed the batch cap", usable: 0, want: 100},
		{name: "gap smaller than the cap is used as-is", usable: 120, want: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, base.MissingFor(tc.usable))
		})
	}

	disabled := base
	disabled.Enabled = false
	assert.Equal(t, 0, disabled.MissingFor(0), "a disabled provider never replenishes")

	noCap := base
	noCap.MaxBatch = 0
	assert.Equal(t, 190, noCap.MissingFor(10), "without a cap the whole gap is registered")
}

func TestUsableKeyCountMatchesRoutingEligibility(t *testing.T) {
	channel := &model.Channel{Key: "k1\nk2\nk3\nk4"}
	channel.ChannelInfo.IsMultiKey = true

	assert.Equal(t, 4, UsableKeyCount(channel), "no status list means every key is usable")

	channel.ChannelInfo.MultiKeyStatusList = map[int]int{
		1: common.ChannelStatusAutoDisabled,
		3: common.ChannelStatusManuallyDisabled,
	}
	assert.Equal(t, 2, UsableKeyCount(channel), "disabled keys are not usable")

	channel.ChannelInfo.MultiKeyStatusList = map[int]int{0: common.ChannelStatusEnabled}
	assert.Equal(t, 4, UsableKeyCount(channel))

	notMultiKey := &model.Channel{Key: "k1"}
	assert.Equal(t, 0, UsableKeyCount(notMultiKey))
	assert.Equal(t, 0, UsableKeyCount(nil))
}

func TestParseAccountLineKeepsOnlyUsableAccounts(t *testing.T) {
	account, ok := parseAccountLine(`{"username":"u_1","fingerprint":"fp","api_key":"dahl_abc"}`)
	require.True(t, ok)
	assert.Equal(t, Account{Username: "u_1", Fingerprint: "fp", APIKey: "dahl_abc"}, account)

	_, ok = parseAccountLine("  ")
	assert.False(t, ok, "a blank line is skipped")

	_, ok = parseAccountLine("not json")
	assert.False(t, ok, "a progress log line is skipped")

	_, ok = parseAccountLine(`{"username":"u_2","fingerprint":"fp"}`)
	assert.False(t, ok, "an account without a key is not persisted")
}

func TestCappedBufferKeepsOnlyTheFirstBytesAndReportsFullLength(t *testing.T) {
	buffer := newCappedBuffer(4)
	written, err := buffer.Write([]byte("abcdef"))
	require.NoError(t, err)
	assert.Equal(t, 6, written, "a writer must report the full length or the child sees a short write")
	assert.Equal(t, "abcd", buffer.String())

	_, _ = buffer.Write([]byte("gh"))
	assert.Equal(t, "abcd", buffer.String(), "bytes past the cap are dropped, not appended")
}

func TestAppendKeysToChannelDeduplicatesAndRecomputesSize(t *testing.T) {
	useRegistrarTestDB(t)

	channel := &model.Channel{
		Id:     424242,
		Name:   "registrar-test",
		Type:   constant.ChannelTypeOpenAI,
		Status: common.ChannelStatusEnabled,
		Key:    "k1\nk2\nk3",
		Group:  "default",
		Models: "m1",
	}
	channel.ChannelInfo.IsMultiKey = true
	channel.ChannelInfo.MultiKeySize = 3
	require.NoError(t, model.DB.Create(channel).Error)

	added, err := AppendKeysToChannel(channel.Id, []string{"k3", "k4", " "})
	require.NoError(t, err)
	assert.Equal(t, 1, added, "only a key the channel does not already carry is added")

	reloaded, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, "k1\nk2\nk3\nk4", reloaded.Key)
	assert.Equal(t, 4, reloaded.ChannelInfo.MultiKeySize, "Update recomputes the key count")
	assert.Equal(t, 4, UsableKeyCount(reloaded))

	added, err = AppendKeysToChannel(channel.Id, []string{"k4"})
	require.NoError(t, err)
	assert.Equal(t, 0, added, "re-adding an existing key is a no-op")
}

func TestAppendKeysToChannelRejectsANonMultiKeyChannel(t *testing.T) {
	useRegistrarTestDB(t)

	channel := &model.Channel{
		Id:     424243,
		Name:   "single-key",
		Type:   constant.ChannelTypeOpenAI,
		Status: common.ChannelStatusEnabled,
		Key:    "k1",
		Group:  "default",
		Models: "m1",
	}
	require.NoError(t, model.DB.Create(channel).Error)

	_, err := AppendKeysToChannel(channel.Id, []string{"k2"})
	require.Error(t, err)

	_, err = AppendKeysToChannel(999999, []string{"k2"})
	require.Error(t, err)
}

func useRegistrarTestDB(t *testing.T) {
	t.Helper()
	previousDB := model.DB
	previousType := common.MainDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
	model.DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetMainDatabaseType(previousType)
	})
}
