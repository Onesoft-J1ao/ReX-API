package perfmetrics

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifyRelayOutcome(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		name string
		ctx  context.Context
		err  *types.NewAPIError
		want Outcome
	}{
		{"success", context.Background(), nil, OutcomeSuccess},
		{"upstream bad request included by default", context.Background(), types.InitOpenAIError("unknown", 400), OutcomeFailure},
		{"upstream credentials returned as 400", context.Background(), types.InitOpenAIError("invalid_api_key", 400), OutcomeFailure},
		{"wrapped credentials keep their cause", context.Background(), types.NewErrorWithStatusCode(types.InitOpenAIError("invalid_api_key", 400), types.ErrorCodeInvalidRequest, 400), OutcomeFailure},
		{"upstream context limit returned as 500", context.Background(), types.InitOpenAIError("context_length_exceeded", 500), OutcomeIgnored},
		{"local rate limit", context.Background(), types.NewErrorWithStatusCode(errors.New("limited"), types.ErrorCodeInvalidRequest, 429), OutcomeIgnored},
		{"upstream rate limit", context.Background(), types.InitOpenAIError("rate_limit_exceeded", 429), OutcomeFailure},
		{"local quota", context.Background(), types.NewError(errors.New("quota"), types.ErrorCodeInsufficientUserQuota), OutcomeIgnored},
		{"local violation fee", context.Background(), types.NewError(errors.New("csam"), types.ErrorCodeViolationFeeGrokCSAM), OutcomeIgnored},
		{"upstream quota", context.Background(), types.InitOpenAIError("insufficient_quota", 429), OutcomeFailure},
		{"upstream gateway quota", context.Background(), types.InitOpenAIError(types.ErrorCodeInsufficientUserQuota, 403), OutcomeFailure},
		{"unavailable channel", context.Background(), types.NewErrorWithStatusCode(errors.New("disabled"), types.ErrorCodeGetChannelFailed, 403), OutcomeFailure},
		{"empty upstream response", context.Background(), types.NewError(errors.New("empty"), types.ErrorCodeEmptyResponse), OutcomeFailure},
		{"network failure", context.Background(), types.NewOpenAIError(errors.New("connection refused"), types.ErrorCodeDoRequestFailed, 500), OutcomeFailure},
		{"client cancellation", canceled, types.NewOpenAIError(errors.New("context canceled"), types.ErrorCodeDoRequestFailed, 500), OutcomeIgnored},
		{"upstream deadline", context.Background(), types.NewOpenAIError(context.DeadlineExceeded, types.ErrorCodeDoRequestFailed, 504), OutcomeFailure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ClassifyRelayOutcome(tc.ctx, &relaycommon.RelayInfo{}, tc.err))
		})
	}
	assert.Equal(t, OutcomeIgnored, ClassifyRelayOutcome(context.Background(), &relaycommon.RelayInfo{PerformanceBusinessRejection: true}, nil))
}

func TestStreamOutcomeClassification(t *testing.T) {
	for _, tc := range []struct {
		name string
		mark func(*relaycommon.StreamStatus)
		end  relaycommon.StreamEndReason
		want Outcome
	}{
		{"completed", (*relaycommon.StreamStatus).MarkCompleted, relaycommon.StreamEndReasonEOF, OutcomeSuccess},
		{"business rejection", func(s *relaycommon.StreamStatus) { s.MarkFailed("context_length_exceeded", "", 0) }, relaycommon.StreamEndReasonEOF, OutcomeIgnored},
		{"service error", func(s *relaycommon.StreamStatus) { s.MarkFailed("server_error", "", 0) }, relaycommon.StreamEndReasonEOF, OutcomeFailure},
		{"error after completion", func(s *relaycommon.StreamStatus) { s.MarkCompleted(); s.MarkFailed("", "server_error", 0) }, relaycommon.StreamEndReasonEOF, OutcomeFailure},
		{"output limit", func(s *relaycommon.StreamStatus) { s.MarkIncomplete("max_output_tokens") }, relaycommon.StreamEndReasonEOF, OutcomeSuccess},
		{"content filter", func(s *relaycommon.StreamStatus) { s.MarkIncomplete("content_filter") }, relaycommon.StreamEndReasonEOF, OutcomeIgnored},
		{"client cancel", (*relaycommon.StreamStatus).MarkCancelled, relaycommon.StreamEndReasonEOF, OutcomeIgnored},
		{"client gone", nil, relaycommon.StreamEndReasonClientGone, OutcomeIgnored},
		{"timeout", nil, relaycommon.StreamEndReasonTimeout, OutcomeFailure},
		{"missing terminal", nil, relaycommon.StreamEndReasonEOF, OutcomeFailure},
		{"done marker without terminal", nil, relaycommon.StreamEndReasonDone, OutcomeSuccess},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stream := relaycommon.NewStreamStatus()
			stream.RequireTerminal()
			if tc.mark != nil {
				tc.mark(stream)
			}
			stream.SetEndReason(tc.end, nil)
			assert.Equal(t, tc.want, ClassifyRelayOutcome(context.Background(), &relaycommon.RelayInfo{StreamStatus: stream}, nil))
		})
	}
}

func TestClientCancellationDuringUpstreamRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stream := relaycommon.NewStreamStatus()
	stream.RequireTerminal()
	stream.SetEndReason(relaycommon.StreamEndReasonScannerErr, errors.New("reader closed"))
	assert.Equal(t, OutcomeIgnored, ClassifyRelayOutcome(ctx, &relaycommon.RelayInfo{StreamStatus: stream}, nil))

	deadline := relaycommon.NewStreamStatus()
	deadline.SetEndReason(relaycommon.StreamEndReasonClientGone, context.DeadlineExceeded)
	assert.Equal(t, OutcomeFailure, ClassifyRelayOutcome(context.Background(), &relaycommon.RelayInfo{StreamStatus: deadline}, nil))
}

func TestPerformanceWindowIncludesCurrentHour(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 37, 0, 0, time.UTC)
	start, end := queryWindow(now, 24)
	assert.Equal(t, time.Date(2026, 9, 13, 13, 0, 0, 0, time.UTC).Unix(), start)
	assert.Equal(t, now.Unix(), end)
}

func TestHourlySuccessSeriesWeightsSmallerBuckets(t *testing.T) {
	points := recentSuccessSeries(map[int64]counters{
		3600: {requestCount: 100, successCount: 100},
		3900: {requestCount: 1},
		7200: {requestCount: 1, successCount: 1},
	})
	assert.Equal(t, []SuccessRatePoint{{Ts: 3600, SuccessRate: 99.01}, {Ts: 7200, SuccessRate: 100}}, points)
}

// Terminal task sampling: success/failure counts, end-to-end latency, and
// token throughput only for successful tasks that report tokens.
func TestRecordTaskResultSamplesTerminalTasks(t *testing.T) {
	hotBuckets.Clear()
	t.Cleanup(func() { hotBuckets.Clear() })
	now := time.Now().Unix()
	RecordTaskResult(&model.Task{
		Status:     model.TaskStatusSuccess,
		Group:      "a",
		SubmitTime: now - 120,
		StartTime:  now - 100,
		FinishTime: now,
		Properties: model.Properties{OriginModelName: "video-model"},
	}, &relaycommon.TaskInfo{TotalTokens: 5000})
	RecordTaskResult(&model.Task{
		Status:     model.TaskStatusFailure,
		Group:      "a",
		SubmitTime: now - 60,
		FinishTime: now,
		Properties: model.Properties{OriginModelName: "video-model"},
	}, relaycommon.FailTaskInfo("boom"))
	RecordTaskResult(&model.Task{Status: model.TaskStatusSuccess, FinishTime: now}, nil)

	merged := map[bucketKey]counters{}
	hotBuckets.Range(func(key, value any) bool {
		k := key.(bucketKey)
		require.Equal(t, "video-model", k.model)
		require.Equal(t, "a", k.group)
		k.bucketTs = 0
		mergeCounters(merged, k, value.(*atomicBucket).snapshot())
		return true
	})
	assert.Equal(t, counters{
		requestCount:   2,
		successCount:   1,
		totalLatencyMs: 180000,
		outputTokens:   5000,
		generationMs:   100000,
	}, merged[bucketKey{model: "video-model", group: "a"}])
}

