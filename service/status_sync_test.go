package service

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Fixtures mirror the live shape of https://status.s3.fan/status/s3ai-api
// (an Uptime Kuma status page): monitor metadata lives under publicGroupList,
// heartbeats are keyed by monitor id as a JSON string, and status codes are
// 0=down 1=up 2=pending 3=maintenance.
const statusPageFixture = `{
  "config": {"slug": "s3ai-api", "title": "S3AI API", "autoRefreshInterval": 300},
  "incidents": [],
  "publicGroupList": [
    {"id": 13, "name": "S3AI API", "weight": 1, "monitorList": [
      {"id": 37, "name": "源站", "sendUrl": 0, "type": "ping"},
      {"id": 29, "name": "主站", "sendUrl": 0, "type": "json-query"}
    ]},
    {"id": 14, "name": "号池", "weight": 2, "monitorList": [
      {"id": 42, "name": "Antigravity 按次Flash", "sendUrl": 0, "type": "keyword"}
    ]}
  ],
  "maintenanceList": []
}`

func heartbeatFixture(entries map[int]struct {
	status int
	age    time.Duration
	msg    string
}) string {
	now := time.Now().UTC()
	parts := make([]string, 0, len(entries))
	for id, entry := range entries {
		parts = append(parts, fmt.Sprintf(
			`"%d": [{"status": 1, "time": %q, "msg": "", "ping": 120}, {"status": %d, "time": %q, "msg": %q, "ping": 80}]`,
			id,
			now.Add(-2*time.Hour).Format("2006-01-02 15:04:05.000"),
			entry.status,
			now.Add(-entry.age).Format("2006-01-02 15:04:05.000"),
			entry.msg,
		))
	}
	return fmt.Sprintf(`{"heartbeatList": {%s}, "uptimeList": {"37_24": 0.99}}`, strings.Join(parts, ","))
}

func TestParseStatusPageSource(t *testing.T) {
	cases := []struct {
		name        string
		raw         string
		wantAPIBase string
		wantSlug    string
		wantErr     bool
	}{
		{
			name:        "spa url as given by the product owner",
			raw:         "https://status.s3.fan/status/s3ai-api",
			wantAPIBase: "https://status.s3.fan/api/status-page",
			wantSlug:    "s3ai-api",
		},
		{
			name:        "json api url resolves identically",
			raw:         "https://status.s3.fan/api/status-page/s3ai-api",
			wantAPIBase: "https://status.s3.fan/api/status-page",
			wantSlug:    "s3ai-api",
		},
		{
			name:        "trailing slash and host port are preserved",
			raw:         "http://localhost:3001/status/foo/",
			wantAPIBase: "http://localhost:3001/api/status-page",
			wantSlug:    "foo",
		},
		{name: "no slug", raw: "https://status.s3.fan/", wantErr: true},
		{name: "non http scheme", raw: "ftp://status.s3.fan/status/x", wantErr: true},
		{name: "empty", raw: "   ", wantErr: true},
		{name: "unparsable", raw: "://not-a-url", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			apiBase, slug, err := ParseStatusPageSource(tc.raw)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantAPIBase, apiBase)
			assert.Equal(t, tc.wantSlug, slug)
		})
	}
}

func TestParseStatusPageJSON(t *testing.T) {
	metas, err := parseStatusPageJSON([]byte(statusPageFixture))
	require.NoError(t, err)
	require.Len(t, metas, 3)
	assert.Equal(t, remoteMonitorMeta{name: "源站", group: "S3AI API"}, metas[37])
	assert.Equal(t, remoteMonitorMeta{name: "Antigravity 按次Flash", group: "号池"}, metas[42])
}

