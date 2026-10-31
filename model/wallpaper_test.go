package model

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Pinned billing fixture: $1 = 500000 quota and ¥8 = $1, so one 0.5 元 change
// costs exactly 31250 quota. Exact values keep the assertions decisive.
const (
	testWallpaperQuotaPerUnit = 500000.0
	testWallpaperRate         = 8.0
	testWallpaperPriceYuan    = 0.5
	testWallpaperQuota        = 31250
)

func setupWallpaperTest(t *testing.T, quota int) *User {
	t.Helper()
	truncateTables(t)

	previousQuotaPerUnit := common.QuotaPerUnit
	previousRate := operation_setting.USDExchangeRate
	t.Cleanup(func() {
		common.QuotaPerUnit = previousQuotaPerUnit
		operation_setting.USDExchangeRate = previousRate
	})
	common.QuotaPerUnit = testWallpaperQuotaPerUnit
	operation_setting.USDExchangeRate = testWallpaperRate

	setting := operation_setting.GetWallpaperSetting()
	restore := *setting
	t.Cleanup(func() { *setting = restore })
	setting.Enabled = true
	setting.PriceYuan = testWallpaperPriceYuan
	setting.MonthlyChangeLimit = 3

	user := &User{
		Username: "wallpaper-user",
		Password: "wallpaper-password",
		Email:    "wallpaper@example.com",
		Quota:    quota,
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, DB.Create(user).Error)
	return user
}

func wallpaperQuotaOf(t *testing.T, userId int) int {
	t.Helper()
	var user User
	require.NoError(t, DB.Select("quota").Where("id = ?", userId).First(&user).Error)
	return user.Quota
}

func TestWallpaperQuotaConvertsYuanAtTheConfiguredExchangeRate(t *testing.T) {
	previousQuotaPerUnit := common.QuotaPerUnit
	previousRate := operation_setting.USDExchangeRate
	t.Cleanup(func() {
		common.QuotaPerUnit = previousQuotaPerUnit
		operation_setting.USDExchangeRate = previousRate
	})

	tests := []struct {
		name         string
		priceYuan    float64
		quotaPerUnit float64
		rate         float64
		want         int
	}{
		{name: "the configured 0.5 元", priceYuan: 0.5, quotaPerUnit: 500000, rate: 8, want: 31250},
		{name: "a fractional rate rounds up to the next quota", priceYuan: 0.5, quotaPerUnit: 500000, rate: 7.3, want: 34247},
		{name: "a free change costs nothing", priceYuan: 0, quotaPerUnit: 500000, rate: 8, want: 0},
		{name: "a non-positive price costs nothing", priceYuan: -1, quotaPerUnit: 500000, rate: 8, want: 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			common.QuotaPerUnit = tc.quotaPerUnit
			operation_setting.USDExchangeRate = tc.rate

			quota, err := WallpaperQuota(tc.priceYuan)
			require.NoError(t, err)
			assert.Equal(t, tc.want, quota)
		})
	}
}

func TestWallpaperQuotaRejectsAPriceBeyondTheWalletRange(t *testing.T) {
	previousRate := operation_setting.USDExchangeRate
	t.Cleanup(func() { operation_setting.USDExchangeRate = previousRate })
	operation_setting.USDExchangeRate = testWallpaperRate

	_, err := WallpaperQuota(1e12)
	require.Error(t, err, "an absurd price must fail loudly instead of saturating into a bogus charge")
}

