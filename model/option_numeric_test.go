/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A value the parser cannot read used to be stored and then silently dropped,
// leaving the setting at zero while the page still showed the stored string.
// For a count-like option that zero is not a neutral default: a rate limit of
// 0 disables the limit and a minimum top-up of 0 removes the floor.
func TestValidateNumericOptionRejectsUnreadableValues(t *testing.T) {
	for _, key := range []string{
		"ModelRequestRateLimitCount",
		"RegisterDailyLimit",
		"MinTopUp",
		"StripeMinTopUp",
		"WaffoMinTopUp",
		"NowPaymentsMinTopUp",
		"RetryTimes",
		"PreConsumedQuota",
		"DataExportInterval",
		"StreamCacheQueueLength",
		"SMTPPort",
		"FileUploadPermission",
	} {
		t.Run(key+"/decimal", func(t *testing.T) {
			require.Error(t, validateNumericOption(key, "0.5"))
		})
		t.Run(key+"/negative", func(t *testing.T) {
			require.Error(t, validateNumericOption(key, "-1"))
		})
		t.Run(key+"/text", func(t *testing.T) {
			require.Error(t, validateNumericOption(key, "many"))
		})
	}

	for _, key := range []string{"QuotaPerUnit", "Price", "USDExchangeRate", "ChannelDisableThreshold"} {
		t.Run(key+"/text", func(t *testing.T) {
			require.Error(t, validateNumericOption(key, "free"))
		})
		t.Run(key+"/negative", func(t *testing.T) {
			require.Error(t, validateNumericOption(key, "-0.5"))
		})
	}

	// Zero QuotaPerUnit makes every charge compute to zero, so it is not a
	// value the form may store even though the other amounts allow it.
	require.Error(t, validateNumericOption("QuotaPerUnit", "0"))
	require.NoError(t, validateNumericOption("Price", "0"))
	require.NoError(t, validateNumericOption("ChannelDisableThreshold", "0"))
}

func TestValidateNumericOptionAcceptsWellFormedValues(t *testing.T) {
	for _, key := range []string{
		"ModelRequestRateLimitCount",
		"RegisterDailyLimit",
		"MinTopUp",
		"RetryTimes",
		"FileUploadPermission",
	} {
		require.NoError(t, validateNumericOption(key, "0"), key)
		require.NoError(t, validateNumericOption(key, "42"), key)
		require.NoError(t, validateNumericOption(key, " 42 "), key)
		// An empty value clears the option rather than failing the save.
		require.NoError(t, validateNumericOption(key, ""), key)
	}
	for _, key := range []string{"QuotaPerUnit", "Price", "USDExchangeRate"} {
		require.NoError(t, validateNumericOption(key, "0.05"), key)
		require.NoError(t, validateNumericOption(key, "7.2"), key)
	}
	// Keys the registry does not know are left to whatever validation the
	// option itself carries.
	require.NoError(t, validateNumericOption("SomeStringOption", "0.5"))
}

// Loading an unreadable value must keep the current setting rather than reset
// it to zero — the bug this guards is only visible on reload.
func TestAssignIntOptionKeepsCurrentValueWhenUnreadable(t *testing.T) {
	target := 7
	assignIntOption(&target, "RetryTimes", "12")
	assert.Equal(t, 12, target)

	assignIntOption(&target, "RetryTimes", "0.5")
	assert.Equal(t, 12, target, "an unreadable value must not reset the setting")

	assignIntOption(&target, "RetryTimes", "")
	assert.Equal(t, 12, target, "an empty value clears the option, not the setting")

	assignIntOption(&target, "RetryTimes", "0")
	assert.Equal(t, 0, target, "zero is a real value for a count")
}

func TestAssignFloatOptionKeepsCurrentValueWhenUnreadable(t *testing.T) {
	target := 500000.0
	assignFloatOption(&target, "QuotaPerUnit", "1000")
	assert.Equal(t, 1000.0, target)

	assignFloatOption(&target, "QuotaPerUnit", "free")
	assert.Equal(t, 1000.0, target)

	assignFloatOption(&target, "QuotaPerUnit", "0")
	assert.Equal(t, 1000.0, target, "a non-positive QuotaPerUnit would zero every charge")

	assignFloatOption(&target, "QuotaPerUnit", "0.25")
	assert.Equal(t, 0.25, target)
}

// The end-to-end shape: an option saved as a decimal reaches updateOptionMap
// and must leave the running setting alone.
func TestUpdateOptionMapKeepsNumericSettingWhenValueIsUnreadable(t *testing.T) {
	originalRetry := common.RetryTimes
	// updateOptionMap writes into the shared option map, which is only
	// initialised once the options have been loaded from a database. Restore
	// whatever the other tests left rather than assuming one shape.
	originalOptionMap := common.OptionMap
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	t.Cleanup(func() {
		common.RetryTimes = originalRetry
		common.OptionMap = originalOptionMap
	})

	require.NoError(t, updateOptionMap("RetryTimes", "3"))
	require.Equal(t, 3, common.RetryTimes)

	updateOptionMap("RetryTimes", "0.5")
	assert.Equal(t, 3, common.RetryTimes, "a decimal retry count must not disable retries silently")
}
