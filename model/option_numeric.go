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
	"errors"
	"math"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// Numeric options are stored as strings and parsed on load. Every one of them
// used to discard the parse error, so a value the form had accepted but the
// parser could not read — a decimal where a whole number was required, say —
// left the setting at zero in memory while the row and the page both showed the
// stored string.
//
// For the count-like options that zero is not a harmless default: a rate limit
// of 0 disables the limit, a minimum top-up of 0 removes the floor, and
// QuotaPerUnit of 0 makes every charge compute to zero.
//
// Each option is therefore checked when it is saved, before the row is written,
// and re-checked on load, where an unreadable value keeps the current setting
// instead of resetting it.

// intOptions hold whole numbers. A negative value is accepted only where the
// setting itself documents one; 0 is always valid because most of these are
// "disabled" or "unlimited" switches.
var intOptions = map[string]bool{
	"SMTPPort":                             true,
	"RegisterDailyLimit":                   true,
	"SupportMessageLimit":                  true,
	"MinTopUp":                             true,
	"StripeMinTopUp":                       true,
	"WaffoMinTopUp":                        true,
	"WaffoPancakeMinTopUp":                 true,
	"MoneroConfirmations":                  true,
	"MoneroMaxSubaddresses":                true,
	"NowPaymentsMinTopUp":                  true,
	"NowPaymentsPaymentExpirationMins":     true,
	"LinuxDOMinimumTrustLevel":             true,
	"QuotaRemindThreshold":                 true,
	"PreConsumedQuota":                     true,
	"ModelRequestRateLimitCount":           true,
	"ModelRequestRateLimitDurationMinutes": true,
	"ModelRequestRateLimitSuccessCount":    true,
	"RetryTimes":                           true,
	"DataExportInterval":                   true,
	"StreamCacheQueueLength":               true,
}

// floatOptions hold decimal amounts: unit prices and exchange rates.
var floatOptions = map[string]bool{
	"Price":                         true,
	"USDExchangeRate":               true,
	"StripeUnitPrice":               true,
	"WaffoUnitPrice":                true,
	"WaffoPancakeUSDToCurrencyRate": true,
	"WaffoPancakeUnitPrice":         true,
	"MoneroUSDToCurrencyRate":       true,
	"NowPaymentsUSDToCurrencyRate":  true,
	"ChannelDisableThreshold":       true,
	"QuotaPerUnit":                  true,
}

// intOptionForPermission reports whether the key is one of the per-channel
// *Permission toggles, which share a suffix rather than a fixed key list.
func isPermissionOption(key string) bool {
	return strings.HasSuffix(key, "Permission")
}

// validateNumericOption rejects a value the parser will not be able to read
// back. It runs before the row is written, so a bad value fails the save loudly
// instead of taking effect as zero.
func validateNumericOption(key, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		// An empty value clears the option; the loader keeps whatever the
		// default is rather than parsing it.
		return nil
	}
	if isPermissionOption(key) || intOptions[key] {
		parsed, err := strconv.Atoi(trimmed)
		if err != nil {
			return errors.New(key + " must be a whole number, not " + value)
		}
		if parsed < 0 && key != "QuotaRemindThreshold" {
			// QuotaRemindThreshold is a non-negative quota amount like the
			// others; nothing here stores a negative count.
			return errors.New(key + " must not be negative")
		}
		return nil
	}
	if floatOptions[key] {
		parsed, err := strconv.ParseFloat(trimmed, 64)
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			return errors.New(key + " must be a number, not " + value)
		}
		if key == "QuotaPerUnit" && parsed <= 0 {
			// Every charge is a product of QuotaPerUnit, so zero or negative
			// would make the whole instance bill at zero.
			return errors.New(key + " must be greater than 0")
		}
		if key != "QuotaPerUnit" && parsed < 0 {
			return errors.New(key + " must not be negative")
		}
		return nil
	}
	return nil
}

// assignIntOption parses an integer option, keeping the current value when the
// stored string is unreadable. Loading such a value must not reset the setting
// to zero, which is what discarding strconv.Atoi's error used to do.
func assignIntOption(target *int, key, value string) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return
	}
	parsed, err := strconv.Atoi(trimmed)
	if err != nil {
		common.SysLog("ignoring invalid integer option " + key + "=" + value)
		return
	}
	*target = parsed
}

// assignFloatOption is assignIntOption for decimal amounts.
func assignFloatOption(target *float64, key, value string) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return
	}
	parsed, err := strconv.ParseFloat(trimmed, 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		common.SysLog("ignoring invalid numeric option " + key + "=" + value)
		return
	}
	if key == "QuotaPerUnit" && parsed <= 0 {
		common.SysLog("ignoring non-positive QuotaPerUnit=" + value)
		return
	}
	*target = parsed
}
