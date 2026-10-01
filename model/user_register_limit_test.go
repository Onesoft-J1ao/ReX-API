package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createUserAt inserts a minimal user with an explicit created_at so the daily
// window can be exercised without waiting for a day boundary.
func createUserAt(t *testing.T, id int, username string, createdAt int64) {
	t.Helper()
	require.NoError(t, DB.Create(&User{
		Id:          id,
		Username:    username,
		Password:    "password123",
		DisplayName: username,
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AffCode:     fmt.Sprintf("aff%06d", id),
		CreatedAt:   createdAt,
	}).Error)
}

func withRegisterDailyLimit(t *testing.T, limit int) {
	t.Helper()
	previous := common.RegisterDailyLimit
	common.RegisterDailyLimit = limit
	t.Cleanup(func() { common.RegisterDailyLimit = previous })
}

func TestCheckRegisterDailyLimitIgnoresAccountsFromEarlierDays(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Exec("DELETE FROM users").Error)
	withRegisterDailyLimit(t, 2)

	now := time.Now()
	createUserAt(t, 910001, "yesterday", now.AddDate(0, 0, -1).Unix())
	createUserAt(t, 910002, "two_days_ago", now.AddDate(0, 0, -2).Unix())

	require.NoError(t, CheckRegisterDailyLimit(), "only accounts created today consume today's quota")
}

func TestCheckRegisterDailyLimitRejectsOnceTheDaysQuotaIsUsed(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Exec("DELETE FROM users").Error)
	withRegisterDailyLimit(t, 2)

	today := time.Now().Unix()
	createUserAt(t, 910011, "today_one", today)
	require.NoError(t, CheckRegisterDailyLimit(), "one of two slots is still free")

	createUserAt(t, 910012, "today_two", today)
	assert.ErrorIs(t, CheckRegisterDailyLimit(), ErrRegisterDailyLimitReached)
}

func TestCheckRegisterDailyLimitCountsRemovedAccounts(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Exec("DELETE FROM users").Error)
	withRegisterDailyLimit(t, 1)

	createUserAt(t, 910021, "deleted_today", time.Now().Unix())
	require.NoError(t, DB.Where("id = ?", 910021).Delete(&User{}).Error)

	assert.ErrorIs(t, CheckRegisterDailyLimit(), ErrRegisterDailyLimitReached,
		"soft-deleting an account must not hand its slot back")
}

func TestCheckRegisterDailyLimitIsDisabledWhenZero(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Exec("DELETE FROM users").Error)
	withRegisterDailyLimit(t, 0)

	today := time.Now().Unix()
	for i := range 5 {
		createUserAt(t, 910031+i, fmt.Sprintf("unlimited_%d", i), today)
	}

	require.NoError(t, CheckRegisterDailyLimit(), "a zero limit means unlimited signups")
}

func TestCountUsersCreatedSinceCountsRemovedRows(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Exec("DELETE FROM users").Error)

	now := time.Now().Unix()
	createUserAt(t, 910041, "live", now)
	createUserAt(t, 910042, "removed", now)
	require.NoError(t, DB.Where("id = ?", 910042).Delete(&User{}).Error)

	count, err := CountUsersCreatedSince(now - 60)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)
}
