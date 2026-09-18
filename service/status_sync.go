package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/channel_status_sync_setting"
)

const (
	// maxStatusSyncResponseBytes bounds every body read from the remote status
	// page. The real payloads are a few hundred KB at most; the cap keeps a
	// hostile or broken endpoint from exhausting memory.
	maxStatusSyncResponseBytes = 2 << 20
	statusSyncRequestTimeout   = 15 * time.Second
	statusSyncUserAgent        = "new-api-channel-status-sync/1.0"
)

// RemoteStatusKind is the normalized monitor state. Uptime Kuma reports numeric
// heartbeat codes; only "down" and "up" are actionable here, because treating a
// pending/maintenance monitor as down would disable healthy channels.
type RemoteStatusKind string

const (
	RemoteStatusUp          RemoteStatusKind = "up"
	RemoteStatusDown        RemoteStatusKind = "down"
	RemoteStatusPending     RemoteStatusKind = "pending"
	RemoteStatusMaintenance RemoteStatusKind = "maintenance"
	RemoteStatusUnknown     RemoteStatusKind = "unknown"
)

// Uptime Kuma heartbeat codes.
const (
	kumaStatusDown        = 0
	kumaStatusUp          = 1
	kumaStatusPending     = 2
	kumaStatusMaintenance = 3
)

// RemoteMonitor is one monitor from the remote status page, already flattened
// out of its group and reduced to its latest heartbeat.
type RemoteMonitor struct {
	ID      int              `json:"id"`
	Name    string           `json:"name"`
	Group   string           `json:"group,omitempty"`
	Status  RemoteStatusKind `json:"status"`
	Time    string           `json:"time,omitempty"`
	AgeSec  int64            `json:"age_seconds"` // -1 when the heartbeat time could not be parsed
	Message string           `json:"message,omitempty"`
}

// SyncAction is what the plan proposes for one channel.
type SyncAction string

const (
	SyncActionDisable SyncAction = "disable"
	SyncActionEnable  SyncAction = "enable"
	SyncActionNoop    SyncAction = "noop"
	SyncActionSkip    SyncAction = "skip"
)

// StatusSyncPlanEntry is one proposed change (or one explicit refusal to change).
// Skipped entries are reported rather than dropped so an operator can see which
// channels the sync deliberately left alone, and why.
type StatusSyncPlanEntry struct {
	ChannelID     int              `json:"channel_id"`
	ChannelName   string           `json:"channel_name"`
	MonitorID     int              `json:"monitor_id,omitempty"`
	MonitorName   string           `json:"monitor_name,omitempty"`
	MonitorGroup  string           `json:"monitor_group,omitempty"`
	RemoteStatus  RemoteStatusKind `json:"remote_status"`
	CurrentStatus int              `json:"current_status"`
	TargetStatus  int              `json:"target_status,omitempty"`
	Action        SyncAction       `json:"action"`
	Reason        string           `json:"reason"`
}

// StatusSyncPlan is a full dry-run result: what the remote page said, how it
// mapped onto local channels, and whether the change set is allowed to run.
type StatusSyncPlan struct {
	SourceURL     string                `json:"source_url"`
	GeneratedAt   int64                 `json:"generated_at"`
	MonitorCount  int                   `json:"monitor_count"`
	ChannelCount  int                   `json:"channel_count"`
	DisableCount  int                   `json:"disable_count"`
	EnableCount   int                   `json:"enable_count"`
	Blocked       bool                  `json:"blocked"`
	BlockedReason string                `json:"blocked_reason,omitempty"`
	Warnings      []string              `json:"warnings,omitempty"`
	Entries       []StatusSyncPlanEntry `json:"entries"`
}