func TestParseStatusPageJSONRejectsMalformedInput(t *testing.T) {
	// A truncated body and a 200-with-HTML error page must both be errors, not an
	// empty monitor set: an empty set would look like a clean no-op cycle.
	for name, body := range map[string]string{
		"truncated":       `{"publicGroupList": [{"name": "x"`,
		"html error page": `<html><body>502 Bad Gateway</body></html>`,
		"missing field":   `{"config": {"slug": "s3ai-api"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseStatusPageJSON([]byte(body))
			require.Error(t, err)
		})
	}
}

func TestParseHeartbeatJSONMapsKumaStatusCodes(t *testing.T) {
	metas := map[int]remoteMonitorMeta{
		37: {name: "源站", group: "S3AI API"},
		29: {name: "主站", group: "S3AI API"},
		42: {name: "Antigravity 按次Flash", group: "号池"},
		50: {name: "GPT Pro 20x/5x正价", group: "号池"},
	}
	body := heartbeatFixture(map[int]struct {
		status int
		age    time.Duration
		msg    string
	}{
		37: {status: 1, age: time.Minute},
		29: {status: 0, age: 2 * time.Minute},
		42: {status: 2, age: 3 * time.Minute, msg: "retrying"},
		50: {status: 3, age: 4 * time.Minute},
	})

	monitors, err := parseHeartbeatJSON([]byte(body), metas, time.Now().UTC())
	require.NoError(t, err)
	require.Len(t, monitors, 4)

	byID := map[int]RemoteMonitor{}
	for _, monitor := range monitors {
		byID[monitor.ID] = monitor
	}
	assert.Equal(t, RemoteStatusUp, byID[37].Status)
	assert.Equal(t, RemoteStatusDown, byID[29].Status)
	assert.Equal(t, RemoteStatusPending, byID[42].Status)
	assert.Equal(t, RemoteStatusMaintenance, byID[50].Status)
	assert.Equal(t, "retrying", byID[42].Message)
	// Freshness is derived from the newest sample, so a monitor that reported a
	// minute ago must not be treated as stale.
	assert.Positive(t, byID[37].AgeSec)
	assert.Less(t, byID[37].AgeSec, int64(120))
}

func TestParseHeartbeatJSONRejectsMalformedInput(t *testing.T) {
	metas := map[int]remoteMonitorMeta{37: {name: "源站"}}
	for name, body := range map[string]string{
		"truncated":     `{"heartbeatList": {"37": [{"status":`,
		"missing field": `{"uptimeList": {}}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseHeartbeatJSON([]byte(body), metas, time.Now().UTC())
			require.Error(t, err)
		})
	}
}

func TestParseHeartbeatJSONHandlesMonitorWithoutSamples(t *testing.T) {
	metas := map[int]remoteMonitorMeta{37: {name: "源站"}}
	monitors, err := parseHeartbeatJSON([]byte(`{"heartbeatList": {}}`), metas, time.Now().UTC())
	require.NoError(t, err)
	require.Len(t, monitors, 1)
	assert.Equal(t, RemoteStatusUnknown, monitors[0].Status)
	assert.Equal(t, int64(-1), monitors[0].AgeSec)
}

func TestDecodeBoundedBodyRejectsOversizedResponse(t *testing.T) {
	_, err := decodeBoundedBody(strings.NewReader(strings.Repeat("a", maxStatusSyncResponseBytes+1)))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds")

	data, err := decodeBoundedBody(strings.NewReader(`{"ok":true}`))
	require.NoError(t, err)
	assert.Equal(t, `{"ok":true}`, string(data))
}

func TestParseHeartbeatTime(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 45, 57, 0, time.UTC)
	cases := []struct {
		name string
		raw  string
		want int64
	}{
		{name: "millisecond precision", raw: "2026-09-17 12:44:57.000", want: 60},
		{name: "second precision", raw: "2026-09-17 12:40:57", want: 300},
		{name: "rfc3339", raw: "2026-09-17T12:45:57Z", want: 0},
		{name: "empty", raw: "", want: -1},
		{name: "unparsable", raw: "yesterday", want: -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, parseHeartbeatTime(tc.raw, now))
		})
	}
}

