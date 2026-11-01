package perfmetrics

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"

	"github.com/bytedance/gopkg/util/gopool"
)

var hotBuckets sync.Map

// seriesSchema is a stable client cache/schema marker. Do not change it when
// hiding fields or making response-only privacy hardening changes.
const seriesSchema = "dbcd0a3c01b55203"

// upstreamMirrorWindows are the summary windows kept warm in the background.
// The console asks for the 24-hour view, and refreshing only what is asked for
// keeps the loop from polling the third-party upstream for windows nobody reads.
var upstreamMirrorWindows = []int{24}

func Init() {
	go flushLoop()
	go upstreamRefreshLoop(upstreamMirrorWindows)
}

// RecordRelayResult samples one finished relay exactly once, at the request
// boundary, regardless of how many channel attempts it took.
func RecordRelayResult(ctx context.Context, info *relaycommon.RelayInfo, apiErr *types.NewAPIError) {
	if info == nil {
		return
	}
	// Real traffic through a group proves the group is serving, which is what
	// flexible status probing paces itself by. Only successes count: a group
	// that is failing must keep being probed.
	if !info.IsChannelTest && ClassifyRelayOutcome(ctx, info, apiErr) != OutcomeFailure {
		NoteStatusCheckPassiveActivity(info.UsingGroup)
	}
	outcome := ClassifyRelayOutcome(ctx, info, apiErr)
	if outcome == OutcomeIgnored {
		return
	}
	now := time.Now()
	hasTtft := info.IsStream && info.HasSendResponse()
	ttftMs := int64(0)
	if hasTtft {
		ttftMs = info.FirstResponseTime.Sub(info.StartTime).Milliseconds()
	}
	latencyMs := now.Sub(info.StartTime).Milliseconds()
	generationMs := latencyMs
	if hasTtft {
		generationMs = now.Sub(info.FirstResponseTime).Milliseconds()
	}
	if generationMs <= 0 {
		generationMs = latencyMs
	}
	cachedTokens, inputTokens := cacheTokenUsage(info.PerformanceUsage)
	Record(Sample{
		Model:        info.OriginModelName,
		Group:        info.UsingGroup,
		LatencyMs:    latencyMs,
		TtftMs:       ttftMs,
		HasTtft:      hasTtft,
		Success:      outcome == OutcomeSuccess,
		OutputTokens: info.PerformanceOutputTokens,
		GenerationMs: generationMs,
		CacheHit:     hasCacheHit(info.PerformanceUsage),
		CacheSample:  info.PerformanceUsage != nil,
		CachedTokens: cachedTokens,
		InputTokens:  inputTokens,
	})
}

func cacheTokenUsage(usage *dto.Usage) (int64, int64) {
	if usage == nil {
		return 0, 0
	}
	inputTokens := int64(usage.PromptTokens)
	if inputTokens <= 0 {
		inputTokens = int64(usage.InputTokens)
	}
	if inputTokens <= 0 {
		return 0, 0
	}
	cachedTokens := int64(usage.PromptTokensDetails.CachedTokens)
	if cachedTokens <= 0 {
		cachedTokens = int64(usage.PromptCacheHitTokens)
	}
	if cachedTokens <= 0 && usage.InputTokensDetails != nil {
		cachedTokens = int64(usage.InputTokensDetails.CachedTokens)
	}
	if cachedTokens < 0 {
		cachedTokens = 0
	}
	if cachedTokens > inputTokens {
		inputTokens = adjustedCacheInputTokens(inputTokens, cachedTokens)
	}
	return cachedTokens, inputTokens
}

const maxPerfMetricInt64 = int64(1<<63 - 1)

func adjustedCacheInputTokens(inputTokens int64, cachedTokens int64) int64 {
	if cachedTokens <= 0 {
		return inputTokens
	}
	if inputTokens > maxPerfMetricInt64-cachedTokens {
		return maxPerfMetricInt64
	}
	return inputTokens + cachedTokens
}

func hasCacheHit(usage *dto.Usage) bool {
	if usage == nil {
		return false
	}
	if usage.PromptTokensDetails.CachedTokens > 0 || usage.PromptCacheHitTokens > 0 {
		return true
	}
	return usage.InputTokensDetails != nil && usage.InputTokensDetails.CachedTokens > 0
}