// ParseStatusPageSource turns a human status page URL into the JSON API base plus
// the page slug. Both the SPA URL (.../status/<slug>) and the API URL
// (.../api/status-page/<slug>) resolve to the same origin + last path segment,
// so only the slug and origin are taken from the input.
func ParseStatusPageSource(rawURL string) (apiBase string, slug string, err error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return "", "", errors.New("status sync: source url is empty")
	}
	parsed, parseErr := url.Parse(trimmed)
	if parseErr != nil {
		return "", "", fmt.Errorf("status sync: invalid source url %q: %w", trimmed, parseErr)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", "", fmt.Errorf("status sync: source url must be http(s), got %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return "", "", fmt.Errorf("status sync: source url %q has no host", trimmed)
	}
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	slug = ""
	for i := len(segments) - 1; i >= 0; i-- {
		if segments[i] != "" {
			slug = segments[i]
			break
		}
	}
	if slug == "" {
		return "", "", fmt.Errorf("status sync: source url %q has no status page slug", trimmed)
	}
	return fmt.Sprintf("%s://%s/api/status-page", parsed.Scheme, parsed.Host), slug, nil
}

// decodeBoundedBody reads at most maxStatusSyncResponseBytes and fails loudly
// rather than silently truncating into a half-parsed (and therefore wrong) plan.
func decodeBoundedBody(body io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxStatusSyncResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("status sync: reading response body: %w", err)
	}
	if len(data) > maxStatusSyncResponseBytes {
		return nil, fmt.Errorf("status sync: response exceeds %d bytes", maxStatusSyncResponseBytes)
	}
	return data, nil
}

