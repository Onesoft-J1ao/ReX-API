package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A subscription plan can carry independent usage caps on top of its total
// quota: a 5-hour cap bounds a burst while the weekly cap meters a longer
// allowance out of the same total. These tests pin the cap arithmetic and,
// most importantly, that a rejected request leaves no trace behind.

func TestCanIncreaseSubscriptionUsageAllowsExactlyUpToTheLimit(t *testing.T) {
	assert.True(t, canIncreaseSubscriptionUsage(0, 12, 12), "the full limit must fit")
	assert.False(t, canIncreaseSubscriptionUsage(12, 12, 1), "one unit past must not")
	assert.True(t, canIncreaseSubscriptionUsage(11, 12, 1))
}

func TestCanIncreaseSubscriptionUsageTreatsZeroLimitAsDisabled(t *testing.T) {
	assert.True(t, canIncreaseSubscriptionUsage(1_000_000, 0, 1_000_000),
		"a zero cap means the limit is off, not that nothing may be spent")
}

func TestCanIncreaseSubscriptionUsageRejectsOverflow(t *testing.T) {
	// A charge that would wrap the counter must be refused, not accepted.
	assert.False(t, canIncreaseSubscriptionUsage(1, 0, int64(^uint64(0)>>1)))
	assert.False(t, canIncreaseSubscriptionUsage(-1, 10, 1), "a negative used value is corrupt")
	assert.False(t, canIncreaseSubscriptionUsage(0, 10, 0))
}

func TestDecreaseSubscriptionUsageClampsAtZero(t *testing.T) {
	assert.Equal(t, int64(4), decreaseSubscriptionUsage(10, -6))
	// A refund can arrive after the window rolled over, when the counter is
	// already below the amount being returned.
	assert.Zero(t, decreaseSubscriptionUsage(3, -10))
	assert.Zero(t, decreaseSubscriptionUsage(0, -5))
	assert.Zero(t, decreaseSubscriptionUsage(10, 6), "a positive delta is not a decrease")
}

func TestMaybeResetSubscriptionUsageLimitRollsAnElapsedWindow(t *testing.T) {
	now := int64(1_700_000_000)
	used := int64(12)
	last := now - 6*3600
	next := now - 3600 // an hour past due

	changed := maybeResetSubscriptionUsageLimit(
		&used, &last, &next, 12, SubscriptionResetFiveHour, now-24*3600, 0, now,
	)

	assert.True(t, changed)
	assert.Zero(t, used, "an elapsed window starts over")
	assert.Greater(t, next, now, "the next reset must be in the future")
}

func TestMaybeResetSubscriptionUsageLimitLeavesAnActiveWindowAlone(t *testing.T) {
	now := int64(1_700_000_000)
	used := int64(5)
	last := now - 3600
	next := now + 4*3600

	changed := maybeResetSubscriptionUsageLimit(
		&used, &last, &next, 12, SubscriptionResetFiveHour, now-24*3600, 0, now,
	)

	assert.False(t, changed)
	assert.Equal(t, int64(5), used)
}

func TestMaybeResetSubscriptionUsageLimitSkipsDisabledCaps(t *testing.T) {
	now := int64(1_700_000_000)
	used := int64(0)
	last := int64(0)
	next := int64(0)

	changed := maybeResetSubscriptionUsageLimit(
		&used, &last, &next, 0, SubscriptionResetFiveHour, now, 0, now,
	)

	assert.False(t, changed)
	assert.Zero(t, last, "a disabled cap must not acquire an anchor")
}

// An untouched 5-hour window has no anchor. Anchoring it at subscription
// creation would start the clock before the user made a request, so the first
// consumption sets it instead.
func TestMaybeResetSubscriptionUsageLimitDoesNotAnchorAnUnusedFiveHourWindow(t *testing.T) {
	now := int64(1_700_000_000)
	used := int64(0)
	last := int64(0)
	next := int64(0)

	changed := maybeResetSubscriptionUsageLimit(
		&used, &last, &next, 12, SubscriptionResetFiveHour, now-10*3600, 0, now,
	)

	assert.False(t, changed)
	assert.Zero(t, last)
	assert.Zero(t, next)
}

func TestInitializeSubscriptionUsageWindowOpensAtFirstConsumption(t *testing.T) {
	now := int64(1_700_000_000)
	last := int64(0)
	next := int64(0)

	changed := initializeSubscriptionUsageWindow(
		0, 5, &last, &next, 12, SubscriptionResetFiveHour, 0, now,
	)

	assert.True(t, changed)
	assert.Equal(t, now, last, "the window opens now, not at purchase")
	assert.Equal(t, now+5*3600, next)
}

func TestInitializeSubscriptionUsageWindowIsInertOnceAnchored(t *testing.T) {
	now := int64(1_700_000_000)
	last := now - 3600
	next := now + 4*3600

	changed := initializeSubscriptionUsageWindow(
		0, 5, &last, &next, 12, SubscriptionResetFiveHour, 0, now,
	)

	assert.False(t, changed)
	assert.Equal(t, now-3600, last, "an anchored window keeps its anchor")
}