func newTestChannel(id int, name string, status int) *model.Channel {
	return &model.Channel{Id: id, Name: name, Status: status}
}

func baseOptions() StatusSyncOptions {
	return StatusSyncOptions{
		SourceURL:          "https://status.s3.fan/status/s3ai-api",
		DisableOnDown:      true,
		EnableOnUp:         true,
		StaleAfterMinutes:  30,
		MaxDisableFraction: 0.5,
	}
}

func TestBuildStatusSyncPlanActions(t *testing.T) {
	cases := []struct {
		name         string
		remoteStatus RemoteStatusKind
		remoteAgeSec int64
		channelState int
		options      func(StatusSyncOptions) StatusSyncOptions
		wantAction   SyncAction
		wantTarget   int
	}{
		{
			name:         "down monitor auto-disables an enabled channel",
			remoteStatus: RemoteStatusDown, remoteAgeSec: 60, channelState: common.ChannelStatusEnabled,
			wantAction: SyncActionDisable, wantTarget: common.ChannelStatusAutoDisabled,
		},
		{
			name:         "down monitor leaves an already auto-disabled channel alone",
			remoteStatus: RemoteStatusDown, remoteAgeSec: 60, channelState: common.ChannelStatusAutoDisabled,
			wantAction: SyncActionNoop,
		},
		{
			name:         "down monitor never overrides a human disable",
			remoteStatus: RemoteStatusDown, remoteAgeSec: 60, channelState: common.ChannelStatusManuallyDisabled,
			wantAction: SyncActionNoop,
		},
		{
			name:         "recovered monitor re-enables what automation disabled",
			remoteStatus: RemoteStatusUp, remoteAgeSec: 60, channelState: common.ChannelStatusAutoDisabled,
			wantAction: SyncActionEnable, wantTarget: common.ChannelStatusEnabled,
		},
		{
			name:         "recovered monitor never overrides a human disable",
			remoteStatus: RemoteStatusUp, remoteAgeSec: 60, channelState: common.ChannelStatusManuallyDisabled,
			wantAction: SyncActionSkip,
		},
		{
			name:         "enabled channel with an up monitor is a no-op",
			remoteStatus: RemoteStatusUp, remoteAgeSec: 60, channelState: common.ChannelStatusEnabled,
			wantAction: SyncActionNoop,
		},
		{
			name:         "pending monitor is not actionable",
			remoteStatus: RemoteStatusPending, remoteAgeSec: 60, channelState: common.ChannelStatusEnabled,
			wantAction: SyncActionSkip,
		},
		{
			name:         "maintenance monitor is not actionable",
			remoteStatus: RemoteStatusMaintenance, remoteAgeSec: 60, channelState: common.ChannelStatusEnabled,
			wantAction: SyncActionSkip,
		},
		{
			name:         "stale heartbeat is not actionable",
			remoteStatus: RemoteStatusDown, remoteAgeSec: 3600, channelState: common.ChannelStatusEnabled,
			wantAction: SyncActionSkip,
		},
		{
			name:         "unparsable heartbeat time is not actionable",
			remoteStatus: RemoteStatusDown, remoteAgeSec: -1, channelState: common.ChannelStatusEnabled,
			wantAction: SyncActionSkip,
		},
		{
			name:         "disable_on_down off suppresses the disable",
			remoteStatus: RemoteStatusDown, remoteAgeSec: 60, channelState: common.ChannelStatusEnabled,
			options:    func(o StatusSyncOptions) StatusSyncOptions { o.DisableOnDown = false; return o },
			wantAction: SyncActionSkip,
		},
		{
			name:         "enable_on_up off suppresses the re-enable",
			remoteStatus: RemoteStatusUp, remoteAgeSec: 60, channelState: common.ChannelStatusAutoDisabled,
			options:    func(o StatusSyncOptions) StatusSyncOptions { o.EnableOnUp = false; return o },
			wantAction: SyncActionSkip,
		},
		{
			name:         "staleness check disabled still acts on an old heartbeat",
			remoteStatus: RemoteStatusDown, remoteAgeSec: 86400, channelState: common.ChannelStatusEnabled,
			options:    func(o StatusSyncOptions) StatusSyncOptions { o.StaleAfterMinutes = 0; return o },
			wantAction: SyncActionDisable, wantTarget: common.ChannelStatusAutoDisabled,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			options := baseOptions()
			if tc.options != nil {
				options = tc.options(options)
			}
			monitors := []RemoteMonitor{{
				ID: 29, Name: "主站", Group: "S3AI API",
				Status: tc.remoteStatus, AgeSec: tc.remoteAgeSec,
			}}
			channels := []*model.Channel{newTestChannel(7, "主站", tc.channelState)}

			plan := BuildStatusSyncPlan(options, monitors, channels)
			require.Len(t, plan.Entries, 1)
			entry := plan.Entries[0]
			assert.Equal(t, tc.wantAction, entry.Action, entry.Reason)
			assert.Equal(t, tc.channelState, entry.CurrentStatus)
			if tc.wantAction == SyncActionDisable || tc.wantAction == SyncActionEnable {
				assert.Equal(t, tc.wantTarget, entry.TargetStatus)
			}
		})
	}
}

