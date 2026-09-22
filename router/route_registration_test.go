package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildTestRouter returns a router carrying every registered route, with no
// server started behind it.
func buildTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetRouter(engine, WebAssets{})
	return engine
}

// SetRouter panics when one method+path pair is registered twice, and the panic
// happens at startup rather than under load, so it takes the whole process down.
// Building the router in a test catches a duplicate registration before it can
// reach a running instance.
func TestRouterRegistersEveryPathOnce(t *testing.T) {
	buildTestRouter(t)
}

// The mailed edit link proves the request came from us; it is not an admin
// credential. Writing a price therefore also requires a root session or access
// token, so a forwarded or leaked link cannot change billing on its own.
func TestPriceSentinelApplyRequiresAdminCredentials(t *testing.T) {
	engine := buildTestRouter(t)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/price-sentinel/edit/apply",
		strings.NewReader(`{"token":"mailed","model_name":"m","field":"model_ratio","value":1}`),
	)
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, request)

	require.NotEqual(t, http.StatusOK, recorder.Code,
		"an unauthenticated apply must not be accepted")
	assert.Contains(t,
		[]int{http.StatusUnauthorized, http.StatusForbidden},
		recorder.Code,
		"an unauthenticated apply must be rejected as unauthenticated, not treated as a missing route")
}
