package middleware

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

// PlaygroundFingerprintHeader carries the browser-side visitor id the free
// allowance is accounted against. Clients that send none fall back to their IP.
const PlaygroundFingerprintHeader = "X-Client-Fingerprint"

// PlaygroundGuard keeps the playground on this site and records who is calling
// it.
//
// The origin check is what stops a third-party page from reverse-proxying the
// playground: a browser sends Origin, or at least Referer, on every playground
// request, and a page on another host carries that other host.
func PlaygroundGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		setting := operation_setting.GetPlaygroundSetting()
		if !setting.Enabled {
			c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "Playground is disabled"})
			c.Abort()
			return
		}

		if setting.RequireBrowserOrigin && !playgroundOriginAllowed(c, setting.AllowedOrigins) {
			c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "Playground requests must come from this site"})
			c.Abort()
			return
		}

		fingerprint := strings.TrimSpace(c.GetHeader(PlaygroundFingerprintHeader))
		if len(fingerprint) > 128 {
			fingerprint = fingerprint[:128]
		}
		common.SetContextKey(c, constant.ContextKeyPlaygroundFingerprint, fingerprint)
		common.SetContextKey(c, constant.ContextKeyPlaygroundIP, c.ClientIP())
		c.Next()
	}
}

// playgroundOriginAllowed accepts a request whose Origin — or, when there is
// none, Referer — belongs to the host the request was sent to, or to an origin
// the deployment allowlisted on top of that. Ports and schemes are ignored so a
// development proxy on another port still matches.
func playgroundOriginAllowed(c *gin.Context, allowed []string) bool {
	origin := c.GetHeader("Origin")
	if origin == "" {
		referer := c.GetHeader("Referer")
		if referer == "" {
			return false
		}
		parsedReferer, err := url.Parse(referer)
		if err != nil || parsedReferer.Host == "" {
			return false
		}
		origin = parsedReferer.Scheme + "://" + parsedReferer.Host
	}

	parsedOrigin, err := url.Parse(origin)
	if err != nil || parsedOrigin.Host == "" {
		return false
	}

	requestHost, err := url.Parse("//" + c.Request.Host)
	if err != nil {
		return false
	}
	if strings.EqualFold(parsedOrigin.Hostname(), requestHost.Hostname()) {
		return true
	}

	for _, candidate := range allowed {
		if strings.EqualFold(strings.TrimRight(candidate, "/"), origin) {
			return true
		}
	}
	return false
}