// RecordTaskResult samples one async task exactly once, at its terminal
// transition, after the polling status CAS is won. Latency is the end-to-end
// task duration (second resolution, unlike the ms-resolution relay samples);
// throughput is only present when the provider reports tokens.
func RecordTaskResult(task *model.Task, result *relaycommon.TaskInfo) {
	if task == nil {
		return
	}
	modelName := task.Properties.OriginModelName
	if bc := task.PrivateData.BillingContext; bc != nil && bc.OriginModelName != "" {
		modelName = bc.OriginModelName
	}
	endTs := task.FinishTime
	if endTs <= 0 {
		endTs = time.Now().Unix()
	}
	sample := Sample{
		Model:   modelName,
		Group:   task.Group,
		Success: task.Status == model.TaskStatusSuccess,
	}
	if task.SubmitTime > 0 && endTs > task.SubmitTime {
		sample.LatencyMs = (endTs - task.SubmitTime) * 1000
	}
	if sample.Success && result != nil {
		tokens := int64(cmp.Or(result.TotalTokens, result.CompletionTokens))
		genStart := cmp.Or(task.StartTime, task.SubmitTime)
		if tokens > 0 && genStart > 0 && endTs > genStart {
			sample.OutputTokens = tokens
			sample.GenerationMs = (endTs - genStart) * 1000
		}
	}
	Record(sample)
}

func Record(sample Sample) {
	setting := perf_metrics_setting.GetSetting()
	if !setting.Enabled || sample.Model == "" {
		return
	}
	if sample.Group == "" {
		sample.Group = "default"
	}
	if sample.LatencyMs < 0 {
		sample.LatencyMs = 0
	}

	key := bucketKey{
		model:    sample.Model,
		group:    sample.Group,
		bucketTs: bucketStart(time.Now().Unix()),
	}
	actual, _ := hotBuckets.LoadOrStore(key, &atomicBucket{})
	actual.(*atomicBucket).add(sample)
	gopool.Go(func() {
		recordRedis(key, sample)
	})
}

// queryWindow covers the current partial hour plus the preceding complete
// hours, so every reader shares the same hourly buckets.
func queryWindow(now time.Time, hours int) (int64, int64) {
	if hours <= 0 {
		hours = 24
	}
	hours = min(hours, 24*30)
	endTs := now.Unix()
	return endTs - endTs%3600 - int64(hours-1)*3600, endTs
}

func Query(params QueryParams) (QueryResult, error) {
	startTs, endTs := queryWindow(time.Now(), params.Hours)
	allowedGroups := allowedGroupSet(params.AllowedGroups)

	merged := map[bucketKey]counters{}
	rows, err := model.GetPerfMetrics(params.Model, params.Group, startTs, endTs)
	if err != nil {
		return QueryResult{}, err
	}
	for _, row := range rows {
		if allowedGroups != nil {
			if _, ok := allowedGroups[row.Group]; !ok {
				continue
			}
		}
		mergeCounters(merged, bucketKey{
			model:    row.ModelName,
			group:    row.Group,
			bucketTs: row.BucketTs,
		}, counters{
			requestCount:     row.RequestCount,
			successCount:     row.SuccessCount,
			totalLatencyMs:   row.TotalLatencyMs,
			ttftSumMs:        row.TtftSumMs,
			ttftCount:        row.TtftCount,
			outputTokens:     row.OutputTokens,
			generationMs:     row.GenerationMs,
			cacheHitCount:    row.CacheHitCount,
			cacheSampleCount: row.CacheSampleCount,
			cachedTokens:     row.CachedTokens,
			inputTokens:      row.InputTokens,
		})
	}

	hotBuckets.Range(func(key, value any) bool {
		k := key.(bucketKey)
		if k.model != params.Model || k.bucketTs < startTs || k.bucketTs > endTs {
			return true
		}
		if params.Group != "" && k.group != params.Group {
			return true
		}
		if allowedGroups != nil {
			if _, ok := allowedGroups[k.group]; !ok {
				return true
			}
		}
		mergeCounters(merged, k, value.(*atomicBucket).snapshot())
		return true
	})

	result := buildQueryResult(params.Model, merged)
	result.WindowStart, result.WindowEnd = startTs, endTs
	// Models this instance mirrors from upstream but has not carried enough
	// traffic for on its own have no local series. The mirror's own hourly
	// success and latency series fill the detail charts; per-group rows stay
	// empty because the mirror does not split them by our groups.
	if result.Summary == nil {
		if mirrored, ok := upstreamModelSummary(params.Model, params.Hours); ok {
			result.Summary = &Summary{
				AvgLatencyMs: mirrored.AvgLatencyMs,
				SuccessRate:  mirrored.SuccessRate,
				AvgTps:       mirrored.AvgTps,
			}
			result.Series = upstreamDetailSeries(mirrored)
		}
	}
	return result, nil
}

