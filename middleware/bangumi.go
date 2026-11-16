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
package middleware

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

// BangumiWatchEnabledKey is the option an operator flips to stop visitors from
// watching anime while keeping the broadcast calendar readable.
const BangumiWatchEnabledKey = "BangumiWatchEnabled"

// BangumiWatchOptions reports whether watching is enabled. An unset option
// means enabled, so existing deployments keep their behaviour.
func BangumiWatchOptions() bool {
	common.OptionMapRWMutex.RLock()
	value, ok := common.OptionMap[BangumiWatchEnabledKey]
	common.OptionMapRWMutex.RUnlock()
	return !ok || value != "false"
}

// BangumiWatchEnabled blocks playback-facing bangumi endpoints when the
// operator turned watching off. Catalog endpoints (calendar, ranking, search,
// subject metadata) are registered without this gate on purpose: "only the
// broadcast log" is the mode this switch exists for.
func BangumiWatchEnabled() gin.HandlerFunc {
	return func(c *gin.Context) {
		if BangumiWatchOptions() {
			c.Next()
			return
		}
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "在线观看已关闭，仅提供放送日历",
		})
		c.Abort()
	}
}