// TEST_PERF_MYSQL_DSN / TEST_PERF_POSTGRES_DSN optionally run the aggregation
// against isolated real MySQL/PostgreSQL databases.
func TestPerformanceAggregationAndFlush(t *testing.T) {
	for _, dialect := range []struct{ name, env string }{
		{"sqlite", ""}, {"mysql", "TEST_PERF_MYSQL_DSN"}, {"postgres", "TEST_PERF_POSTGRES_DSN"},
	} {
		t.Run(dialect.name, func(t *testing.T) {
			dsn := ""
			if dialect.env != "" {
				dsn = os.Getenv(dialect.env)
				if dsn == "" {
					t.Skip("isolated test database DSN is not configured")
				}
			}
			t.Setenv("SQL_DSN", dsn)
			// The summary mirrors a third-party instance's public summary. A
			// test must not reach the network: the mirrored models would be
			// merged into the result and the assertions below would depend on
			// whatever the upstream happens to serve that day.
			previousSetting := perf_metrics_setting.GetSetting()
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
				"perf_metrics_setting.upstream_enabled": "false",
			}))
			t.Cleanup(func() {
				require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
					"perf_metrics_setting.upstream_enabled": strconv.FormatBool(previousSetting.UpstreamEnabled),
				}))
			})
			oldDB, oldPath, oldMaster, oldRedis := model.DB, common.SQLitePath, common.IsMasterNode, common.RedisEnabled
			oldType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
			common.SQLitePath, common.IsMasterNode, common.RedisEnabled = filepath.Join(t.TempDir(), "perf.db"), false, false
			hotBuckets.Clear()
			t.Cleanup(func() {
				model.DB, common.SQLitePath, common.IsMasterNode, common.RedisEnabled = oldDB, oldPath, oldMaster, oldRedis
				common.SetDatabaseTypes(oldType, oldLogType)
				hotBuckets.Clear()
			})
			require.NoError(t, model.InitDB())
			db := model.DB
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			require.NoError(t, db.Migrator().DropTable(&model.PerfMetric{}))
			require.NoError(t, db.AutoMigrate(&model.PerfMetric{}))

			now := time.Now()
			start, _ := queryWindow(now, 24)
			hour := now.Unix() - now.Unix()%3600 - 3600
			// Historical counters remain usable without reclassification or migration.
			for _, row := range []model.PerfMetric{
				{ModelName: "test-model", Group: "a", BucketTs: hour, RequestCount: 100, SuccessCount: 100, TotalLatencyMs: 100000, TtftCount: 100, TtftSumMs: 10000, OutputTokens: 200, GenerationMs: 40000},
				{ModelName: "test-model", Group: "inactive", BucketTs: hour, RequestCount: 100},
				{ModelName: "test-model", Group: "a", BucketTs: start - 3600, RequestCount: 100},
			} {
				require.NoError(t, model.UpsertPerfMetric(&row))
			}
			groups := []string{"a", "b"}

			RecordRelayResult(context.Background(), &relaycommon.RelayInfo{OriginModelName: "test-model", UsingGroup: "b", StartTime: now}, types.InitOpenAIError("context_length_exceeded", 400))
			businessRejected, err := QuerySummaryAll(24, groups)
			require.NoError(t, err)
			require.NotNil(t, businessRejected.Summary)
			assert.Equal(t, 100.0, businessRejected.Summary.SuccessRate)

			failure := &atomicBucket{}
			failure.add(Sample{LatencyMs: 2000})
			hotBuckets.Store(bucketKey{model: "test-model", group: "b", bucketTs: hour}, failure)
			before, err := Query(QueryParams{Model: "test-model", Hours: 24, AllowedGroups: groups})
			require.NoError(t, err)
			require.NotNil(t, before.Summary)
			assert.Equal(t, Summary{SuccessRate: 99.01, AvgLatencyMs: 1009, AvgTps: 5}, *before.Summary)
			assert.Equal(t, start, before.WindowStart)
			require.Len(t, before.Series, 1)
			assert.Equal(t, hour, before.Series[0].Ts)
			assert.InDelta(t, 99.01, before.Series[0].SuccessRate, 0.01)
			require.Len(t, before.Groups, 2)

			summary, err := QuerySummaryAll(24, groups)
			require.NoError(t, err)
			assert.Equal(t, before.Summary, summary.Summary)
			require.Len(t, summary.Models, 1)
			assert.Equal(t, 99.01, summary.Models[0].SuccessRate)
			assert.Equal(t, 99.01, summary.Models[0].RecentSuccessSeries[0].SuccessRate)
			// The model-wide figure above blends both groups. The per-group split
			// must keep them apart: group "a" carried only successes while group
			// "b" contributed the one failure, so a card scoped to either group
			// reads its own number rather than the blend.
			require.Len(t, summary.GroupModels, 2)
			byGroup := map[string]GroupModelSummary{}
			for _, row := range summary.GroupModels {
				byGroup[row.Group] = row
			}
			require.Contains(t, byGroup, "a")
			require.Contains(t, byGroup, "b")
			assert.Equal(t, "test-model", byGroup["a"].ModelName)
			assert.InDelta(t, 100.0, byGroup["a"].SuccessRate, 0.01)
			assert.InDelta(t, 0.0, byGroup["b"].SuccessRate, 0.01)
			assert.NotEqual(t, byGroup["a"].SuccessRate, byGroup["b"].SuccessRate,
				"two groups must not report one model-wide number")
			encoded, err := common.Marshal(summary)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), "request_count")
			assert.NotContains(t, string(encoded), "success_count")

			flushCompletedBuckets()
			flushCompletedBuckets()
			after, err := Query(QueryParams{Model: "test-model", Hours: 24, AllowedGroups: groups})
			require.NoError(t, err)
			assert.Equal(t, before.Summary, after.Summary)
			assert.Equal(t, before.Series, after.Series)

			RecordRelayResult(context.Background(), &relaycommon.RelayInfo{OriginModelName: "test-model", UsingGroup: "a", StartTime: now}, types.InitOpenAIError("context_length_exceeded", 400))
			after, err = Query(QueryParams{Model: "test-model", Hours: 24, AllowedGroups: groups})
			require.NoError(t, err)
			assert.Equal(t, before.Summary, after.Summary)
			onlyA, err := Query(QueryParams{Model: "test-model", Group: "a", Hours: 24, AllowedGroups: groups})
			require.NoError(t, err)
			assert.Equal(t, 100.0, onlyA.Summary.SuccessRate)
			empty, err := Query(QueryParams{Model: "missing", Hours: 24, AllowedGroups: groups})
			require.NoError(t, err)
			assert.Nil(t, empty.Summary)
			assert.Empty(t, empty.Series)

			require.NoError(t, model.UpsertPerfMetric(&model.PerfMetric{ModelName: "second-model", Group: "a", BucketTs: hour, RequestCount: 1}))
			combined, err := QuerySummaryAll(24, groups)
			require.NoError(t, err)
			assert.Equal(t, 98.04, combined.Summary.SuccessRate)
			assert.Equal(t, 99.01, combined.Models[0].SuccessRate)
		})
	}
}

