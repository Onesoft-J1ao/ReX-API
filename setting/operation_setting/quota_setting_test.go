package operation_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The trust-quota fast path skips pre-consume and charges only at settle, so
// concurrent bursts can overspend before the balance moves. It stays off unless
// an operator raises the threshold deliberately.
func TestTrustQuotaFastPathOffByDefault(t *testing.T) {
	assert.Equal(t, 0.0, GetQuotaSetting().TrustQuotaUSD)
}

// The minimum billable floor replaced "a zero price means a free request": a
// model with no price still costs one unit, so an empty wallet cannot use it.
func TestMinBillablePriceDefaultsToTheFloor(t *testing.T) {
	assert.Equal(t, DefaultMinBillablePrice, GetQuotaSetting().MinBillablePrice)
	assert.GreaterOrEqual(t, MinBillableQuota(), 1)
}

func TestValidateQuotaOptionRejectsUnusableValues(t *testing.T) {
	require.NoError(t, ValidateQuotaOption("quota_setting.trust_quota_usd", "0"))
	require.NoError(t, ValidateQuotaOption("quota_setting.trust_quota_usd", "25"))
	// A zero multiplier would reserve nothing, which is the same as having no
	// pre-consume at all, so it is rejected rather than silently accepted.
	require.Error(t, ValidateQuotaOption("quota_setting.pre_consume_multiplier", "0"))
	require.Error(t, ValidateQuotaOption("quota_setting.trust_quota_usd", "-1"))
	require.Error(t, ValidateQuotaOption("quota_setting.trust_quota_usd", "abc"))
	// Keys this validator does not own are left to their own validators.
	require.NoError(t, ValidateQuotaOption("quota_setting.min_billable_price", "-1"))
}
