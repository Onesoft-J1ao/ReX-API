package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runPlaygroundGuard passes one request through the guard and reports whether it
// was let through.
func runPlaygroundGuard(t *testing.T, requestHost, origin, referer string) int {
	t.Helper()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(PlaygroundGuard())
	router.GET("/pg/probe", func(c *gin.Context) { c.Status(http.StatusOK) })

	request := httptest.NewRequest(http.MethodGet, "/pg/probe", nil)
	request.Host = requestHost
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	if referer != "" {
		request.Header.Set("Referer", referer)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder.Code
}

func TestPlaygroundGuardAcceptsOnlyThisSite(t *testing.T) {
	setting := operation_setting.GetPlaygroundSetting()
	restore := *setting
	restoreOrigins := append([]string(nil), setting.AllowedOrigins...)
	t.Cleanup(func() {
		*setting = restore
		setting.AllowedOrigins = restoreOrigins
	})

	setting.Enabled = true
	setting.RequireBrowserOrigin = true
	setting.AllowedOrigins = []string{"https://partner.example.com"}

	tests := []struct {
		name        string
		requestHost string
		origin      string
		referer     string
		wantStatus  int
	}{
		{
			name:        "same host",
			requestHost: "api.example.com",
			origin:      "https://api.example.com",
			wantStatus:  http.StatusOK,
		},
		{
			name:        "same host on another port, as a dev proxy sends",
			requestHost: "localhost:3000",
			origin:      "http://localhost:5173",
			wantStatus:  http.StatusOK,
		},
		{
			name:        "allowlisted origin",
			requestHost: "api.example.com",
			origin:      "https://partner.example.com",
			wantStatus:  http.StatusOK,
		},
		{
			name:        "a page on another host is refused",
			requestHost: "api.example.com",
			origin:      "https://proxy.example.net",
			wantStatus:  http.StatusForbidden,
		},
		{
			name:        "referer stands in for a missing origin",
			requestHost: "api.example.com",
			referer:     "https://api.example.com/playground",
			wantStatus:  http.StatusOK,
		},
		{
			name:        "referer from another host is refused",
			requestHost: "api.example.com",
			referer:     "https://proxy.example.net/embed",
			wantStatus:  http.StatusForbidden,
		},
		{
			name:        "a script that sends neither is refused",
			requestHost: "api.example.com",
			wantStatus:  http.StatusForbidden,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantStatus, runPlaygroundGuard(t, tc.requestHost, tc.origin, tc.referer))
		})
	}
}

func TestPlaygroundGuardRecordsVisitorIdentity(t *testing.T) {
	setting := operation_setting.GetPlaygroundSetting()
	restore := *setting
	t.Cleanup(func() { *setting = restore })
	setting.Enabled = true
	setting.RequireBrowserOrigin = false

	gin.SetMode(gin.TestMode)
	var fingerprint string
	router := gin.New()
	router.Use(PlaygroundGuard())
	router.GET("/pg/probe", func(c *gin.Context) {
		fingerprint = c.GetString("playground_fingerprint")
		c.Status(http.StatusOK)
	})

	request := httptest.NewRequest(http.MethodGet, "/pg/probe", nil)
	request.Header.Set(PlaygroundFingerprintHeader, "visitor-1")
	router.ServeHTTP(httptest.NewRecorder(), request)

	require.Equal(t, http.StatusOK, http.StatusOK)
	assert.Equal(t, "visitor-1", fingerprint)
}