func TestUpstreamDetailSeriesMergesLatencyAndSuccess(t *testing.T) {
	series := upstreamDetailSeries(ModelSummary{
		RecentSuccessSeries: []SuccessRatePoint{
			{Ts: 20, SuccessRate: 90},
			{Ts: 10, SuccessRate: 100},
		},
		RecentLatencySeries: []LatencyPoint{
			{Ts: 10, LatencyMs: 400},
			{Ts: 30, LatencyMs: 250},
		},
	})
	require.Len(t, series, 3)
	assert.Equal(t, int64(10), series[0].Ts)
	assert.Equal(t, 100.0, series[0].SuccessRate)
	assert.Equal(t, int64(400), series[0].AvgLatencyMs)
	assert.Equal(t, int64(0), series[0].AvgTtftMs)
	assert.Equal(t, int64(20), series[1].Ts)
	assert.Equal(t, 90.0, series[1].SuccessRate)
	assert.Equal(t, int64(30), series[2].Ts)
	assert.Equal(t, int64(250), series[2].AvgLatencyMs)
}

func TestUpstreamDetailSeriesFillsAvgLatencyWhenMissing(t *testing.T) {
	series := upstreamDetailSeries(ModelSummary{
		AvgLatencyMs:        3210,
		RecentSuccessSeries: []SuccessRatePoint{{Ts: 10, SuccessRate: 100}, {Ts: 20, SuccessRate: 90}},
	})
	require.Len(t, series, 2)
	assert.Equal(t, int64(3210), series[0].AvgLatencyMs)
	assert.Equal(t, int64(3210), series[1].AvgLatencyMs)
}