func TestPreConsumeEnforcesTheFiveHourCapAndLeavesNoChargeOnRejection(t *testing.T) {
	truncateTables(t)

	now := GetDBTimestamp()
	plan := &SubscriptionPlan{
		Id:               9801,
		Title:            "Go Plan",
		PriceAmount:      40,
		DurationUnit:     SubscriptionDurationMonth,
		DurationValue:    1,
		TotalAmount:      30_000_000,
		QuotaResetPeriod: SubscriptionResetWeekly,
		FiveHourLimit:    12,
		WeeklyLimit:      60,
	}
	seedSubscriptionResetPlan(t, plan)
	seedSubscriptionResetSub(t, &UserSubscription{
		Id: 9802, UserId: 501, PlanId: plan.Id,
		AmountTotal: 30_000_000, AmountUsed: 0,
		StartTime: now - 60, EndTime: now + 30*24*3600, Status: "active",
		FiveHourLimit: 12, WeeklyLimit: 60,
	})

	// Spend the whole 5-hour allowance.
	_, err := PreConsumeUserSubscription("req-5h-1", 501, "glm-5.3", 0, 12)
	require.NoError(t, err)

	// The next request is refused by the 5-hour cap, and the refusal must not
	// have written a pre-consume record: a later refund would otherwise credit
	// back quota that was never charged.
	_, err = PreConsumeUserSubscription("req-5h-2", 501, "glm-5.3", 0, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "5-hour")

	var records int64
	require.NoError(t, DB.Model(&SubscriptionPreConsumeRecord{}).Where("request_id = ?", "req-5h-2").Count(&records).Error)
	assert.Zero(t, records, "a rejected attempt must leave no pre-consume record")

	sub := getSubscriptionResetSub(t, 9802)
	assert.Equal(t, int64(12), sub.AmountUsed, "the total must only reflect the accepted request")
	assert.Equal(t, int64(12), sub.FiveHourUsed)
}

// The 5-hour cap must roll even while the weekly total still has time left on
// it. Resetting only the billing period would leave a burst window stuck shut
// for the rest of the week.
func TestFiveHourCapRollsWhileTheWeeklyWindowIsStillOpen(t *testing.T) {
	truncateTables(t)

	now := GetDBTimestamp()
	plan := &SubscriptionPlan{
		Id:               9811,
		Title:            "Go Plan",
		PriceAmount:      40,
		DurationUnit:     SubscriptionDurationMonth,
		DurationValue:    1,
		TotalAmount:      30_000_000,
		QuotaResetPeriod: SubscriptionResetWeekly,
		FiveHourLimit:    12,
		WeeklyLimit:      60,
	}
	seedSubscriptionResetPlan(t, plan)
	// The 5-hour window is already past due; the weekly one is not.
	seedSubscriptionResetSub(t, &UserSubscription{
		Id: 9812, UserId: 502, PlanId: plan.Id,
		AmountTotal: 30_000_000, AmountUsed: 12,
		StartTime: now - 24*3600, EndTime: now + 30*24*3600, Status: "active",
		LastResetTime:         now - 3600,
		NextResetTime:         now + 6*24*3600,
		FiveHourLimit:         12,
		FiveHourUsed:          12,
		FiveHourLastResetTime: now - 6*3600,
		FiveHourNextResetTime: now - 3600,
		WeeklyLimit:           60,
		WeeklyUsed:            12,
		WeeklyLastResetTime:   now - 3600,
		WeeklyNextResetTime:   now + 6*24*3600,
	})

	// The rolled-over 5-hour window must admit the request again.
	_, err := PreConsumeUserSubscription("req-5h-roll", 502, "glm-5.3", 0, 5)
	require.NoError(t, err)

	sub := getSubscriptionResetSub(t, 9812)
	assert.Equal(t, int64(5), sub.FiveHourUsed, "the 5-hour window rolled over")
	assert.Equal(t, int64(17), sub.WeeklyUsed, "the weekly window kept accumulating")
	assert.Equal(t, int64(17), sub.AmountUsed)
}

// A refund must return the amount to every window it was charged against, and
// it must not deadlock: the refund already holds a transaction open, so the
// delta must be applied inside it rather than through a nested transaction.
func TestRefundReturnsQuotaToEveryWindowWithoutDeadlock(t *testing.T) {
	truncateTables(t)

	now := GetDBTimestamp()
	plan := &SubscriptionPlan{
		Id:               9821,
		Title:            "Go Plan",
		PriceAmount:      40,
		DurationUnit:     SubscriptionDurationMonth,
		DurationValue:    1,
		TotalAmount:      30_000_000,
		QuotaResetPeriod: SubscriptionResetWeekly,
		FiveHourLimit:    12,
		WeeklyLimit:      60,
	}
	seedSubscriptionResetPlan(t, plan)
	seedSubscriptionResetSub(t, &UserSubscription{
		Id: 9822, UserId: 503, PlanId: plan.Id,
		AmountTotal: 30_000_000, AmountUsed: 0,
		StartTime: now - 60, EndTime: now + 30*24*3600, Status: "active",
		FiveHourLimit: 12, WeeklyLimit: 60,
	})

	_, err := PreConsumeUserSubscription("req-refund-5h", 503, "glm-5.3", 0, 12)
	require.NoError(t, err)
	require.NoError(t, RefundSubscriptionPreConsume("req-refund-5h"))

	sub := getSubscriptionResetSub(t, 9822)
	assert.Zero(t, sub.AmountUsed, "a refund must return the amount to the total")
	assert.Zero(t, sub.FiveHourUsed, "and to the 5-hour window")
	assert.Zero(t, sub.WeeklyUsed, "and to the weekly window")

	// The headroom is usable again.
	_, err = PreConsumeUserSubscription("req-refund-5h-2", 503, "glm-5.3", 0, 12)
	require.NoError(t, err)
}
