package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// runBlockedIPGuard passes one request through the guard and reports the status.
func runBlockedIPGuard(t *testing.T, remoteAddr, forwardedFor string) int {
	t.Helper()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(BlockedIPGuard())
	router.GET("/v1/models", func(c *gin.Context) { c.Status(http.StatusOK) })

	request := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	request.RemoteAddr = remoteAddr
	if forwardedFor != "" {
		request.Header.Set("X-Forwarded-For", forwardedFor)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder.Code
}

func TestBlockedIPGuard(t *testing.T) {
	setting := operation_setting.GetSecuritySetting()
	restore := *setting
	restoreIPs := append([]string(nil), setting.BlockedIPs...)
	t.Cleanup(func() {
		*setting = restore
		setting.BlockedIPs = restoreIPs
	})

	setting.BlockedIPEnabled = true
	setting.BlockedIPs = []string{"8.219.150.152", "203.0.113.0/24"}

	tests := []struct {
		name         string
		remoteAddr   string
		forwardedFor string
		wantStatus   int
	}{
		{
			name:       "a banned address is refused",
			remoteAddr: "8.219.150.152:44321",
			wantStatus: http.StatusForbidden,
		},
		{
			name:         "the forwarded client is what is matched, not the proxy",
			remoteAddr:   "10.0.0.1:44321",
			forwardedFor: "8.219.150.152",
			wantStatus:   http.StatusForbidden,
		},
		{
			name:       "a banned range is refused",
			remoteAddr: "203.0.113.77:1234",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "an unrelated address is served",
			remoteAddr: "198.51.100.9:1234",
			wantStatus: http.StatusOK,
		},
		{
			name:       "an address next to the banned range is served",
			remoteAddr: "203.0.114.1:1234",
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantStatus, runBlockedIPGuard(t, tc.remoteAddr, tc.forwardedFor))
		})
	}

	t.Run("the switch turns the ban list off", func(t *testing.T) {
		setting.BlockedIPEnabled = false
		assert.Equal(t, http.StatusOK, runBlockedIPGuard(t, "8.219.150.152:44321", ""))
	})
}