// upstreamDetailSeries turns the mirror's per-model history into the detail
// chart shape. Latency stays in avg_latency_ms: the mirror reports end-to-end
// latency, not time-to-first-token, and the chart falls back to that field.
//
// The mirrored summary has no hourly latency series at all — measured against
// the live upstream, every model returns only recent_success_series plus one
// window-wide avg_latency_ms. A model with no measured hourly latency therefore
// gets that average repeated on each success-rate hour, so the latency chart
// exists for every mirrored model instead of only the ones we probed ourselves.
func upstreamDetailSeries(mirrored ModelSummary) []BucketPoint {
	byTs := make(map[int64]*BucketPoint, len(mirrored.RecentSuccessSeries)+len(mirrored.RecentLatencySeries))
	order := make([]int64, 0, len(mirrored.RecentSuccessSeries)+len(mirrored.RecentLatencySeries))
	pointAt := func(ts int64) *BucketPoint {
		if point, ok := byTs[ts]; ok {
			return point
		}
		point := &BucketPoint{Ts: ts}
		byTs[ts] = point
		order = append(order, ts)
		return point
	}
	for _, point := range mirrored.RecentSuccessSeries {
		pointAt(point.Ts).SuccessRate = point.SuccessRate
	}
	measuredLatency := len(mirrored.RecentLatencySeries) > 0
	for _, point := range mirrored.RecentLatencySeries {
		pointAt(point.Ts).AvgLatencyMs = point.LatencyMs
	}
	if !measuredLatency && mirrored.AvgLatencyMs > 0 {
		for _, point := range byTs {
			point.AvgLatencyMs = mirrored.AvgLatencyMs
		}
	}
	series := make([]BucketPoint, 0, len(order))
	for _, ts := range order {
		series = append(series, *byTs[ts])
	}
	sort.Slice(series, func(i, j int) bool { return series[i].Ts < series[j].Ts })
	return series
}

func QuerySummaryAll(hours int, groups []string) (SummaryAllResult, error) {
	startTs, endTs := queryWindow(time.Now(), hours)
	allowedGroups := allowedGroupSet(groups)

	rows, err := model.GetPerfMetricsSummaryBucketsAll(startTs, endTs, groups)
	if err != nil {
		return SummaryAllResult{}, err
	}

	totals := map[string]counters{}
	modelBuckets := map[string]map[int64]counters{}
	for _, row := range rows {
		if row.ModelName == StatusCheckProbeModel {
			continue
		}
		value := counters{
			requestCount:   row.RequestCount,
			successCount:   row.SuccessCount,
			totalLatencyMs: row.TotalLatencyMs,
			outputTokens:   row.OutputTokens,
			generationMs:   row.GenerationMs,
		}
		mergeModelTotals(totals, row.ModelName, value)
		mergeModelBucket(modelBuckets, row.ModelName, row.BucketTs, value)
	}

	hotBuckets.Range(func(key, value any) bool {
		k := key.(bucketKey)
		if k.model == StatusCheckProbeModel {
			return true
		}
		if k.bucketTs < startTs || k.bucketTs > endTs {
			return true
		}
		if allowedGroups != nil {
			if _, ok := allowedGroups[k.group]; !ok {
				return true
			}
		}
		snap := value.(*atomicBucket).snapshot()
		if snap.requestCount == 0 {
			return true
		}
		mergeModelTotals(totals, k.model, snap)
		mergeModelBucket(modelBuckets, k.model, k.bucketTs, snap)
		return true
	})

	all := counters{}
	models := make([]ModelSummary, 0, len(totals))
	for name, total := range totals {
		if total.requestCount == 0 {
			continue
		}
		all.requestCount += total.requestCount
		all.successCount += total.successCount
		all.totalLatencyMs += total.totalLatencyMs
		all.outputTokens += total.outputTokens
		all.generationMs += total.generationMs
		avgLatency := total.totalLatencyMs / total.requestCount
		successRate := float64(total.successCount) / float64(total.requestCount) * 100
		avgTps := 0.0
		if total.generationMs > 0 {
			avgTps = float64(total.outputTokens) / (float64(total.generationMs) / 1000.0)
		}
		models = append(models, ModelSummary{
			ModelName:           name,
			AvgLatencyMs:        avgLatency,
			SuccessRate:         math.Round(successRate*100) / 100,
			AvgTps:              math.Round(avgTps*100) / 100,
			RecentSuccessSeries: recentSuccessSeries(modelBuckets[name]),
			RecentLatencySeries: recentLatencySeries(modelBuckets[name]),
			RequestCount:        total.requestCount,
		})
	}

	return SummaryAllResult{
		Summary:     summarize(all),
		WindowStart: startTs,
		WindowEnd:   endTs,
		Models:      mergeUpstreamModels(models, hours),
		GroupModels: buildGroupModels(startTs, endTs, groups, allowedGroups),
	}, nil
}