// The mirrored summary lists every model the upstream instance serves, which is
// not the set this instance advertises. Merging it unfiltered put a retired
// model back on the perf pages after the model square had already dropped it.
func TestMergeUpstreamModelsSkipsModelsThePlazaNoLongerServes(t *testing.T) {
	seedUpstreamMirror(t, 24, ModelSummary{ModelName: "still-served"}, ModelSummary{ModelName: "retired-model"})

	local := []ModelSummary{{ModelName: "local-only", RequestCount: 5}}
	served := map[string]struct{}{"still-served": {}, "local-only": {}}

	names := summaryModelNames(mergeUpstreamModelsServedBy(local, 24, served))

	assert.Contains(t, names, "still-served",
		"a model the group still serves keeps its mirrored row")
	assert.Contains(t, names, "local-only",
		"our own traffic is never filtered out")
	assert.NotContains(t, names, "retired-model",
		"a model the plaza dropped must not come back through the mirror")
}

// A nil served set means the ability lookup came back empty. That is an unknown
// rather than a verdict, so the merge keeps its previous behaviour instead of
// blanking every mirrored model.
func TestMergeUpstreamModelsKeepsMirrorWhenServedSetIsUnknown(t *testing.T) {
	seedUpstreamMirror(t, 24, ModelSummary{ModelName: "retired-model"})

	names := summaryModelNames(mergeUpstreamModelsServedBy(nil, 24, nil))

	assert.Contains(t, names, "retired-model")
}

func TestServedModelNameWithMappingUsesChannelAlias(t *testing.T) {
	served := map[string]struct{}{"deepseek-v4.1-flash-1017": {}}
	mappings := map[string]string{"deepseek-v4.1-flash-1017": "deepseek-v4.1-flash"}

	local, ok := servedModelNameWithMapping(served, mappings, "deepseek-v4.1-flash")
	require.True(t, ok)
	assert.Equal(t, "deepseek-v4.1-flash-1017", local)

	_, ok = servedModelNameWithMapping(served, mappings, "deepseek-v4-pro")
	assert.False(t, ok, "an unrelated upstream model must not match through the alias")
}

// seedUpstreamMirror publishes a mirrored summary into the cache the readers
// consult, and restores the previous mirror and settings afterwards.
func seedUpstreamMirror(t *testing.T, hours int, models ...ModelSummary) {
	t.Helper()
	previous := perf_metrics_setting.GetSetting()
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"perf_metrics_setting.upstream_enabled":     "true",
		"perf_metrics_setting.upstream_summary_url": "http://upstream.test/summary",
	}))

	upstreamSummaryCache.Lock()
	previousEntries := upstreamSummaryCache.entries
	upstreamSummaryCache.entries = map[upstreamCacheKey]upstreamCacheEntry{
		{source: "furry", hours: hours}: {result: SummaryAllResult{Models: models}},
	}
	upstreamSummaryCache.Unlock()

	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
			"perf_metrics_setting.upstream_enabled":     strconv.FormatBool(previous.UpstreamEnabled),
			"perf_metrics_setting.upstream_summary_url": previous.UpstreamSummaryUrl,
		}))
		upstreamSummaryCache.Lock()
		upstreamSummaryCache.entries = previousEntries
		upstreamSummaryCache.Unlock()
	})
}

func summaryModelNames(models []ModelSummary) []string {
	names := make([]string, 0, len(models))
	for _, m := range models {
		names = append(names, m.ModelName)
	}
	return names
}
