package pricesentinel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The reference site answers a model it does not price with a placeholder ratio
// instead of omitting it. Keeping that entry would report a free model as a real
// difference and, once applied, price it at 37.5 - which the square renders as
// $75 per million.
func TestFetchUpstreamPricingDropsTheUnpricedPlaceholder(t *testing.T) {
	body := `{"success":true,"data":[
		{"model_name":"free-model","model_ratio":37.5,"completion_ratio":1},
		{"model_name":"gpt-4.5-preview","model_ratio":37.5,"completion_ratio":4},
		{"model_name":"grok-4.6","model_ratio":3,"completion_ratio":2,"cache_ratio":0.5}
	]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	upstream, err := fetchUpstreamPricing(context.Background(), server.URL, 5*time.Second)
	require.NoError(t, err)

	assert.NotContains(t, upstream, "free-model")
	// 37.5 is also a legitimately published ratio, so it survives when the
	// completion ratio is not the placeholder's 1.
	assert.Contains(t, upstream, "gpt-4.5-preview")
	assert.Contains(t, upstream, "grok-4.6")
}
