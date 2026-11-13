package controller

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/status_check_setting"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

// ------------------- GET /api/status-check -------------------

// groupHealthResponse mirrors what fuzzy.vg's /api/status-check returns so the
// /status page can render announcement + per-group cards + per-model rows.
// statusBucketDTO is one time bucket of a model's observed traffic, used to
// draw the per-model history strip. Kept compact because the page renders one
// series per model.
type statusBucketDTO struct {
	Ts        int64 `json:"ts"`
	Requests  int64 `json:"requests"`
	OkPct     int   `json:"ok_pct"`
	LatencyMs int64 `json:"latency_ms"`
}

type modelHealthDTO struct {
	ModelName       string            `json:"model"`
	Group           string            `json:"group"`
	Availability    float64           `json:"availability"`
	LatencyMs       int64             `json:"latency_ms"`
	Level           string            `json:"level"`
	ChannelId       int               `json:"channel_id,omitempty"`
	Ok              bool              `json:"ok"`
	ErrorCode       int               `json:"error_code,omitempty"`
	ErrorMsg        string            `json:"error_msg,omitempty"`
	HasActive       bool              `json:"has_active"`
	PassiveRequests int64             `json:"passive_requests,omitempty"`
	PassiveSuccess  int64             `json:"passive_success,omitempty"`
	Series          []statusBucketDTO `json:"series,omitempty"`
	// HasData is false for a model the group serves but that neither a probe nor
	// any traffic has reached inside the window. The card shows it as "no data"
	// rather than as a healthy model, and it contributes to no availability
	// average — an unobserved model is not a working one.
	HasData bool `json:"has_data"`
	// Source names the mirror a row was filled from ("mirror:furry" and so on).
	// Empty means the row is ours: a probe result or our own traffic.
	Source string `json:"source,omitempty"`
}

type groupHealthDTO struct {
	Group        string           `json:"group"`
	Availability float64          `json:"availability"`
	AvgLatencyMs int64            `json:"avg_latency_ms"`
	TotalProbes  int              `json:"total_probes"`
	OkProbes     int              `json:"ok_probes"`
	Level        string           `json:"level"`
	Models       []modelHealthDTO `json:"models"`
	// Observed is false when the group has neither probe results nor traffic in
	// the window. Its Availability is still filled in — a group with nothing to
	// report reads as 100% — but the card renders that figure as unknown rather
	// than as a green 100%, which is what made "all green" and "all red" on the
	// same model possible.
	Observed bool `json:"observed"`
}

type statusCheckResponse struct {
	Announcement string           `json:"announcement"`
	Groups       []groupHealthDTO `json:"groups"`
	// WindowStart is the hour the per-model `series` buckets are anchored to.
	// The model square draws each series as a 24-slot strip, and a strip needs
	// to know which hour slot 0 is; the perf summary's window is computed from a
	// different setting, so borrowing that one placed bars on the wrong hours.
	WindowStart int64 `json:"window_start"`
}