func TestBuildStatusSyncPlanRefusesAmbiguousAndUnmatchedMonitors(t *testing.T) {
	options := baseOptions()
	downMonitor := func(id int, name string) RemoteMonitor {
		return RemoteMonitor{ID: id, Name: name, Status: RemoteStatusDown, AgeSec: 60}
	}

	t.Run("one monitor matching two channels changes nothing", func(t *testing.T) {
		channels := []*model.Channel{
			newTestChannel(1, "主站", common.ChannelStatusEnabled),
			newTestChannel(2, "主站", common.ChannelStatusEnabled),
		}
		plan := BuildStatusSyncPlan(options, []RemoteMonitor{downMonitor(29, "主站")}, channels)
		require.Len(t, plan.Entries, 2)
		for _, entry := range plan.Entries {
			assert.Equal(t, SyncActionSkip, entry.Action)
			assert.Contains(t, entry.Reason, "refusing to guess")
		}
		assert.Zero(t, plan.DisableCount)
	})

	t.Run("one channel claimed by two monitors changes nothing", func(t *testing.T) {
		channels := []*model.Channel{newTestChannel(1, "主站", common.ChannelStatusEnabled)}
		plan := BuildStatusSyncPlan(options,
			[]RemoteMonitor{downMonitor(29, "主站"), downMonitor(30, "主站")}, channels)
		require.Len(t, plan.Entries, 2)
		for _, entry := range plan.Entries {
			assert.Equal(t, SyncActionSkip, entry.Action)
			assert.Contains(t, entry.Reason, "claimed by 2 monitors")
		}
		assert.Zero(t, plan.DisableCount)
	})

	t.Run("unmatched monitor produces no entry", func(t *testing.T) {
		channels := []*model.Channel{newTestChannel(1, "主站", common.ChannelStatusEnabled)}
		plan := BuildStatusSyncPlan(options, []RemoteMonitor{downMonitor(99, "未配置的监控")}, channels)
		assert.Empty(t, plan.Entries)
	})

	t.Run("case and whitespace differences still match", func(t *testing.T) {
		channels := []*model.Channel{newTestChannel(1, "  主站 ", common.ChannelStatusEnabled)}
		plan := BuildStatusSyncPlan(options, []RemoteMonitor{downMonitor(29, "主站")}, channels)
		require.Len(t, plan.Entries, 1)
		assert.Equal(t, SyncActionDisable, plan.Entries[0].Action)
	})
}

