package controller

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withPriceSentinelSetting(t *testing.T, values map[string]string) {
	t.Helper()
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})
	require.NoError(t, config.GlobalConfig.LoadFromDB(values))
}

// With no send time configured the handler keeps the plain interval rule, so an
// instance that never sets an anchor schedules exactly as it did before.
func TestPriceSentinelNextRunFallsBackToTheInterval(t *testing.T) {
	withPriceSentinelSetting(t, map[string]string{
		"price_sentinel_setting.interval_minutes": "300",
		"price_sentinel_setting.send_time":        "",
	})

	lastRun := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	assert.Equal(t, lastRun.Add(5*time.Hour), priceSentinelHandler{}.NextRunAfter(lastRun))
}

// An anchor holds the digest at a chosen time of day: a 5h interval anchored at
// 09:00 means the slots are 09:00, 14:00, 19:00, 00:00, ...
func TestPriceSentinelNextRunAnchorsToTheSendTime(t *testing.T) {
	withPriceSentinelSetting(t, map[string]string{
		"price_sentinel_setting.interval_minutes": "300",
		"price_sentinel_setting.send_time":        "09:00",
	})
	handler := priceSentinelHandler{}

	at := func(hour, minute int) time.Time {
		return time.Date(2026, 9, 30, hour, minute, 0, 0, time.UTC)
	}

	assert.Equal(t, at(9, 0), handler.NextRunAfter(at(6, 0)),
		"a run before today's slot waits for it")

	assert.Equal(t, at(14, 0), handler.NextRunAfter(at(9, 0)),
		"a run exactly on the slot advances by one interval")

	assert.Equal(t, at(19, 0), handler.NextRunAfter(at(15, 30)),
		"a run between slots lands on the next one")

	assert.Equal(t, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), handler.NextRunAfter(at(23, 0)),
		"past the last slot of the day the series rolls into the next day")
}

// Nothing validates the stored string, so a malformed anchor must degrade to the
// plain interval rather than stall the digest forever.
func TestPriceSentinelNextRunIgnoresAMalformedSendTime(t *testing.T) {
	withPriceSentinelSetting(t, map[string]string{
		"price_sentinel_setting.interval_minutes": "60",
		"price_sentinel_setting.send_time":        "not-a-time",
	})

	lastRun := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	assert.Equal(t, lastRun.Add(time.Hour), priceSentinelHandler{}.NextRunAfter(lastRun))
}
