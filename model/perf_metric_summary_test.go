package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The model square renders its health badges from the perf-metrics summary, so
// the aggregation must surface a model that has rows in the requested window.
func TestPerfMetricsSummaryBucketsAllReturnsModelsInWindow(t *testing.T) {
	now := time.Now().Unix()
	bucket := now - now%3600

	require.NoError(t, DB.Create(&PerfMetric{
		ModelName: "summary-window-model", Group: "default", BucketTs: bucket,
		RequestCount: 7, SuccessCount: 6, TotalLatencyMs: 1400,
		OutputTokens: 300, GenerationMs: 3000,
	}).Error)

	rows, err := GetPerfMetricsSummaryBucketsAll(now-3600, now+3600, []string{"default"})
	require.NoError(t, err)

	var found *PerfMetricSummaryBucket
	for i := range rows {
		if rows[i].ModelName == "summary-window-model" {
			found = &rows[i]
		}
	}
	require.NotNil(t, found, "a model with rows in the window must be summarised")
	assert.EqualValues(t, 7, found.RequestCount)
	assert.EqualValues(t, 6, found.SuccessCount)
	assert.EqualValues(t, 300, found.OutputTokens)
}

// Rows outside the requested window must not leak into the summary.
func TestPerfMetricsSummaryBucketsAllExcludesRowsOutsideWindow(t *testing.T) {
	now := time.Now().Unix()
	stale := now - 72*3600

	require.NoError(t, DB.Create(&PerfMetric{
		ModelName: "summary-stale-model", Group: "default", BucketTs: stale,
		RequestCount: 5, SuccessCount: 5,
	}).Error)

	rows, err := GetPerfMetricsSummaryBucketsAll(now-3600, now+3600, []string{"default"})
	require.NoError(t, err)
	for _, row := range rows {
		assert.NotEqual(t, "summary-stale-model", row.ModelName,
			"a row older than the window must not appear in the summary")
	}
}
