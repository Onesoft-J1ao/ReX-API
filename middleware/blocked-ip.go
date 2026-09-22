package middleware

import (
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

// BlockedIPGuard rejects requests from addresses the deployment has banned.
//
// A banned address is matched against the client address the request came from,
// so behind a proxy or tunnel it matches the forwarded client, not the proxy:
// that is the address that keeps re-registering accounts, and it is the one an
// operator can meaningfully ban. Entries are single addresses or CIDR ranges.
func BlockedIPGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		setting := operation_setting.GetSecuritySetting()
		if !setting.BlockedIPEnabled || len(setting.BlockedIPs) == 0 {
			c.Next()
			return
		}

		clientIP := net.ParseIP(c.ClientIP())
		if clientIP == nil {
			c.Next()
			return
		}

		for _, entry := range setting.BlockedIPs {
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue
			}
			blocked := false
			if _, network, err := net.ParseCIDR(entry); err == nil {
				blocked = network.Contains(clientIP)
			} else if banned := net.ParseIP(entry); banned != nil {
				blocked = banned.Equal(clientIP)
			}
			if !blocked {
				continue
			}
			common.SysLog(fmt.Sprintf("blocked request from banned address %s: %s %s", clientIP, c.Request.Method, c.Request.URL.Path))
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "Your address has been blocked",
			})
			return
		}

		c.Next()
	}
}