// buildGroupModels splits the same figures by group. It reads the group column
// that QuerySummaryAll's model-wide pass deliberately drops, so a card scoped to
// one group shows that group's own uptime instead of an average across every
// group the model is routed through.
//
// Only locally measured rows are included: the mirrored upstream summary has no
// group dimension at all, so lending its numbers to a group would put another
// deployment's uptime on this group's card.
func buildGroupModels(startTs int64, endTs int64, groups []string, allowed map[string]struct{}) []GroupModelSummary {
	rows, err := model.GetPerfMetricsGroupBucketsAll(startTs, endTs, groups)
	if err != nil {
		return nil
	}

	type key struct{ group, model string }
	totals := map[key]counters{}
	buckets := map[key]map[int64]counters{}
	for _, row := range rows {
		if row.ModelName == StatusCheckProbeModel {
			continue
		}
		if allowed != nil {
			if _, ok := allowed[row.Group]; !ok {
				continue
			}
		}
		k := key{group: row.Group, model: row.ModelName}
		if buckets[k] == nil {
			buckets[k] = map[int64]counters{}
		}
		value := counters{
			requestCount:   row.RequestCount,
			successCount:   row.SuccessCount,
			totalLatencyMs: row.TotalLatencyMs,
			outputTokens:   row.OutputTokens,
			generationMs:   row.GenerationMs,
		}
		current := totals[k]
		mergeStatusCounters(&current, value)
		totals[k] = current
		bucket := buckets[k][row.BucketTs]
		mergeStatusCounters(&bucket, value)
		buckets[k][row.BucketTs] = bucket
	}

	// hot buckets hold in-flight samples for models probed this cycle; without
	// them a freshly probed group reads as empty until the next flush.
	hotBuckets.Range(func(k any, value any) bool {
		bucket := k.(bucketKey)
		if bucket.model == StatusCheckProbeModel {
			return true
		}
		if bucket.bucketTs < startTs || bucket.bucketTs > endTs {
			return true
		}
		if allowed != nil {
			if _, ok := allowed[bucket.group]; !ok {
				return true
			}
		}
		snap := value.(*atomicBucket).snapshot()
		if snap.requestCount == 0 {
			return true
		}
		key := key{group: bucket.group, model: bucket.model}
		if buckets[key] == nil {
			buckets[key] = map[int64]counters{}
		}
		current := totals[key]
		mergeStatusCounters(&current, snap)
		totals[key] = current
		return true
	})

	out := make([]GroupModelSummary, 0, len(totals))
	for k, total := range totals {
		if total.requestCount == 0 {
			continue
		}
		avgTps := 0.0
		if total.generationMs > 0 {
			avgTps = float64(total.outputTokens) / (float64(total.generationMs) / 1000.0)
		}
		out = append(out, GroupModelSummary{
			Group:               k.group,
			ModelName:           k.model,
			AvgLatencyMs:        total.totalLatencyMs / total.requestCount,
			SuccessRate:         math.Round(successRate(total)*100) / 100,
			AvgTps:              math.Round(avgTps*100) / 100,
			RecentSuccessSeries: recentSuccessSeries(buckets[k]),
			RecentLatencySeries: recentLatencySeries(buckets[k]),
		})
	}

	// Upstream summaries are model-wide and have no local group dimension. Map
	// each source only onto groups whose enabled channels use that source, and
	// only for models that group actually serves. This fills cold groups without
	// pretending a different provider's traffic was local traffic.
	localKeys := make(map[key]struct{}, len(out))
	for _, row := range out {
		localKeys[key{group: row.Group, model: row.ModelName}] = struct{}{}
	}
	groupURLs, err := model.GetEnabledGroupBaseURLs(groups)
	if err == nil {
		servedByGroup := make(map[string]map[string]struct{}, len(groups))
		for _, group := range groups {
			servedByGroup[group] = make(map[string]struct{})
			for _, name := range model.GetGroupEnabledModels(group) {
				servedByGroup[group][name] = struct{}{}
			}
		}
		summaries := make(map[string]SummaryAllResult)
		for group, bases := range groupURLs {
			for _, base := range bases {
				source := upstreamSourceForBaseURL(base)
				if source == "" {
					continue
				}
				if _, ok := summaries[source]; !ok {
					if summary, ok := fetchUpstreamSummaryFor(source, 24); ok {
						summaries[source] = summary
					}
				}
				summary, ok := summaries[source]
				if !ok {
					continue
				}
				for _, mirror := range summary.Models {
					if _, served := servedByGroup[group][mirror.ModelName]; !served {
						continue
					}
					k := key{group: group, model: mirror.ModelName}
					if _, exists := localKeys[k]; exists {
						continue
					}
					upstreamGroup := upstreamHealthGroup(source, group)
					if upstreamGroup == "" {
						continue
					}
					detail, ok := fetchUpstreamDetailFor(source, mirror.ModelName, 24)
					if !ok {
						continue
					}
					matchedGroup := false
					for _, detailGroup := range detail.Groups {
						if detailGroup.Group != upstreamGroup {
							continue
						}
						mirror = modelSummaryFromGroup(mirror, detailGroup)
						matchedGroup = true
						break
					}
					if !matchedGroup {
						continue
					}
					out = append(out, GroupModelSummary{
						Group:               group,
						ModelName:           mirror.ModelName,
						AvgLatencyMs:        mirror.AvgLatencyMs,
						SuccessRate:         mirror.SuccessRate,
						AvgTps:              mirror.AvgTps,
						RecentSuccessSeries: mirror.RecentSuccessSeries,
						RecentLatencySeries: mirror.RecentLatencySeries,
						Source:              SourceMirror + ":" + source,
					})
					localKeys[k] = struct{}{}
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Group != out[j].Group {
			return out[i].Group < out[j].Group
		}
		return out[i].ModelName < out[j].ModelName
	})
	return out
}

func upstreamHealthGroup(source, localGroup string) string {
	return perf_metrics_setting.GetSetting().UpstreamHealthGroups[source][localGroup]
}

func modelSummaryFromGroup(model ModelSummary, group GroupResult) ModelSummary {
	model.AvgLatencyMs = group.AvgLatencyMs
	model.SuccessRate = group.SuccessRate
	model.AvgTps = group.AvgTps
	model.RecentSuccessSeries = make([]SuccessRatePoint, 0, len(group.Series))
	model.RecentLatencySeries = make([]LatencyPoint, 0, len(group.Series))
	for _, point := range group.Series {
		model.RecentSuccessSeries = append(model.RecentSuccessSeries, SuccessRatePoint{Ts: point.Ts, SuccessRate: point.SuccessRate})
		model.RecentLatencySeries = append(model.RecentLatencySeries, LatencyPoint{Ts: point.Ts, LatencyMs: point.AvgLatencyMs})
	}
	return model
}

func upstreamSourceForBaseURL(baseURL string) string {
	host := strings.ToLower(baseURL)
	switch {
	case strings.Contains(host, "furry.vg"):
		return "furry"
	case strings.Contains(host, "sheapi"):
		return "sheapi"
	case strings.Contains(host, "mcapple.top"):
		return "mcapple"
	default:
		return ""
	}
}

// SourceMirror marks numbers that came from another deployment's public
// summary. It has no group dimension, so a per-group card must not present it as
// a reading of the group on screen.
const SourceMirror = "mirror"

// mergeUpstreamModels overlays the mirrored upstream summary on our own numbers.
//
// Our own rows always win for a model we served: they are the only figures that
// belong to *this* deployment and to a specific group. The mirror fills in the
// models we carried no traffic for at all, and those entries are marked
// SourceMirror so a group-scoped card can tell the difference between "this group
// is at 43%" and "another instance's blended figure for this model is 43%".
//
// Upstream entries carry no request count, so they sort after models we have
// actually served; ordering stays "busiest local model first".
func mergeUpstreamModels(local []ModelSummary, hours int) []ModelSummary {
	return mergeUpstreamModelsServedBy(local, hours, servedModelNames())
}

// servedModelNames returns the models an enabled channel still serves, which is
// the set the model square advertises. It returns nil when nothing is served or
// the lookup came back empty, and a nil set disables the filter so a database
// hiccup degrades to the previous behaviour instead of blanking the page.
func servedModelNames() map[string]struct{} {
	// GetEnabledModels reads through the shared handle and panics when no
	// database has been opened, which is the state a caller that only merges a
	// mirrored summary runs in. Absent abilities are unknown, not a verdict.
	if model.DB == nil {
		return nil
	}
	names := model.GetEnabledModels()
	if len(names) == 0 {
		return nil
	}
	served := make(map[string]struct{}, len(names))
	for _, name := range names {
		served[name] = struct{}{}
	}
	return served
}

// mergeUpstreamModelsServedBy is mergeUpstreamModels with the served set passed
// in, so the filter can be exercised without a database.
func mergeUpstreamModelsServedBy(local []ModelSummary, hours int, served map[string]struct{}) []ModelSummary {
	upstream, ok := fetchUpstreamSummary(hours)
	if !ok {
		sort.Slice(local, func(i, j int) bool {
			return local[i].RequestCount > local[j].RequestCount
		})
		return local
	}

	merged := make([]ModelSummary, 0, len(local)+len(upstream.Models))
	localNames := make(map[string]struct{}, len(local))
	for _, model := range local {
		localNames[model.ModelName] = struct{}{}
	}
	for _, model := range upstream.Models {
		if model.ModelName == "" {
			continue
		}
		// The mirror reports every model its own instance serves, which is not
		// the set we advertise: a model whose abilities were all disabled still
		// arrives in the mirror, and merging it would put it back on the perf
		// pages after the model square had already dropped it.
		if served != nil {
			if _, ok := served[model.ModelName]; !ok {
				continue
			}
		}
		// Our own rows win. They are the only figures about this deployment, and
		// the only ones that can be split by group; letting the mirror overwrite
		// them is what made every group show one instance's blended number.
		if _, ours := localNames[model.ModelName]; ours {
			continue
		}
		model.RequestCount = 0
		model.Source = SourceMirror
		merged = append(merged, model)
	}
	merged = append(merged, local...)
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].RequestCount > merged[j].RequestCount
	})
	return merged
}

