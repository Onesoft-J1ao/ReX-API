package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func requestClientIP(router http.Handler, remoteAddr string, forwardedFor string) string {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/client-ip", nil)
	request.RemoteAddr = remoteAddr
	if forwardedFor != "" {
		request.Header.Set("X-Forwarded-For", forwardedFor)
	}
	router.ServeHTTP(recorder, request)
	return recorder.Body.String()
}

func newClientIPRouter() *gin.Engine {
	router := gin.New()
	router.GET("/client-ip", func(c *gin.Context) {
		c.String(http.StatusOK, c.ClientIP())
	})
	return router
}

func TestConfigureTrustedProxiesDefaultsToLoopbackOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("TRUSTED_PROXIES", "")
	router := newClientIPRouter()
	require.NoError(t, ConfigureTrustedProxies(router))

	// A same-host proxy is the shape the default exists for.
	for _, remoteAddr := range []string{"127.0.0.1:12345", "[::1]:12345"} {
		clientIP := requestClientIP(router, remoteAddr, "203.0.113.10")
		assert.Equal(t, "203.0.113.10", clientIP, "%s is a trusted proxy", remoteAddr)
	}

	// A peer on the private network is not trusted by default: it could
	// otherwise set its own client IP through X-Forwarded-For and walk past the
	// token IP allowlist and the rate-limit buckets. Such a deployment declares
	// the proxy with TRUSTED_PROXIES.
	for _, testCase := range []struct{ remoteAddr, want string }{
		{"10.20.30.40:12345", "10.20.30.40"},
		{"172.20.0.2:12345", "172.20.0.2"},
		{"192.168.10.2:12345", "192.168.10.2"},
		{"[fd12:3456::2]:12345", "fd12:3456::2"},
	} {
		clientIP := requestClientIP(router, testCase.remoteAddr, "203.0.113.10")
		assert.Equal(t, testCase.want, clientIP, "%s must not be trusted by default", testCase.remoteAddr)
	}
}

func TestConfigureTrustedProxiesDefaultRejectsPublicPeerHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("TRUSTED_PROXIES", " \t ")
	router := newClientIPRouter()
	require.NoError(t, ConfigureTrustedProxies(router))

	clientIP := requestClientIP(router, "198.51.100.10:12345", "203.0.113.10")
	assert.Equal(t, "198.51.100.10", clientIP, "a public peer must not make a spoofed X-Forwarded-For authoritative")
}

// The unset default trusts loopback only. A proxy on the same host connects
// from 127.0.0.1 and its X-Forwarded-For is honoured, while a peer on the
// private network is not a proxy: honouring it would let any host on the LAN
// choose its own client IP, which is what the token IP allowlist and the
// rate-limit buckets are keyed on.
func TestConfigureTrustedProxiesDefaultTrustsLoopbackOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("TRUSTED_PROXIES", "")
	router := newClientIPRouter()
	require.NoError(t, ConfigureTrustedProxies(router))

	fromLoopback := requestClientIP(router, "127.0.0.1:12345", "192.0.2.99, 203.0.113.10")
	assert.Equal(t, "203.0.113.10", fromLoopback, "the first public hop from the trusted proxy must win over a client-supplied prefix")

	fromPrivatePeer := requestClientIP(router, "172.20.0.2:12345", "203.0.113.10")
	assert.Equal(t, "172.20.0.2", fromPrivatePeer, "a private-network peer must not make a spoofed X-Forwarded-For authoritative")
}

func TestConfigureTrustedProxiesNoneDisablesForwardedHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("TRUSTED_PROXIES", " NoNe ")
	router := newClientIPRouter()
	require.NoError(t, ConfigureTrustedProxies(router))

	clientIP := requestClientIP(router, "127.0.0.1:12345", "203.0.113.10")
	assert.Equal(t, "127.0.0.1", clientIP)
}

func TestConfigureTrustedProxiesAcceptsTrimmedIPsAndCIDRs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("TRUSTED_PROXIES", " 192.0.2.0/24, 198.51.100.30 ")
	router := newClientIPRouter()
	require.NoError(t, ConfigureTrustedProxies(router))

	trustedClientIP := requestClientIP(router, "192.0.2.10:12345", "203.0.113.20")
	assert.Equal(t, "203.0.113.20", trustedClientIP)

	trustedExactIP := requestClientIP(router, "198.51.100.30:12345", "203.0.113.21")
	assert.Equal(t, "203.0.113.21", trustedExactIP)

	untrustedClientIP := requestClientIP(router, "198.51.100.20:12345", "203.0.113.22")
	assert.Equal(t, "198.51.100.20", untrustedClientIP)

	defaultProxyIP := requestClientIP(router, "127.0.0.1:12345", "203.0.113.23")
	assert.Equal(t, "127.0.0.1", defaultProxyIP, "an explicit list must replace, not extend, the compatibility defaults")
}

func TestConfigureTrustedProxiesRejectsInvalidConfiguration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	testCases := []struct {
		name  string
		value string
	}{
		{name: "no entries", value: ", ,"},
		{name: "invalid entry", value: "not-an-ip"},
		{name: "mixed valid and invalid entries", value: "127.0.0.1, not-an-ip"},
		{name: "none mixed with valid entry", value: "none,127.0.0.1"},
		{name: "valid entry mixed with none", value: "127.0.0.1,NONE"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("TRUSTED_PROXIES", testCase.value)
			router := newClientIPRouter()
			assert.Error(t, ConfigureTrustedProxies(router))
		})
	}
}