func TestBuildStatusSyncPlanUsesAliases(t *testing.T) {
	t.Run("alias maps a remote name onto a differently named channel", func(t *testing.T) {
		options := baseOptions()
		options.Aliases = map[string]string{"主站": "s3-origin"}
		monitors := []RemoteMonitor{{ID: 29, Name: "主站", Status: RemoteStatusDown, AgeSec: 60}}
		channels := []*model.Channel{newTestChannel(1, "s3-origin", common.ChannelStatusEnabled)}

		plan := BuildStatusSyncPlan(options, monitors, channels)
		require.Len(t, plan.Entries, 1)
		assert.Equal(t, SyncActionDisable, plan.Entries[0].Action)
		assert.Equal(t, 1, plan.Entries[0].ChannelID)
	})

	t.Run("alias pointing at a missing channel is reported, not guessed", func(t *testing.T) {
		options := baseOptions()
		options.Aliases = map[string]string{"主站": "does-not-exist"}
		monitors := []RemoteMonitor{{ID: 29, Name: "主站", Status: RemoteStatusDown, AgeSec: 60}}
		channels := []*model.Channel{newTestChannel(1, "主站", common.ChannelStatusEnabled)}

		plan := BuildStatusSyncPlan(options, monitors, channels)
		assert.Empty(t, plan.Entries)
		require.Len(t, plan.Warnings, 1)
		assert.Contains(t, plan.Warnings[0], "matches no channel")
	})
}

func TestBuildStatusSyncPlanSanityLimit(t *testing.T) {
	buildFleet := func() ([]RemoteMonitor, []*model.Channel) {
		monitors := make([]RemoteMonitor, 0, 3)
		channels := make([]*model.Channel, 0, 10)
		for i := range 10 {
			name := fmt.Sprintf("channel-%d", i)
			channels = append(channels, newTestChannel(i+1, name, common.ChannelStatusEnabled))
			if i < 3 {
				monitors = append(monitors, RemoteMonitor{
					ID: 100 + i, Name: name, Status: RemoteStatusDown, AgeSec: 60,
				})
			}
		}
		return monitors, channels
	}

	t.Run("a mass-down response is refused", func(t *testing.T) {
		options := baseOptions()
		options.MaxDisableFraction = 0.1 // allows 1 of 10
		monitors, channels := buildFleet()

		plan := BuildStatusSyncPlan(options, monitors, channels)
		assert.True(t, plan.Blocked)
		assert.Contains(t, plan.BlockedReason, "would disable 3 of 10 channels")
		assert.Equal(t, 3, plan.DisableCount)

		_, err := ApplyStatusSyncPlan(&plan)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "blocked")
	})

	t.Run("a change set within the fraction is allowed", func(t *testing.T) {
		options := baseOptions()
		options.MaxDisableFraction = 0.5 // allows 5 of 10
		monitors, channels := buildFleet()

		plan := BuildStatusSyncPlan(options, monitors, channels)
		assert.False(t, plan.Blocked)
		assert.Equal(t, 3, plan.DisableCount)
	})

	t.Run("an empty monitor set is refused", func(t *testing.T) {
		plan := BuildStatusSyncPlan(baseOptions(), nil, []*model.Channel{newTestChannel(1, "主站", common.ChannelStatusEnabled)})
		assert.True(t, plan.Blocked)
		assert.Contains(t, plan.BlockedReason, "no monitors")
	})
}

func TestApplyStatusSyncPlanRejectsInvalidInput(t *testing.T) {
	t.Run("nil plan", func(t *testing.T) {
		_, err := ApplyStatusSyncPlan(nil)
		require.Error(t, err)
	})
	t.Run("plan with nothing to do changes nothing", func(t *testing.T) {
		changed, err := ApplyStatusSyncPlan(&StatusSyncPlan{Entries: []StatusSyncPlanEntry{
			{ChannelID: 1, Action: SyncActionSkip, Reason: "unmatched"},
			{ChannelID: 2, Action: SyncActionNoop, Reason: "already enabled"},
		}})
		require.NoError(t, err)
		assert.Zero(t, changed)
	})
}