func recentLatencySeries(buckets map[int64]counters) []LatencyPoint {
	if len(buckets) == 0 {
		return nil
	}
	hourly := map[int64]counters{}
	for ts, value := range buckets {
		hourTs := ts - ts%3600
		merged := hourly[hourTs]
		merged.requestCount += value.requestCount
		merged.totalLatencyMs += value.totalLatencyMs
		hourly[hourTs] = merged
	}
	timestamps := make([]int64, 0, len(hourly))
	for hourTs, value := range hourly {
		if value.requestCount == 0 {
			continue
		}
		timestamps = append(timestamps, hourTs)
	}
	if len(timestamps) == 0 {
		return nil
	}
	sort.Slice(timestamps, func(i, j int) bool {
		return timestamps[i] < timestamps[j]
	})
	points := make([]LatencyPoint, 0, len(timestamps))
	for _, hourTs := range timestamps {
		points = append(points, LatencyPoint{
			Ts:        hourTs,
			LatencyMs: hourly[hourTs].totalLatencyMs / hourly[hourTs].requestCount,
		})
	}
	return points
}

func mergeModelTotals(totals map[string]counters, modelName string, value counters) {
	if value.requestCount == 0 {
		return
	}
	current := totals[modelName]
	current.requestCount += value.requestCount
	current.successCount += value.successCount
	current.totalLatencyMs += value.totalLatencyMs
	current.ttftSumMs += value.ttftSumMs
	current.ttftCount += value.ttftCount
	current.outputTokens += value.outputTokens
	current.generationMs += value.generationMs
	totals[modelName] = current
}

