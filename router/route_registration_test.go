package router

import (
	"testing"

	"github.com/gin-gonic/gin"
)

// SetRouter panics when one method+path pair is registered twice, and the panic
// happens at startup rather than under load, so it takes the whole process down.
// Building the router in a test catches a duplicate registration before it can
// reach a running instance.
func TestRouterRegistersEveryPathOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	SetRouter(gin.New(), WebAssets{})
}
