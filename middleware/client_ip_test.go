package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func newIPTestEngine() (*gin.Engine, *string) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.ForwardedByClientIP = true
	_ = engine.SetTrustedProxies([]string{"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12"})
	engine.Use(ResolveClientIP())
	var seen string
	engine.GET("/ip", func(c *gin.Context) {
		seen = c.ClientIP()
		c.Status(http.StatusOK)
	})
	return engine, &seen
}

func doRequest(engine *gin.Engine, remoteAddr string, headers map[string]string) {
	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = remoteAddr
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	engine.ServeHTTP(httptest.NewRecorder(), req)
}

// A Cloudflare Tunnel connects from loopback, so without this the recorded
// address is the tunnel's own origin rather than the visitor.
func TestResolveClientIPUsesCloudflareHeaderFromTrustedPeer(t *testing.T) {
	engine, seen := newIPTestEngine()

	doRequest(engine, "127.0.0.1:5555", map[string]string{
		"CF-Connecting-IP": "203.0.113.9",
	})

	assert.Equal(t, "203.0.113.9", *seen)
}

func TestResolveClientIPIgnoresCloudflareHeaderFromUntrustedPeer(t *testing.T) {
	engine, seen := newIPTestEngine()

	// A direct caller must not be able to claim someone else's address.
	doRequest(engine, "198.51.100.7:5555", map[string]string{
		"CF-Connecting-IP": "203.0.113.9",
	})

	assert.Equal(t, "198.51.100.7", *seen)
}

func TestResolveClientIPLeavesRequestsWithoutTheHeaderAlone(t *testing.T) {
	engine, seen := newIPTestEngine()

	doRequest(engine, "10.1.2.3:5555", map[string]string{
		"X-Forwarded-For": "198.51.100.7",
	})

	assert.Equal(t, "198.51.100.7", *seen)
}

func TestResolveClientIPRejectsMalformedHeaderValue(t *testing.T) {
	engine, seen := newIPTestEngine()

	doRequest(engine, "127.0.0.1:5555", map[string]string{
		"CF-Connecting-IP": "not-an-ip",
	})

	assert.Equal(t, "127.0.0.1", *seen)
}