func mergeModelBucket(modelBuckets map[string]map[int64]counters, modelName string, bucketTs int64, value counters) {
	if value.requestCount == 0 {
		return
	}
	if _, ok := modelBuckets[modelName]; !ok {
		modelBuckets[modelName] = map[int64]counters{}
	}
	current := modelBuckets[modelName][bucketTs]
	current.requestCount += value.requestCount
	current.successCount += value.successCount
	current.totalLatencyMs += value.totalLatencyMs
	current.ttftSumMs += value.ttftSumMs
	current.ttftCount += value.ttftCount
	current.outputTokens += value.outputTokens
	current.generationMs += value.generationMs
	modelBuckets[modelName][bucketTs] = current
}

func recentSuccessSeries(buckets map[int64]counters) []SuccessRatePoint {
	if len(buckets) == 0 {
		return nil
	}
	hourly := map[int64]counters{}
	for ts, value := range buckets {
		hourTs := ts - ts%3600
		merged := hourly[hourTs]
		merged.requestCount += value.requestCount
		merged.successCount += value.successCount
		hourly[hourTs] = merged
	}
	timestamps := make([]int64, 0, len(hourly))
	for hourTs, value := range hourly {
		if value.requestCount == 0 {
			continue
		}
		timestamps = append(timestamps, hourTs)
	}
	if len(timestamps) == 0 {
		return nil
	}
	sort.Slice(timestamps, func(i, j int) bool {
		return timestamps[i] < timestamps[j]
	})
	points := make([]SuccessRatePoint, 0, len(timestamps))
	for _, hourTs := range timestamps {
		points = append(points, SuccessRatePoint{
			Ts:          hourTs,
			SuccessRate: math.Round(successRate(hourly[hourTs])*100) / 100,
		})
	}
	return points
}