func TestNormalizeWallpaperURLKeepsOnlySafeAbsoluteHTTPAddresses(t *testing.T) {
	tooLong := "https://cdn.example.com/" + strings.Repeat("a", WallpaperImageURLMaxLength)

	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr error
	}{
		{name: "https is stored as given", raw: "https://cdn.example.com/a.png", want: "https://cdn.example.com/a.png"},
		{name: "http is accepted and trimmed", raw: "  http://cdn.example.com/b.jpg  ", want: "http://cdn.example.com/b.jpg"},
		{name: "a query string survives", raw: "https://cdn.example.com/a.png?v=2", want: "https://cdn.example.com/a.png?v=2"},
		{name: "empty is rejected", raw: "   ", wantErr: ErrWallpaperURLRequired},
		{name: "a non-http scheme is rejected", raw: "ftp://cdn.example.com/a.png", wantErr: ErrWallpaperURLInvalid},
		{name: "an inline data payload is rejected", raw: "data:image/png;base64,AAAA", wantErr: ErrWallpaperURLInvalid},
		{name: "a script url is rejected", raw: "javascript:alert(1)", wantErr: ErrWallpaperURLInvalid},
		{name: "a relative path is rejected", raw: "/assets/a.png", wantErr: ErrWallpaperURLInvalid},
		{name: "an address without a host is rejected", raw: "https:///a.png", wantErr: ErrWallpaperURLInvalid},
		{name: "embedded credentials are rejected", raw: "https://user:pw@cdn.example.com/a.png", wantErr: ErrWallpaperURLInvalid},
		{name: "a quote cannot escape the css url", raw: `https://cdn.example.com/a"b.png`, wantErr: ErrWallpaperURLInvalid},
		{name: "a parenthesis cannot escape the css url", raw: "https://cdn.example.com/a(b).png", wantErr: ErrWallpaperURLInvalid},
		{name: "an inner space is rejected", raw: "https://cdn.example.com/a b.png", wantErr: ErrWallpaperURLInvalid},
		{name: "an oversized address is rejected", raw: tooLong, wantErr: ErrWallpaperURLTooLong},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeWallpaperURL(tc.raw)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestSetUserWallpaperAllowsExactlyTheConfiguredMonthlyLimit(t *testing.T) {
	user := setupWallpaperTest(t, 1_000_000)

	for i := 1; i <= 3; i++ {
		saved, err := SetUserWallpaper(user.Id, fmt.Sprintf("https://cdn.example.com/w%d.png", i))
		require.NoError(t, err)
		assert.Equal(t, i, saved.ChangeCount, "change %d is the %dth of the month", i, i)
		assert.Equal(t, fmt.Sprintf("https://cdn.example.com/w%d.png", i), saved.WallpaperUrl)
	}

	_, err := SetUserWallpaper(user.Id, "https://cdn.example.com/w4.png")
	require.ErrorIs(t, err, ErrWallpaperMonthlyLimit)

	assert.Equal(t, 1_000_000-3*testWallpaperQuota, wallpaperQuotaOf(t, user.Id), "the refused change is not charged")

	status, err := GetUserWallpaperStatus(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 3, status.ChangesUsed)
	assert.Equal(t, 0, status.ChangesRemaining)
	assert.Equal(t, "https://cdn.example.com/w3.png", status.WallpaperUrl, "the refused change does not overwrite the current wallpaper")
}

func TestSetUserWallpaperLeavesTheWalletAndAllowanceUntouchedWhenBalanceIsShort(t *testing.T) {
	user := setupWallpaperTest(t, testWallpaperQuota-1)

	_, err := SetUserWallpaper(user.Id, "https://cdn.example.com/w1.png")
	require.ErrorIs(t, err, ErrWallpaperInsufficient)

	assert.Equal(t, testWallpaperQuota-1, wallpaperQuotaOf(t, user.Id))

	var rows int64
	require.NoError(t, DB.Model(&UserWallpaper{}).Where("user_id = ?", user.Id).Count(&rows).Error)
	assert.Zero(t, rows, "a failed charge must not consume a change")

	status, err := GetUserWallpaperStatus(user.Id)
	require.NoError(t, err)
	assert.Zero(t, status.ChangesUsed)
	assert.Equal(t, 3, status.ChangesRemaining)
}

func TestSetUserWallpaperRollsBackTheChargeWhenRecordingTheWallpaperFails(t *testing.T) {
	user := setupWallpaperTest(t, 1_000_000)
	require.NoError(t, DB.Migrator().DropTable(&UserWallpaper{}))
	t.Cleanup(func() { require.NoError(t, DB.AutoMigrate(&UserWallpaper{})) })

	_, err := SetUserWallpaper(user.Id, "https://cdn.example.com/w1.png")
	require.Error(t, err)
	assert.Equal(t, 1_000_000, wallpaperQuotaOf(t, user.Id), "a change that could not be recorded must not be charged")
}

func TestSetUserWallpaperStartsAFreshAllowanceInANewMonth(t *testing.T) {
	user := setupWallpaperTest(t, 1_000_000)
	// First day of the current month minus one day, not AddDate(0,-1,0): on the
	// 31st that normalizes to the 1st of the same month, so "last month" and
	// "this month" become the same period and the test fails every 31st.
	now := time.Now()
	previousPeriod := WallpaperPeriod(time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).AddDate(0, 0, -1))
	require.NoError(t, DB.Create(&UserWallpaper{
		UserId:       user.Id,
		WallpaperUrl: "https://cdn.example.com/old.png",
		Period:       previousPeriod,
		ChangeCount:  3,
		CreatedAt:    common.GetTimestamp(),
		UpdatedAt:    common.GetTimestamp(),
	}).Error)

	saved, err := SetUserWallpaper(user.Id, "https://cdn.example.com/new.png")
	require.NoError(t, err)
	assert.Equal(t, 1, saved.ChangeCount, "last month's exhausted allowance does not carry over")

	status, err := GetUserWallpaperStatus(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 1, status.ChangesUsed)
	assert.Equal(t, 2, status.ChangesRemaining)
	assert.Equal(t, "https://cdn.example.com/new.png", status.WallpaperUrl)
}

