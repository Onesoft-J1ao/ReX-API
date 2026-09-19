package perfmetrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withUpstreamSetting(t *testing.T, url string) {
	t.Helper()
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
		resetUpstreamCache()
	})
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"perf_metrics_setting.upstream_summary_url": url,
		"perf_metrics_setting.upstream_enabled":     "true",
	}))
}

func resetUpstreamCache() {
	upstreamSummaryCache.Lock()
	upstreamSummaryCache.entry = upstreamCacheEntry{}
	upstreamSummaryCache.Unlock()
}

func upstreamServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}

// The mirrored upstream is the primary source: it samples models we never
// carried, which is what keeps pay-per-request models off our own probe budget.
func TestMergeUpstreamModelsUpstreamWinsForSharedModels(t *testing.T) {
	server := upstreamServer(t, `{"success":true,"data":{"models":[
		{"model_name":"shared","avg_latency_ms":1234,"success_rate":99.5,"avg_tps":42.5}
	]}}`)
	withUpstreamSetting(t, server.URL)

	local := []ModelSummary{
		{ModelName: "shared", AvgLatencyMs: 9999, SuccessRate: 10, AvgTps: 1, RequestCount: 50},
		{ModelName: "local-only", AvgLatencyMs: 500, SuccessRate: 88, AvgTps: 20, RequestCount: 5},
	}

	merged := mergeUpstreamModels(local, 24)

	byName := map[string]ModelSummary{}
	for _, model := range merged {
		byName[model.ModelName] = model
	}
	require.Len(t, merged, 2)
	assert.EqualValues(t, 1234, byName["shared"].AvgLatencyMs, "the upstream value must replace ours")
	assert.EqualValues(t, 99.5, byName["shared"].SuccessRate)
	assert.EqualValues(t, 500, byName["local-only"].AvgLatencyMs, "a model the upstream omits keeps our data")
}

// A model we have actually served sorts ahead of a mirrored one, because only
// local models carry a request count.
func TestMergeUpstreamModelsSortsLocallyServedFirst(t *testing.T) {
	server := upstreamServer(t, `{"success":true,"data":{"models":[
		{"model_name":"mirrored-a","avg_latency_ms":10,"success_rate":90,"avg_tps":1},
		{"model_name":"mirrored-b","avg_latency_ms":20,"success_rate":90,"avg_tps":1}
	]}}`)
	withUpstreamSetting(t, server.URL)

	merged := mergeUpstreamModels([]ModelSummary{
		{ModelName: "ours", RequestCount: 7},
	}, 24)

	require.Len(t, merged, 3)
	assert.Equal(t, "ours", merged[0].ModelName)
}

// An unreachable upstream must not empty the page: our own numbers still serve.
func TestMergeUpstreamModelsFallsBackWhenUpstreamFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	withUpstreamSetting(t, server.URL)

	local := []ModelSummary{{ModelName: "ours", AvgLatencyMs: 321, RequestCount: 3}}
	merged := mergeUpstreamModels(local, 24)

	require.Len(t, merged, 1)
	assert.Equal(t, "ours", merged[0].ModelName)
	assert.EqualValues(t, 321, merged[0].AvgLatencyMs)
}

// The upstream is a third party; turning it off must leave our data untouched.
func TestMergeUpstreamModelsRespectsDisabledSetting(t *testing.T) {
	withUpstreamSetting(t, "http://127.0.0.1:1/never-called")
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"perf_metrics_setting.upstream_enabled": "false",
	}))

	local := []ModelSummary{{ModelName: "ours", RequestCount: 1}}
	merged := mergeUpstreamModels(local, 24)

	require.Len(t, merged, 1)
	assert.Equal(t, "ours", merged[0].ModelName)
}