// GetStatusCheck /api/status-check — merge active probe results (group_health /
// model_health tables) with passive perf_metrics rollup into one payload.
func GetStatusCheck(c *gin.Context) {
	cfg := status_check_setting.GetSetting()
	// The page promises to mirror the model square, so the group set must come
	// from what actually serves traffic, not from a hand-kept list. The
	// configured list still decides the order and can hide a group, but a group
	// missing from it (every group added since the list was written) now shows
	// up, and a group whose channels were all disabled drops out.
	servedGroups, err := model.GetGroupsWithEnabledChannel()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// Same gate the model square uses: a group is shown when it is listed in
	// UserUsableGroups and still has an enabled channel. Anything else is either
	// a group with no traffic capability or an internal one the square hides.
	squareGroups := map[string]struct{}{}
	for g := range setting.GetUserUsableGroupsCopy() {
		if _, ok := servedGroups[g]; ok {
			squareGroups[g] = struct{}{}
		}
	}
	// The configured list still decides order and can narrow the set; groups
	// missing from it (anything added since it was written) are appended, and
	// names whose channels are gone are dropped by GetGroupsExcluding.
	groups := status_check_setting.GetGroupsExcluding(squareGroups)
	listed := map[string]struct{}{}
	for _, g := range groups {
		listed[g] = struct{}{}
	}
	for g := range squareGroups {
		if _, ok := listed[g]; !ok {
			groups = append(groups, g)
		}
	}
	sort.Strings(groups)

	// A probe cycle writes its rows when it *finishes*, and a full cycle across
	// every grouped channel takes far longer than two configured intervals (the
	// interval spaces the starts, not the completions). Sizing this window off
	// the interval therefore dropped every active probe the moment a slow cycle
	// overran it, and the page fell back to passive traffic only — "no probe data
	// at all". The latest-per-key queries already ignore superseded cycles, so a
	// window wide enough to hold one completed cycle is all this needs; use the
	// passive window so a reached model keeps its probe reading for as long as the
	// traffic it is merged with.
	passiveWindowHours := status_check_setting.GetPassiveWindowHours()
	since := time.Now().Unix() - int64(passiveWindowHours)*3600

	activeGroups, err := model.GetLatestGroupHealth(groups, since)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	activeByGroup := map[string]model.GroupHealth{}
	for _, g := range activeGroups {
		activeByGroup[g.Group] = g
	}

	// passive perf_metrics rollup across the configured window
	endTs := time.Now().Unix()
	startTs := endTs - int64(passiveWindowHours)*3600
	passiveSummary, err := model.GetPerfMetricsSummaryByGroup(startTs, endTs, groups)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// keyed by group|model
	passive := map[string]model.PerfMetricSummaryByGroup{}
	for _, s := range passiveSummary {
		passive[s.Group+"|"+s.ModelName] = s
	}

	// Per-model history for the availability strip. One query covers every
	// model in every group, so the strip costs no extra round-trips per row.
	buckets, err := model.GetPerfMetricBucketsByGroup(startTs, endTs, groups)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	seriesByModel := map[string][]statusBucketDTO{}
	// The strip's slot 0 is the earliest bucket actually returned, not the
	// query cutoff: the cutoff is wall-clock and unaligned, so anchoring on it
	// left the first hour empty and shifted every bar by one.
	var earliestBucket int64
	for _, b := range buckets {
		key := b.Group + "|" + b.ModelName
		if earliestBucket == 0 || b.BucketTs < earliestBucket {
			earliestBucket = b.BucketTs
		}
		okPct := 0
		if b.RequestCount > 0 {
			okPct = int(float64(b.SuccessCount) / float64(b.RequestCount) * 100)
		}
		latency := int64(0)
		if b.RequestCount > 0 {
			latency = b.TotalLatencyMs / b.RequestCount
		}
		seriesByModel[key] = append(seriesByModel[key], statusBucketDTO{
			Ts:        b.BucketTs,
			Requests:  b.RequestCount,
			OkPct:     okPct,
			LatencyMs: latency,
		})
	}

	excluded := status_check_setting.GetExcludedModelsMap()

	// WindowStart is the first hour bucket the query can return. startTs is the
	// wall-clock cutoff and need not sit on an hour boundary, while bucket_ts
	// always does, so anchoring a strip on startTs would put every bar one slot
	// off whenever the request landed mid-hour.
	// WindowStart is the hour the per-model `series` buckets are anchored to.
	// It is the earliest bucket the query actually returned; falling back to the
	// aligned cutoff only when nothing came back, so a strip with no data still
	// has a defined origin.
	if earliestBucket == 0 {
		earliestBucket = startTs - startTs%3600
	}
	out := statusCheckResponse{
		Announcement: cfg.Announcement,
		Groups:       []groupHealthDTO{},
		WindowStart:  earliestBucket,
	}
	for _, group := range groups {
		dto := groupHealthDTO{Group: group, Models: []modelHealthDTO{}}

		// The plaza lists a model only while a channel in the group still serves
		// it. Probe rows and passive metrics outlive that, so without this filter
		// a retired model keeps a card here long after the model square dropped
		// it.
		served := servedModelsForGroup(group)

		// The upstream that serves this group, when one of the mirrored
		// deployments does. Models the group serves that neither a probe nor our
		// own traffic reached are filled from that mirror below, so a group is
		// not left blank just because we only probe one of its models.
		mirrorSource := perfmetrics.UpstreamSourceForGroup(group)
		mirrorAvailSum, mirrorAvailCount := 0.0, 0

		// models in this group come from active probe rows; if no active row yet,
		// fall back to passive perf_metrics models for this group.
		modelRows, err := model.GetLatestModelHealth(group, since)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		// A row's numbers come from the upstream that serves the group when the
		// mirror carries them. That is the per-group reading the upstream plaza
		// shows, and it covers every model the group offers instead of the one
		// model our probe happened to pick — which is why two groups whose
		// channels point at different upstream groups used to show one identical
		// figure. Our probe and our own traffic remain the fallback for groups no
		// mirror backs (国模破甲, free1, self-hosted relays).
		mirrorRow := func(name string) (modelHealthDTO, bool) {
			row, ok := perfmetrics.UpstreamMirrorRow(mirrorSource, group, name, 24)
			if !ok {
				return modelHealthDTO{}, false
			}
			mirrorAvailSum += row.SuccessRate
			mirrorAvailCount++
			return mirrorHealthRow(row), true
		}

		seen := map[string]bool{}
		for _, m := range modelRows {
			if !modelIsAdvertised(served, m.ModelName, excluded) {
				continue
			}
			seen[m.ModelName] = true
			if row, ok := mirrorRow(m.ModelName); ok {
				dto.Models = append(dto.Models, row)
				continue
			}
			dto.Models = append(dto.Models, mergeHealthRow(m, passive, group))
		}
		// passive-only models (no active probe yet) — cold start fallback
		for _, s := range passiveSummary {
			if s.Group != group {
				continue
			}
			if seen[s.ModelName] {
				continue
			}
			if !modelIsAdvertised(served, s.ModelName, excluded) {
				continue
			}
			seen[s.ModelName] = true
			if row, ok := mirrorRow(s.ModelName); ok {
				dto.Models = append(dto.Models, row)
				continue
			}
			dto.Models = append(dto.Models, passiveOnlyRow(s, group))
		}

		// Attach our own observed history strip to rows that do not already
		// carry one. A mirrored row brings the upstream's strip; overwriting it
		// with an empty local series erased exactly the data this card exists to
		// show.
		for i := range dto.Models {
			if len(dto.Models[i].Series) > 0 {
				continue
			}
			dto.Models[i].Series = seriesByModel[group+"|"+dto.Models[i].ModelName]
		}

		// A model the group serves but that neither a probe nor traffic reached
		// in the window still belongs on the card: it is what the group offers,
		// and hiding it until it happens to be exercised makes a freshly
		// configured model invisible. It is listed without data and counts
		// towards no average.
		if served != nil {
			for _, name := range sortedServedModelNames(served) {
				if seen[name] || modelIsExcluded(name, excluded) {
					continue
				}
				if _, hasPassive := passive[group+"|"+name]; hasPassive {
					continue
				}
				seen[name] = true
				if row, ok := perfmetrics.UpstreamMirrorRow(mirrorSource, group, name, 24); ok {
					dto.Models = append(dto.Models, mirrorHealthRow(row))
					mirrorAvailSum += row.SuccessRate
					mirrorAvailCount++
					continue
				}
				dto.Models = append(dto.Models, modelHealthDTO{
					ModelName: name,
					Group:     group,
					Level:     "unknown",
					HasData:   false,
				})
			}
		}

		// group-level roll-up
		active, hasActive := activeByGroup[group]
		if hasActive && active.TotalProbes > 0 {
			dto.TotalProbes = active.TotalProbes
			dto.OkProbes = active.OkProbes
			dto.AvgLatencyMs = active.AvgLatencyMs
		}
		passiveAvail := passiveGroupAvailability(passive, group)
		switch {
		case mirrorAvailCount > 0:
			// The rows on this card are mirrored numbers, so the headline has to
			// be built from the same evidence. Averaging probe results over
			// models the mirror disagrees about is what put a 43% group total
			// above a card whose rows were all red.
			dto.Availability = mirrorAvailSum / float64(mirrorAvailCount)
			dto.Observed = true
		case hasActive && active.TotalProbes > 0:
			dto.Availability = mergeAvailability(active.Availability, passiveAvail, passiveGroupRequests(passive, group))
			dto.Observed = true
		case passiveAvail >= 0:
			dto.Availability = passiveAvail
			dto.Observed = true
		default:
			dto.Availability = 100 // unknown → optimistic
			dto.Observed = false
		}
		dto.Level = GradeAvailability(dto.Availability)

		out.Groups = append(out.Groups, dto)
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}

// mergeHealthRow builds a per-model DTO combining active probe + passive rollup.
// servedModelsForGroup returns the models the plaza still lists for a group,
// read from the same ability table the plaza reads. It returns nil when the
// table reports none, which leaves the page's previous behaviour in place
// rather than blanking a group whose abilities are mid-rebuild.
func servedModelsForGroup(group string) map[string]bool {
	names := model.GetGroupEnabledModels(group)
	if len(names) == 0 {
		return nil
	}
	served := make(map[string]bool, len(names))
	for _, name := range names {
		served[name] = true
	}
	return served
}

// modelIsAdvertised reports whether a model may still appear on the status page:
// the group must serve it, and the admin's exclusion list must not name it.
func modelIsAdvertised(served map[string]bool, modelName string, excluded map[string]struct{}) bool {
	if served != nil && !served[modelName] {
		return false
	}
	if modelIsExcluded(modelName, excluded) {
		return false
	}
	return true
}

func modelIsExcluded(modelName string, excluded map[string]struct{}) bool {
	if excluded == nil {
		return false
	}
	_, skip := excluded[modelName]
	return skip
}

// sortedServedModelNames gives the no-data rows a stable order, so the card
// does not reshuffle between refreshes.
func sortedServedModelNames(served map[string]bool) []string {
	names := make([]string, 0, len(served))
	for name := range served {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func mergeHealthRow(m model.ModelHealth, passive map[string]model.PerfMetricSummaryByGroup, group string) modelHealthDTO {
	dto := modelHealthDTO{
		ModelName: m.ModelName,
		Group:     group,
		LatencyMs: m.LatencyMs,
		ChannelId: m.ChannelId,
		Ok:        m.Ok,
		ErrorCode: m.ErrorCode,
		ErrorMsg:  m.ErrorMsg,
		HasActive: true,
		HasData:   true,
	}
	activeAvail := 0.0
	if m.Ok {
		activeAvail = 100
	}
	var passiveAvail float64 = -1
	var passiveRequests int64
	var passiveLatencyMs int64
	if p, ok := passive[group+"|"+m.ModelName]; ok {
		dto.PassiveRequests = p.RequestCount
		dto.PassiveSuccess = p.SuccessCount
		passiveRequests = p.RequestCount
		if p.RequestCount > 0 {
			passiveAvail = float64(p.SuccessCount) / float64(p.RequestCount) * 100
			passiveLatencyMs = p.TotalLatencyMs / p.RequestCount
		}
	}
	// The row's latency should describe the model as this group serves it, so
	// the group's own traffic wins over the probe once there is any. A probe
	// duration is the whole request, and a failed one measures how fast the
	// failure came back — 45ms is a round trip to nowhere, not a response time.
	if passiveRequests > 0 {
		dto.LatencyMs = passiveLatencyMs
	} else if !m.Ok {
		dto.LatencyMs = 0
	}
	dto.Availability = mergeAvailability(activeAvail, passiveAvail, float64(passiveRequests))
	dto.Level = GradeAvailability(dto.Availability)
	return dto
}

// mirrorHealthRow turns a mirrored upstream card into the row the status page
// renders. The numbers belong to the upstream that serves this group, not to our
// own traffic, so the row carries no channel and no probe flag — but it carries
// the upstream's own 24h strip, which is what makes a card look alive on a group
// whose other models we never probe.
func mirrorHealthRow(row perfmetrics.GroupModelSummary) modelHealthDTO {
	series := make([]statusBucketDTO, 0, len(row.RecentSuccessSeries))
	latencyByTs := make(map[int64]int64, len(row.RecentLatencySeries))
	for _, p := range row.RecentLatencySeries {
		latencyByTs[p.Ts] = p.LatencyMs
	}
	for _, p := range row.RecentSuccessSeries {
		series = append(series, statusBucketDTO{
			Ts:        p.Ts,
			OkPct:     int(p.SuccessRate),
			LatencyMs: latencyByTs[p.Ts],
		})
	}
	return modelHealthDTO{
		ModelName:    row.ModelName,
		Group:        row.Group,
		Availability: row.SuccessRate,
		Level:        GradeAvailability(row.SuccessRate),
		LatencyMs:    row.AvgLatencyMs,
		Ok:           row.SuccessRate >= 70,
		Series:       series,
		HasData:      len(series) > 0 || row.SuccessRate > 0,
		Source:       row.Source,
	}
}

func passiveOnlyRow(s model.PerfMetricSummaryByGroup, group string) modelHealthDTO {
	avail := 100.0
	if s.RequestCount > 0 {
		avail = float64(s.SuccessCount) / float64(s.RequestCount) * 100
	}
	return modelHealthDTO{
		ModelName:       s.ModelName,
		Group:           group,
		Availability:    avail,
		Level:           GradeAvailability(avail),
		Ok:              avail >= 70,
		PassiveRequests: s.RequestCount,
		PassiveSuccess:  s.SuccessCount,
		HasData:         true,
	}
}

// mergeAvailability combines the probe and the passive rollup.
//
// The two sides are weighted by how much evidence each carries: the probe is a
// single request, so it counts as one sample against the group's real traffic.
// Averaging them 50/50 instead let one failed probe halve a group's measured
// uptime — a model with 17 good requests out of 17 read 50%, and the status
// strip next to it, which is built from the requests alone, was all green. With
// no passive traffic the probe is all there is, so it stands alone.
// passiveAvail is -1 when absent.
func mergeAvailability(activeAvail, passiveAvail, passiveRequests float64) float64 {
	if passiveAvail < 0 || passiveRequests <= 0 {
		return activeAvail
	}
	// One probe, passiveRequests observed requests.
	return (activeAvail + passiveAvail*passiveRequests) / (1 + passiveRequests)
}

func passiveGroupAvailability(passive map[string]model.PerfMetricSummaryByGroup, group string) float64 {
	totalReq, totalSucc := passiveGroupCounts(passive, group)
	if totalReq == 0 {
		return -1
	}
	return float64(totalSucc) / float64(totalReq) * 100
}

// passiveGroupRequests is the sample size behind passiveGroupAvailability, used
// to weight that figure against a single probe.
func passiveGroupRequests(passive map[string]model.PerfMetricSummaryByGroup, group string) float64 {
	totalReq, _ := passiveGroupCounts(passive, group)
	return float64(totalReq)
}

func passiveGroupCounts(passive map[string]model.PerfMetricSummaryByGroup, group string) (int64, int64) {
	totalReq, totalSucc := int64(0), int64(0)
	for _, p := range passive {
		if !strings.EqualFold(p.Group, group) {
			continue
		}
		totalReq += p.RequestCount
		totalSucc += p.SuccessCount
	}
	return totalReq, totalSucc
}

// GradeAvailability thresholds mirrored from web/src/features/performance-metrics/lib/format.ts
func GradeAvailability(pct float64) string {
	switch {
	case pct >= 100:
		return "excellent"
	case pct >= 90:
		return "good"
	case pct >= 70:
		return "warning"
	default:
		return "critical"
	}
}

// probeGroupsFor narrows a channel's groups to the ones worth probing. When
// only-free probing is on, a group whose ratio is non-zero is dropped: the
// probe would bill a real request, and the mirrored upstream already reports
// that group's health.
func probeGroupsFor(groups []string, modelName string) []string {
	if !status_check_setting.ShouldProbeOnlyFreeModels() {
		return groups
	}
	return lo.Filter(groups, func(group string, _ int) bool {
		return ratio_setting.IsModelFreeInGroup(modelName, group)
	})
}

// ------------------- scheduled probe -------------------

type groupHealthProbeHandler struct{}

func (groupHealthProbeHandler) Type() string    { return model.SystemTaskTypeGroupHealthProbe }
func (groupHealthProbeHandler) Enabled() bool   { return status_check_setting.GetSetting().Enabled }
func (groupHealthProbeHandler) NewPayload() any { return nil }

func (groupHealthProbeHandler) Interval() time.Duration {
	minutes := status_check_setting.GetIntervalMinutes()
	return time.Duration(minutes) * time.Minute
}

// channelTestUnsupportedTypes mirrors controller/channel-test.go unsupportedTestChannelTypes
// shared with the channel_test flow; keep in sync.
var channelTestUnsupportedTypes = []int{
	constant.ChannelTypeMidjourney,
	constant.ChannelTypeMidjourneyPlus,
	constant.ChannelTypeSunoAPI,
	constant.ChannelTypeKling,
	constant.ChannelTypeJimeng,
	constant.ChannelTypeDoubaoVideo,
	constant.ChannelTypeVidu,
}

// Run executes one probe cycle. For every configured group, iterate channels
// that carry that group and call testChannel directly (NOT performChannelTests),
// so the auto-disable path is bypassed. Write results into model_health and roll
// the group aggregate up into group_health.
func (groupHealthProbeHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary, err := runGroupHealthProbe(ctx, service.NewSystemTaskProgressReporter(task, runnerID))
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

type groupHealthProbeSummary struct {
	Groups   int `json:"groups"`
	Channels int `json:"channels"`
	Tested   int `json:"tested"`
	Ok       int `json:"ok"`
	Failed   int `json:"failed"`
}

// selectFlexibleProbeGroups applies the flexible pacing rules and returns the
// groups this cycle may probe.
//
// A group that has been serving real requests does not need an automatic probe
// to tell it is up, so each group waits out its own quiet period; and once a
// group has been probed maxConsecutiveProbes times without any traffic needing
// it, probing stops until traffic returns. Both limits are per group, so a busy
// group is skipped while a quiet one keeps being checked.
//
// With flexible probing off every group is returned untouched and nothing is
// read or written, so the existing cadence is preserved exactly.
func selectFlexibleProbeGroups(groups []string, now int64) ([]string, error) {
	flexibleConfig := perfmetrics.GetStatusCheckFlexibleConfig()

	// Groups the console has not enabled are returned untouched: the feature is
	// opt-in per group, and a group absent from the document keeps the ordinary
	// cadence with nothing read or written.
	enabled := make([]string, 0, len(groups))
	for _, group := range groups {
		if _, ok := flexibleConfig.EnabledGroup(group); ok {
			enabled = append(enabled, group)
		}
	}
	if len(enabled) == 0 {
		return groups, nil
	}

	states, err := model.GetStatusCheckProbeStates(groups)
	if err != nil {
		return nil, err
	}

	eligible := make([]string, 0, len(groups))
	for _, group := range groups {
		groupConfig, ok := flexibleConfig.EnabledGroup(group)
		if !ok {
			eligible = append(eligible, group)
			continue
		}
		idleSeconds := int64(groupConfig.IdleMinutes) * 60
		maxConsecutive := groupConfig.MaxConsecutiveProbes

		state, exists := states[group]
		if !exists {
			// Open the idle window instead of spending a probe the moment
			// flexible probing is switched on for this group.
			if err := model.InitializeStatusCheckProbeState(group, now); err != nil {
				return nil, err
			}
			continue
		}

		lastPassiveAt := perfmetrics.LatestStatusCheckPassiveActivity(group, state.LastPassiveAt)
		if lastPassiveAt > state.LastPassiveAt {
			if err := model.TouchStatusCheckProbePassiveActivity(group, lastPassiveAt); err != nil {
				return nil, err
			}
		}

		lastActivityAt := state.LastProbeAt
		if lastPassiveAt > lastActivityAt {
			lastActivityAt = lastPassiveAt
		}
		if now-lastActivityAt < idleSeconds {
			continue // recently serving; it has already proven itself
		}

		consecutive := state.ConsecutiveProbeCount
		if lastPassiveAt > state.LastProbeAt {
			consecutive = 0 // traffic arrived after the last probe
		}
		if consecutive >= maxConsecutive {
			continue // nothing has needed this group; stop paying to check it
		}

		eligible = append(eligible, group)
	}
	return eligible, nil
}

// probeUnserved reports whether a probe failure means "this model is not routed
// in this group" rather than "the model is unhealthy". Both cases are decided by
// our own router before the request reaches an upstream: a group that does not
// carry the model answers "no available channel", and a model with no price
// answers model_price_error. Recording them as failures made an idle group look
// like an outage — 1111 such rows in one day dragged default to 5% uptime.
func probeUnserved(result testResult) bool {
	message := ""
	if result.localErr != nil {
		message = result.localErr.Error()
	}
	if result.newAPIError != nil && result.newAPIError.Err != nil {
		message += " " + result.newAPIError.Err.Error()
	}
	for _, marker := range []string{
		"无可用渠道",
		"no available channel",
		"model_price_error",
		"model price not configured",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func runGroupHealthProbe(ctx context.Context, report func(processed, total int)) (*groupHealthProbeSummary, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	groups := status_check_setting.GetGroups()
	excluded := status_check_setting.GetExcludedModelsMap()
	excludedGroups := status_check_setting.GetExcludedGroupsMap()

	// Flexible pacing narrows the cycle to the groups that actually need a
	// probe. Off by default, in which case this returns the configured groups
	// unchanged and reads and writes no state.
	groups, err := selectFlexibleProbeGroups(groups, time.Now().Unix())
	if err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		// Every configured group is serving traffic or has hit its probe cap;
		// there is nothing to spend this cycle.
		return &groupHealthProbeSummary{}, nil
	}

	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		return nil, err
	}

	testUserID, err := resolveChannelTestUserID(nil)
	if err != nil {
		return nil, err
	}

	probeTs := time.Now().Unix()

	// Models the operator disabled in the catalog must not be probed: they are
	// not on the square, so a probe spends a real request to learn about a model
	// nobody can call, and its failure counts against the group's availability.
	disabledModel, err := model.GetDisabledModelMatcher()
	if err != nil {
		common.SysLog("group_health_probe: disabled-model lookup failed: " + err.Error())
	}

	summary := &groupHealthProbeSummary{Groups: len(groups)}
	totalUnits := 0
	channelGroups := make(map[int][]string, len(channels))
	for _, ch := range channels {
		if ch.Status == common.ChannelStatusManuallyDisabled {
			continue
		}
		if lo.Contains(channelTestUnsupportedTypes, ch.Type) {
			continue
		}
		chGroups := ch.GetGroups()
		relevant := lo.Filter(chGroups, func(g string, _ int) bool {
			return lo.Contains(groups, g)
		})
		if len(relevant) == 0 {
			continue
		}
		channelGroups[ch.Id] = relevant
		totalUnits++
	}
	summary.Channels = len(channelGroups)

	processed := 0
	type groupAcc struct {
		total  int
		ok     int
		latSum int64
	}
	acc := make(map[string]*groupAcc, len(groups))
	for _, g := range groups {
		acc[g] = &groupAcc{}
	}
	// Groups this cycle actually probed, recorded once each afterwards so an
	// idle group's streak counts cycles rather than channels.
	probedGroups := make(map[string]struct{}, len(groups))

	for _, ch := range channels {
		if ctx.Err() != nil {
			break
		}
		relevant, ok := channelGroups[ch.Id]
		if !ok {
			continue
		}
		// One live request per channel tells whether its group is healthy.
		// Prefer the channel's configured test model, matching the scheduled
		// channel test, and fall back to the first model the channel serves.
		// Models the upstream mirror does not report are probed as well: that
		// is the only health data those models will ever have.
		// A group listed in ExcludedGroups is never probed: a per-request image
		// group costs real money on every call and the mirror cannot substitute
		// for that, so the operator opts the whole group out rather than us
		// guessing from its model prices.
		if excludedGroups != nil {
			relevant = lo.Filter(relevant, func(g string, _ int) bool {
				_, skip := excludedGroups[g]
				return !skip
			})
			if len(relevant) == 0 {
				continue
			}
		}
		probeModel := pickProbeModel(ch, relevant)
		if probeModel == "" {
			continue
		}
		// A model the upstream that serves this group does not report has no
		// health data unless we ask it ourselves, so every such model is probed
		// even when it is paid. Coverage is decided per group: asking the furry
		// mirror about a SheAPI/McApple group marked its models covered and left
		// their cards blank forever.
		uncovered := uncoveredProbeModels(ch, func(modelName string) bool {
			return perfmetrics.UpstreamCoversModelInGroup(modelName, relevant)
		})
		// A model the catalog disabled is not served, so it is neither the probe
		// model nor an uncovered model worth asking about.
		if disabledModel != nil {
			if disabledModel(probeModel) {
				probeModel = ""
			}
			uncovered = lo.Filter(uncovered, func(name string, _ int) bool {
				return !disabledModel(name)
			})
		}
		if probeModel == "" && len(uncovered) == 0 {
			continue
		}
		uncoveredSet := make(map[string]struct{}, len(uncovered))
		models := make([]string, 0, len(uncovered)+1)
		if probeModel != "" {
			models = append(models, probeModel)
		}
		for _, name := range uncovered {
			uncoveredSet[name] = struct{}{}
			if name != probeModel {
				models = append(models, name)
			}
		}
		for _, modelName := range models {
			groupsForModel := relevant
			if _, isUncovered := uncoveredSet[modelName]; !isUncovered {
				groupsForModel = probeGroupsFor(relevant, modelName)
			}
			if len(groupsForModel) == 0 {
				continue
			}
			if excluded != nil {
				if _, skip := excluded[modelName]; skip {
					continue
				}
			}
			processed++
			if report != nil {
				report(processed, totalUnits)
			}
			tik := time.Now()
			// Walk the endpoints the channel declares for this model. A model may
			// be served on only one of them — a System One decision model on an
			// OpenCode pool answers only /v1/systemone — so probing the first
			// endpoint alone would report it unhealthy forever.
			result := testChannelAcrossEndpoints(ctx, ch, testUserID, modelName, "", false)
			latMs := time.Since(tik).Milliseconds()

			ok := result.localErr == nil && result.newAPIError == nil
			// A pair this deployment does not route is not a health signal: the
			// request never left the router. Writing it would count "we do not
			// serve this here" as "the model is down" and pull the group's
			// availability down for every model it does serve.
			if !ok && probeUnserved(result) {
				continue
			}
			errCode := 0
			errMsg := ""
			if result.newAPIError != nil {
				errCode = result.newAPIError.StatusCode
			}
			if result.localErr != nil {
				errMsg = result.localErr.Error()
			}
			if result.newAPIError != nil && result.newAPIError.Err != nil && errMsg == "" {
				errMsg = result.newAPIError.Err.Error()
			}

			// One live probe per (channel, model); fan the same outcome out
			// to every configured group that owns this channel.
			//
			// A failed probe says nothing about latency: the request may have
			// been rejected here before it ever reached the upstream (an
			// unconfigured price answers in 1ms), or failed at the first hop of
			// a long route. Reporting that duration as the model's latency put
			// "1ms" next to a model that never answered.
			reportedLatency := latMs
			if !ok {
				reportedLatency = 0
			}
			for _, g := range groupsForModel {
				probedGroups[g] = struct{}{}
				mh := model.ModelHealth{
					ModelName: modelName,
					Group:     g,
					ProbeTs:   probeTs,
					LatencyMs: reportedLatency,
					Ok:        ok,
					ErrorCode: errCode,
					ErrorMsg:  errMsg,
					ChannelId: ch.Id,
				}
				if err := model.UpsertModelHealth(&mh); err != nil {
					common.SysLog(fmt.Sprintf("group_health_probe: upsert model_health failed: group=%s model=%s err=%v", g, modelName, err))
				}
				// The model page reads perf samples, not the health table. Only
				// uncovered models are recorded: a covered model's one probe must
				// not replace the mirror's fuller series.
				if _, isUncovered := uncoveredSet[modelName]; isUncovered {
					perfmetrics.Record(perfmetrics.Sample{
						Model:     modelName,
						Group:     g,
						LatencyMs: reportedLatency,
						Success:   ok,
					})
				}
				accG := acc[g]
				accG.total++
				accG.latSum += reportedLatency
				if ok {
					accG.ok++
				}
			}
			summary.Tested++
			if ok {
				summary.Ok++
			} else {
				summary.Failed++
			}
			if common.RequestInterval > 0 {
				select {
				case <-ctx.Done():
					return summary, nil
				case <-time.After(common.RequestInterval):
				}
			}
		}
	}

	// One probe recorded per group for this cycle. Only when flexible probing is
	// on, so an installation that does not use it keeps no pacing state at all.
	if status_check_setting.FlexibleProbingEnabled() {
		for group := range probedGroups {
			if err := model.RecordStatusCheckProbeResult(group, probeTs); err != nil {
				common.SysError(fmt.Sprintf("group_health_probe: record probe result failed: group=%s err=%v", group, err))
			}
		}
	}

	// roll up group_health
	for _, g := range groups {
		a := acc[g]
		if a.total == 0 {
			continue // skip group with no probes
		}
		avail := float64(a.ok) / float64(a.total) * 100
		gh := model.GroupHealth{
			Group:        g,
			ProbeTs:      probeTs,
			Availability: avail,
			AvgLatencyMs: a.latSum / int64(a.total),
			TotalProbes:  a.total,
			OkProbes:     a.ok,
			FailedProbes: a.total - a.ok,
		}
		if err := model.UpsertGroupHealth(&gh); err != nil {
			common.SysError(fmt.Sprintf("group_health_probe: upsert group_health failed: group=%s err=%v", g, err))
		}
	}

	// Keep a month of probe history. The page draws a 24h strip and the group
	// cards answer "how has this been lately", so pruning to ten cycles (under
	// an hour at the default cadence) threw away the very history the page
	// reads: every refresh showed one fresh sample and nothing behind it.
	// A month at a five-minute cadence is ~8.6k rows per group and ~35k per
	// model, which the table handles.
	const healthRetentionDays = 30
	retentionCutoff := probeTs - int64(healthRetentionDays)*24*3600
	if err := model.DeleteHealthBefore(retentionCutoff, retentionCutoff); err != nil {
		common.SysError(fmt.Sprintf("group_health_probe: prune failed: %v", err))
	}

	return summary, nil
}
