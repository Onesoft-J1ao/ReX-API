package model

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupPlaygroundFreeTestDB(t *testing.T) {
	t.Helper()

	previousDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&PlaygroundFreeUsage{}))
	DB = db
	t.Cleanup(func() { DB = previousDB })

	setting := operation_setting.GetPlaygroundSetting()
	restore := *setting
	t.Cleanup(func() { *setting = restore })
	setting.Enabled = true
	setting.FreeGenerations = 2
	setting.FreeTokens = 100
	setting.DedupeByIP = true
}

func TestPlaygroundFreeAllowanceStopsAtTheFirstExhaustedBudget(t *testing.T) {
	setupPlaygroundFreeTestDB(t)

	available, err := PlaygroundFreeAllowanceAvailable("fp-1", "10.0.0.1")
	require.NoError(t, err)
	require.True(t, available, "a fresh visitor starts with the allowance")

	// Two generations of a few tokens each: the generation budget runs out first,
	// so the visitor is done even though tokens remain.
	require.NoError(t, RecordPlaygroundFreeUsage("fp-1", "10.0.0.1", 5))
	require.NoError(t, RecordPlaygroundFreeUsage("fp-1", "10.0.0.1", 5))

	available, err = PlaygroundFreeAllowanceAvailable("fp-1", "10.0.0.1")
	require.NoError(t, err)
	assert.False(t, available, "the generation budget ends the free tier on its own")

	available, err = PlaygroundFreeAllowanceAvailable("fp-2", "10.0.0.2")
	require.NoError(t, err)
	assert.True(t, available, "an unrelated visitor is unaffected")
}

func TestPlaygroundFreeAllowanceTokenBudgetExhaustsOnItsOwn(t *testing.T) {
	setupPlaygroundFreeTestDB(t)
	setting := operation_setting.GetPlaygroundSetting()
	setting.FreeGenerations = 10 // not the binding budget in this case

	require.NoError(t, RecordPlaygroundFreeUsage("fp-3", "10.0.0.3", 100))

	available, err := PlaygroundFreeAllowanceAvailable("fp-3", "10.0.0.3")
	require.NoError(t, err)
	assert.False(t, available, "the token budget ends the free tier on its own")
}

func TestPlaygroundFreeAllowanceIsSharedAcrossScopes(t *testing.T) {
	setupPlaygroundFreeTestDB(t)
	setting := operation_setting.GetPlaygroundSetting()
	setting.FreeGenerations = 1
	setting.FreeTokens = 1000

	// One visit spends the generation budget held by this fingerprint and this
	// IP, so a second account on the same browser and network gets nothing.
	require.NoError(t, RecordPlaygroundFreeUsage("fp-4", "10.0.0.4", 1))

	available, err := PlaygroundFreeAllowanceAvailable("fp-4", "10.0.0.9")
	require.NoError(t, err)
	assert.False(t, available, "the fingerprint budget is shared")

	available, err = PlaygroundFreeAllowanceAvailable("fp-9", "10.0.0.4")
	require.NoError(t, err)
	assert.False(t, available, "the IP budget is shared")

	available, err = PlaygroundFreeAllowanceAvailable("fp-9", "10.0.0.9")
	require.NoError(t, err)
	assert.True(t, available, "an unrelated visitor keeps the allowance")
}

func TestPlaygroundFreeAllowanceOffWhenBudgetsAreZeroOrDisabled(t *testing.T) {
	setupPlaygroundFreeTestDB(t)
	setting := operation_setting.GetPlaygroundSetting()

	setting.FreeGenerations = 0
	setting.FreeTokens = 0
	available, err := PlaygroundFreeAllowanceAvailable("fp-5", "10.0.0.5")
	require.NoError(t, err)
	assert.False(t, available, "zero budgets disable the free tier")

	setting.FreeGenerations = 2
	setting.FreeTokens = 100
	setting.Enabled = false
	available, err = PlaygroundFreeAllowanceAvailable("fp-5", "10.0.0.5")
	require.NoError(t, err)
	assert.False(t, available, "the switch disables the free tier")
}