func allowedGroupSet(groups []string) map[string]struct{} {
	if groups == nil {
		return nil
	}
	allowed := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		allowed[group] = struct{}{}
	}
	return allowed
}

func bucketStart(ts int64) int64 {
	bucketSeconds := perf_metrics_setting.GetBucketSeconds()
	if bucketSeconds <= 0 {
		bucketSeconds = 3600
	}
	return ts - (ts % bucketSeconds)
}

func mergeCounters(merged map[bucketKey]counters, key bucketKey, value counters) {
	if value.requestCount == 0 {
		return
	}
	current := merged[key]
	current.requestCount += value.requestCount
	current.successCount += value.successCount
	current.totalLatencyMs += value.totalLatencyMs
	current.ttftSumMs += value.ttftSumMs
	current.ttftCount += value.ttftCount
	current.outputTokens += value.outputTokens
	current.generationMs += value.generationMs
	current.cacheHitCount += value.cacheHitCount
	current.cacheSampleCount += value.cacheSampleCount
	current.cachedTokens += value.cachedTokens
	current.inputTokens += value.inputTokens
	merged[key] = current
}

func buildQueryResult(modelName string, merged map[bucketKey]counters) QueryResult {
	groupBuckets := map[string]map[int64]counters{}
	modelBuckets := map[string]map[int64]counters{}
	for key, value := range merged {
		if value.requestCount == 0 {
			continue
		}
		if _, ok := groupBuckets[key.group]; !ok {
			groupBuckets[key.group] = map[int64]counters{}
		}
		groupBuckets[key.group][key.bucketTs] = value
		mergeModelBucket(modelBuckets, modelName, key.bucketTs, value)
	}

	groups := make([]string, 0, len(groupBuckets))
	for group := range groupBuckets {
		groups = append(groups, group)
	}
	sort.Strings(groups)

	all := counters{}
	results := make([]GroupResult, 0, len(groups))
	for _, group := range groups {
		buckets := groupBuckets[group]
		timestamps := make([]int64, 0, len(buckets))
		for ts := range buckets {
			timestamps = append(timestamps, ts)
		}
		sort.Slice(timestamps, func(i, j int) bool {
			return timestamps[i] < timestamps[j]
		})

		total := counters{}
		series := make([]BucketPoint, 0, len(timestamps))
		for _, ts := range timestamps {
			value := buckets[ts]
			total.requestCount += value.requestCount
			total.successCount += value.successCount
			total.totalLatencyMs += value.totalLatencyMs
			total.ttftSumMs += value.ttftSumMs
			total.ttftCount += value.ttftCount
			total.outputTokens += value.outputTokens
			total.generationMs += value.generationMs
			series = append(series, bucketPoint(ts, value))
		}
		all.requestCount += total.requestCount
		all.successCount += total.successCount
		all.totalLatencyMs += total.totalLatencyMs
		all.outputTokens += total.outputTokens
		all.generationMs += total.generationMs

		results = append(results, GroupResult{
			Group:        group,
			AvgTtftMs:    avg(total.ttftSumMs, total.ttftCount),
			AvgLatencyMs: avg(total.totalLatencyMs, total.requestCount),
			SuccessRate:  successRate(total),
			AvgTps:       avgTps(total),
			Series:       series,
		})
	}

	modelSeries := make([]BucketPoint, 0, len(modelBuckets[modelName]))
	for ts, value := range modelBuckets[modelName] {
		modelSeries = append(modelSeries, bucketPoint(ts, value))
	}
	sort.Slice(modelSeries, func(i, j int) bool {
		return modelSeries[i].Ts < modelSeries[j].Ts
	})

	return QueryResult{
		ModelName:    modelName,
		SeriesSchema: seriesSchema,
		Summary:      summarize(all),
		Series:       modelSeries,
		Groups:       results,
	}
}