type statusPageJSON struct {
	PublicGroupList []struct {
		Name        string `json:"name"`
		MonitorList []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"monitorList"`
	} `json:"publicGroupList"`
}

type heartbeatJSON struct {
	HeartbeatList map[string][]struct {
		Status int    `json:"status"`
		Time   string `json:"time"`
		Msg    string `json:"msg"`
	} `json:"heartbeatList"`
}

type remoteMonitorMeta struct {
	name  string
	group string
}

// parseStatusPageJSON extracts monitor metadata. Returning an error (rather than
// an empty map) is deliberate: an unparsable page must not look like "zero
// monitors", which would otherwise be a silent no-op.
func parseStatusPageJSON(data []byte) (map[int]remoteMonitorMeta, error) {
	var page statusPageJSON
	if err := common.Unmarshal(data, &page); err != nil {
		return nil, fmt.Errorf("status sync: malformed status page json: %w", err)
	}
	if page.PublicGroupList == nil {
		return nil, errors.New("status sync: status page json has no publicGroupList")
	}
	metas := map[int]remoteMonitorMeta{}
	for _, group := range page.PublicGroupList {
		for _, monitor := range group.MonitorList {
			if monitor.ID == 0 {
				continue
			}
			metas[monitor.ID] = remoteMonitorMeta{name: monitor.Name, group: group.Name}
		}
	}
	return metas, nil
}

func kumaStatusCodeToKind(code int) RemoteStatusKind {
	switch code {
	case kumaStatusUp:
		return RemoteStatusUp
	case kumaStatusDown:
		return RemoteStatusDown
	case kumaStatusPending:
		return RemoteStatusPending
	case kumaStatusMaintenance:
		return RemoteStatusMaintenance
	default:
		return RemoteStatusUnknown
	}
}

// parseHeartbeatTime reads the heartbeat timestamp. Uptime Kuma emits a naive
// local-time string; the deployment this integrates with renders it in UTC (its
// timestamps match the HTTP Date header within a second). An unparsable value
// yields -1, which callers treat as "freshness unknown" instead of "stale".
func parseHeartbeatTime(raw string, now time.Time) int64 {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return -1
	}
	for _, layout := range []string{"2006-01-02 15:04:05.000", "2006-01-02 15:04:05", time.RFC3339} {
		parsed, err := time.Parse(layout, trimmed)
		if err != nil {
			continue
		}
		return int64(now.Sub(parsed).Seconds())
	}
	return -1
}

// parseHeartbeatJSON merges heartbeat history into the monitor metadata, keeping
// only the newest sample per monitor.
func parseHeartbeatJSON(data []byte, metas map[int]remoteMonitorMeta, now time.Time) ([]RemoteMonitor, error) {
	var heartbeats heartbeatJSON
	if err := common.Unmarshal(data, &heartbeats); err != nil {
		return nil, fmt.Errorf("status sync: malformed heartbeat json: %w", err)
	}
	if heartbeats.HeartbeatList == nil {
		return nil, errors.New("status sync: heartbeat json has no heartbeatList")
	}
	monitors := make([]RemoteMonitor, 0, len(metas))
	for id, meta := range metas {
		samples := heartbeats.HeartbeatList[fmt.Sprintf("%d", id)]
		if len(samples) == 0 {
			monitors = append(monitors, RemoteMonitor{
				ID: id, Name: meta.name, Group: meta.group,
				Status: RemoteStatusUnknown, AgeSec: -1,
				Message: "no heartbeat reported for this monitor",
			})
			continue
		}
		last := samples[len(samples)-1]
		monitors = append(monitors, RemoteMonitor{
			ID: id, Name: meta.name, Group: meta.group,
			Status: kumaStatusCodeToKind(last.Status), Time: last.Time,
			AgeSec: parseHeartbeatTime(last.Time, now), Message: last.Msg,
		})
	}
	return monitors, nil
}

// FetchRemoteMonitors pulls the status page and its heartbeats and returns one
// flattened entry per monitor. Any non-200, oversized, or malformed response is
// an error so the caller can skip the cycle instead of acting on partial data.
func FetchRemoteMonitors(ctx context.Context, sourceURL string) ([]RemoteMonitor, error) {
	apiBase, slug, err := ParseStatusPageSource(sourceURL)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: statusSyncRequestTimeout}

	fetch := func(path string) ([]byte, error) {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+path, nil)
		if reqErr != nil {
			return nil, fmt.Errorf("status sync: building request: %w", reqErr)
		}
		req.Header.Set("User-Agent", statusSyncUserAgent)
		resp, doErr := client.Do(req)
		if doErr != nil {
			return nil, fmt.Errorf("status sync: requesting %s: %w", path, doErr)
		}
		defer func() {
			_ = resp.Body.Close()
		}()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("status sync: %s returned http %d", path, resp.StatusCode)
		}
		return decodeBoundedBody(resp.Body)
	}

	pageData, err := fetch("/" + url.PathEscape(slug))
	if err != nil {
		return nil, err
	}
	metas, err := parseStatusPageJSON(pageData)
	if err != nil {
		return nil, err
	}
	if len(metas) == 0 {
		return nil, errors.New("status sync: status page reports no monitors")
	}

	heartbeatData, err := fetch("/heartbeat/" + url.PathEscape(slug))
	if err != nil {
		return nil, err
	}
	return parseHeartbeatJSON(heartbeatData, metas, time.Now().UTC())
}

// StatusSyncOptions is the resolved policy for one sync run. It is passed in
// explicitly rather than read from the global settings inside the planner so the
// decision logic is deterministic and testable.
type StatusSyncOptions struct {
	SourceURL          string
	DisableOnDown      bool
	EnableOnUp         bool
	Aliases            map[string]string // lowercase remote monitor name -> local channel name
	StaleAfterMinutes  int               // 0 disables the freshness check
	MaxDisableFraction float64
}

// StatusSyncOptionsFromSetting resolves the current settings into a run policy.
func StatusSyncOptionsFromSetting() StatusSyncOptions {
	cfg := channel_status_sync_setting.GetSetting()
	return StatusSyncOptions{
		SourceURL:          cfg.SourceURL,
		DisableOnDown:      cfg.DisableOnDown,
		EnableOnUp:         cfg.EnableOnUp,
		Aliases:            channel_status_sync_setting.GetAliases(),
		StaleAfterMinutes:  channel_status_sync_setting.GetStaleAfterMinutes(),
		MaxDisableFraction: channel_status_sync_setting.GetMaxDisableFraction(),
	}
}

// BuildStatusSyncPlan matches remote monitors to local channels and proposes a
// change set. Matching is deliberately conservative:
//
//   - an explicit alias in the settings wins and is the only way to map a monitor
//     whose name differs from the channel name;
//   - otherwise the names must be equal ignoring case and surrounding space;
//   - anything ambiguous (a name shared by several channels, or one channel
//     claimed by several monitors) or unmatched is skipped, never acted on.
//
// Only two transitions are ever proposed: Enabled -> auto-disabled on a down
// monitor, and auto-disabled -> Enabled when the monitor recovers. Human state
// (a manually disabled channel) is never overwritten.
func BuildStatusSyncPlan(opts StatusSyncOptions, monitors []RemoteMonitor, channels []*model.Channel) StatusSyncPlan {
	plan := StatusSyncPlan{
		SourceURL:    opts.SourceURL,
		GeneratedAt:  common.GetTimestamp(),
		MonitorCount: len(monitors),
		ChannelCount: len(channels),
		Entries:      []StatusSyncPlanEntry{},
	}
	maxDisableFraction := opts.MaxDisableFraction
	if maxDisableFraction <= 0 || maxDisableFraction > 1 {
		maxDisableFraction = 0.5
	}

	// candidates[i] holds the channels a monitor could refer to, before ambiguity
	// resolution. channelHits counts how many monitors claim each channel.
	candidates := make([][]*model.Channel, len(monitors))
	channelHits := map[int]int{}
	for i := range monitors {
		localName := monitors[i].Name
		aliasApplied := false
		if alias, ok := opts.Aliases[strings.ToLower(strings.TrimSpace(monitors[i].Name))]; ok {
			localName = alias
			aliasApplied = true
		}
		for _, channel := range channels {
			if strings.EqualFold(strings.TrimSpace(localName), strings.TrimSpace(channel.Name)) {
				candidates[i] = append(candidates[i], channel)
				channelHits[channel.Id]++
			}
		}
		if aliasApplied && len(candidates[i]) == 0 {
			plan.Warnings = append(plan.Warnings,
				fmt.Sprintf("alias %q -> %q matches no channel; monitor %q ignored", monitors[i].Name, localName, monitors[i].Name))
		}
	}

	for i, monitor := range monitors {
		matched := candidates[i]
		if len(matched) == 0 {
			continue // unmatched: nothing to report per-channel
		}
		if len(matched) > 1 {
			for _, channel := range matched {
				plan.Entries = append(plan.Entries, skipEntry(channel, monitor,
					fmt.Sprintf("monitor name matches %d channels; refusing to guess", len(matched))))
			}
			continue
		}
		channel := matched[0]
		if channelHits[channel.Id] > 1 {
			plan.Entries = append(plan.Entries, skipEntry(channel, monitor,
				fmt.Sprintf("channel is claimed by %d monitors; refusing to guess", channelHits[channel.Id])))
			continue
		}
		plan.Entries = append(plan.Entries, decideEntry(opts, channel, monitor))
	}

	for _, entry := range plan.Entries {
		switch entry.Action {
		case SyncActionDisable:
			plan.DisableCount++
		case SyncActionEnable:
			plan.EnableCount++
		}
	}

	// A remote page is untrusted input: refuse a change set that would take out
	// more than the configured fraction of the fleet, and say so loudly.
	if len(monitors) == 0 {
		plan.Blocked = true
		plan.BlockedReason = "remote status page reported no monitors"
	} else if plan.DisableCount > 0 {
		allowed := max(1, int(math.Floor(float64(len(channels))*maxDisableFraction)))
		if plan.DisableCount > allowed {
			plan.Blocked = true
			plan.BlockedReason = fmt.Sprintf(
				"refused: would disable %d of %d channels (limit %d, max_disable_fraction=%.2f)",
				plan.DisableCount, len(channels), allowed, maxDisableFraction)
		}
	}

	if plan.Blocked {
		common.SysError(fmt.Sprintf("channel status sync blocked: %s (source=%s)", plan.BlockedReason, plan.SourceURL))
	}
	return plan
}

func skipEntry(channel *model.Channel, monitor RemoteMonitor, reason string) StatusSyncPlanEntry {
	return StatusSyncPlanEntry{
		ChannelID: channel.Id, ChannelName: channel.Name,
		MonitorID: monitor.ID, MonitorName: monitor.Name, MonitorGroup: monitor.Group,
		RemoteStatus: monitor.Status, CurrentStatus: channel.Status,
		Action: SyncActionSkip, Reason: reason,
	}
}

func decideEntry(opts StatusSyncOptions, channel *model.Channel, monitor RemoteMonitor) StatusSyncPlanEntry {
	entry := StatusSyncPlanEntry{
		ChannelID: channel.Id, ChannelName: channel.Name,
		MonitorID: monitor.ID, MonitorName: monitor.Name, MonitorGroup: monitor.Group,
		RemoteStatus: monitor.Status, CurrentStatus: channel.Status,
	}
	skip := func(reason string) StatusSyncPlanEntry {
		entry.Action = SyncActionSkip
		entry.Reason = reason
		return entry
	}

	if monitor.AgeSec < 0 {
		return skip("heartbeat timestamp unparsable; freshness unknown")
	}
	if opts.StaleAfterMinutes > 0 && monitor.AgeSec > int64(opts.StaleAfterMinutes)*60 {
		return skip(fmt.Sprintf("latest heartbeat is %ds old (>%dm); status page considered stale", monitor.AgeSec, opts.StaleAfterMinutes))
	}

	switch monitor.Status {
	case RemoteStatusDown:
		if !opts.DisableOnDown {
			return skip("disable_on_down is off")
		}
		switch channel.Status {
		case common.ChannelStatusEnabled:
			entry.Action = SyncActionDisable
			entry.TargetStatus = common.ChannelStatusAutoDisabled
			entry.Reason = fmt.Sprintf("monitor %q reports down", monitor.Name)
		case common.ChannelStatusAutoDisabled:
			entry.Action = SyncActionNoop
			entry.Reason = "already auto-disabled by status sync"
		default:
			entry.Action = SyncActionNoop
			entry.Reason = "channel is disabled for a reason outside status sync; leaving as is"
		}
	case RemoteStatusUp:
		if !opts.EnableOnUp {
			return skip("enable_on_up is off")
		}
		switch channel.Status {
		case common.ChannelStatusEnabled:
			entry.Action = SyncActionNoop
			entry.Reason = "already enabled"
		case common.ChannelStatusAutoDisabled:
			entry.Action = SyncActionEnable
			entry.TargetStatus = common.ChannelStatusEnabled
			entry.Reason = fmt.Sprintf("monitor %q recovered", monitor.Name)
		case common.ChannelStatusManuallyDisabled:
			return skip("channel was disabled by hand; refusing to override a manual decision")
		default:
			return skip(fmt.Sprintf("unexpected channel status %d", channel.Status))
		}
	default:
		return skip(fmt.Sprintf("remote status %q is not actionable", monitor.Status))
	}
	return entry
}

// ApplyStatusSyncPlan executes a plan. A blocked plan is never applied. This is
// the only place that writes channel status, and it goes through the shared
// model.UpdateChannelStatus path so abilities, cache and status_reason stay
// consistent with every other enable/disable flow.
func ApplyStatusSyncPlan(plan *StatusSyncPlan) (int, error) {
	if plan == nil {
		return 0, errors.New("status sync: nil plan")
	}
	if plan.Blocked {
		return 0, fmt.Errorf("status sync: refusing to apply a blocked plan: %s", plan.BlockedReason)
	}
	changed := 0
	for _, entry := range plan.Entries {
		if entry.Action != SyncActionDisable && entry.Action != SyncActionEnable {
			continue
		}
		if model.UpdateChannelStatus(entry.ChannelID, "", entry.TargetStatus, entry.Reason) {
			changed++
		}
	}
	if changed > 0 {
		model.InitChannelCache()
	}
	return changed, nil
}

// RunChannelStatusSync performs one full cycle: fetch, match, and (when apply is
// true) persist. The plan is returned either way so a dry run and an applied run
// are described by exactly the same structure.
func RunChannelStatusSync(ctx context.Context, apply bool) (*StatusSyncPlan, int, error) {
	opts := StatusSyncOptionsFromSetting()
	monitors, err := FetchRemoteMonitors(ctx, opts.SourceURL)
	if err != nil {
		return nil, 0, err
	}
	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		return nil, 0, err
	}
	plan := BuildStatusSyncPlan(opts, monitors, channels)
	if !apply {
		return &plan, 0, nil
	}
	changed, err := ApplyStatusSyncPlan(&plan)
	return &plan, changed, err
}