func TestSetUserWallpaperRejectsAnUnchangedAddressWithoutCharging(t *testing.T) {
	user := setupWallpaperTest(t, 1_000_000)
	_, err := SetUserWallpaper(user.Id, "https://cdn.example.com/same.png")
	require.NoError(t, err)

	_, err = SetUserWallpaper(user.Id, "  https://cdn.example.com/same.png  ")
	require.ErrorIs(t, err, ErrWallpaperUnchanged)

	assert.Equal(t, 1_000_000-testWallpaperQuota, wallpaperQuotaOf(t, user.Id))
	status, err := GetUserWallpaperStatus(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 1, status.ChangesUsed)
}

func TestDeleteUserWallpaperClearsTheImageWithoutRestoringTheAllowance(t *testing.T) {
	user := setupWallpaperTest(t, 1_000_000)
	for i := 1; i <= 3; i++ {
		_, err := SetUserWallpaper(user.Id, fmt.Sprintf("https://cdn.example.com/w%d.png", i))
		require.NoError(t, err)
	}

	require.NoError(t, DeleteUserWallpaper(user.Id))

	status, err := GetUserWallpaperStatus(user.Id)
	require.NoError(t, err)
	assert.Empty(t, status.WallpaperUrl)
	assert.Equal(t, 3, status.ChangesUsed, "clearing must not hand a change back")
	assert.Equal(t, 0, status.ChangesRemaining)

	_, err = SetUserWallpaper(user.Id, "https://cdn.example.com/again.png")
	require.ErrorIs(t, err, ErrWallpaperMonthlyLimit)
}

func TestSetUserWallpaperRefusesWhenTheFeatureIsDisabled(t *testing.T) {
	user := setupWallpaperTest(t, 1_000_000)
	operation_setting.GetWallpaperSetting().Enabled = false

	_, err := SetUserWallpaper(user.Id, "https://cdn.example.com/w1.png")
	require.ErrorIs(t, err, ErrWallpaperDisabled)
	assert.Equal(t, 1_000_000, wallpaperQuotaOf(t, user.Id))
}