// summarize returns nil when nothing was sampled, so clients can show
// "no data" instead of a fabricated 0% success rate.
func summarize(total counters) *Summary {
	if total.requestCount <= 0 {
		return nil
	}
	return &Summary{
		AvgLatencyMs: avg(total.totalLatencyMs, total.requestCount),
		SuccessRate:  math.Round(successRate(total)*100) / 100,
		AvgTps:       math.Round(avgTps(total)*100) / 100,
	}
}

func bucketPoint(ts int64, value counters) BucketPoint {
	return BucketPoint{
		Ts:           ts,
		AvgTtftMs:    avg(value.ttftSumMs, value.ttftCount),
		AvgLatencyMs: avg(value.totalLatencyMs, value.requestCount),
		SuccessRate:  successRate(value),
		AvgTps:       avgTps(value),
	}
}

func avg(sum int64, count int64) int64 {
	if count <= 0 {
		return 0
	}
	return sum / count
}

func successRate(value counters) float64 {
	if value.requestCount <= 0 {
		return 0
	}
	return float64(value.successCount) / float64(value.requestCount) * 100
}

func avgTps(value counters) float64 {
	if value.outputTokens <= 0 || value.generationMs <= 0 {
		return 0
	}
	return float64(value.outputTokens) / (float64(value.generationMs) / 1000)
}

func recordRedis(key bucketKey, sample Sample) {
	if !common.RedisEnabled || common.RDB == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	redisKey := redisBucketKey(key)
	pipe := common.RDB.TxPipeline()
	pipe.HIncrBy(ctx, redisKey, "req", 1)
	if sample.Success {
		pipe.HIncrBy(ctx, redisKey, "ok", 1)
	}
	if sample.LatencyMs > 0 {
		pipe.HIncrBy(ctx, redisKey, "lat", sample.LatencyMs)
	}
	if sample.HasTtft && sample.TtftMs >= 0 {
		pipe.HIncrBy(ctx, redisKey, "ttft", sample.TtftMs)
		pipe.HIncrBy(ctx, redisKey, "ttft_n", 1)
	}
	if sample.OutputTokens > 0 && sample.GenerationMs > 0 {
		pipe.HIncrBy(ctx, redisKey, "out", sample.OutputTokens)
		pipe.HIncrBy(ctx, redisKey, "gen_ms", sample.GenerationMs)
	}
	if sample.CacheHit {
		pipe.HIncrBy(ctx, redisKey, "cache", 1)
	}
	if sample.CacheSample {
		pipe.HIncrBy(ctx, redisKey, "cache_n", 1)
	}
	if sample.InputTokens > 0 {
		pipe.HIncrBy(ctx, redisKey, "input_tok", sample.InputTokens)
		if sample.CachedTokens > 0 {
			pipe.HIncrBy(ctx, redisKey, "cache_tok", sample.CachedTokens)
		}
	}
	pipe.Expire(ctx, redisKey, time.Hour)
	_, _ = pipe.Exec(ctx)
}

func mergeRedisActiveBuckets(merged map[bucketKey]counters, params QueryParams, startTs int64, endTs int64) {
	if !common.RedisEnabled || common.RDB == nil || params.Model == "" || params.Group == "" {
		return
	}
	active := bucketStart(time.Now().Unix())
	if active < startTs || active > endTs {
		return
	}
	key := bucketKey{model: params.Model, group: params.Group, bucketTs: active}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	values, err := common.RDB.HGetAll(ctx, redisBucketKey(key)).Result()
	if err != nil || len(values) == 0 {
		return
	}
	mergeCounters(merged, key, redisCounters(values))
}

func redisBucketKey(key bucketKey) string {
	return fmt.Sprintf("perf:%s:%s:%d", key.model, key.group, key.bucketTs)
}

// RecordedSampleCount reports the number of samples buffered in memory for the
// given model and group in the current bucket. It exists so callers outside this
// package can assert that recording happened without reaching into hotBuckets.
func RecordedSampleCount(modelName string, group string) int64 {
	if group == "" {
		group = "default"
	}
	key := bucketKey{
		model:    modelName,
		group:    group,
		bucketTs: bucketStart(time.Now().Unix()),
	}
	value, ok := hotBuckets.Load(key)
	if !ok {
		return 0
	}
	return value.(*atomicBucket).snapshot().requestCount
}
