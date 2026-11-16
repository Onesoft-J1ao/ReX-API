package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseStatusPageURL(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		baseURL string
		slug    string
		wantErr bool
	}{
		{
			name:    "plain status page URL",
			raw:     "https://status.example.com/status/default",
			baseURL: "https://status.example.com",
			slug:    "default",
		},
		{
			name:    "trailing slash",
			raw:     "https://status.example.com/status/default/",
			baseURL: "https://status.example.com",
			slug:    "default",
		},
		{
			name:    "surrounding whitespace",
			raw:     "  https://status.example.com/status/default  ",
			baseURL: "https://status.example.com",
			slug:    "default",
		},
		{
			name:    "query and fragment are ignored",
			raw:     "https://status.example.com/status/my-page?tab=1#top",
			baseURL: "https://status.example.com",
			slug:    "my-page",
		},
		{
			name:    "non-default port is kept",
			raw:     "http://kuma.internal:3001/status/status",
			baseURL: "http://kuma.internal:3001",
			slug:    "status",
		},
		{
			name:    "base path before /status is kept",
			raw:     "https://example.com/uptime/status/page-1",
			baseURL: "https://example.com/uptime",
			slug:    "page-1",
		},
		{
			// A base path may itself contain the word, so the last one wins.
			name:    "rightmost status segment wins",
			raw:     "https://example.com/status/status/page-2",
			baseURL: "https://example.com/status",
			slug:    "page-2",
		},
		{
			name:    "slug with underscore and dash",
			raw:     "https://s.example.com/status/a_B-9",
			baseURL: "https://s.example.com",
			slug:    "a_B-9",
		},
		{name: "empty", raw: "   ", wantErr: true},
		{name: "no scheme", raw: "status.example.com/status/x", wantErr: true},
		{name: "unsupported scheme", raw: "ftp://status.example.com/status/x", wantErr: true},
		{name: "javascript scheme", raw: "javascript:alert(1)", wantErr: true},
		{name: "no host", raw: "/status/x", wantErr: true},
		{name: "not a status page", raw: "https://example.com/dashboard", wantErr: true},
		{name: "bare status path", raw: "https://example.com/status", wantErr: true},
		{name: "slug has a space", raw: "https://example.com/status/a b", wantErr: true},
		{name: "slug traverses", raw: "https://example.com/status/..", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			baseURL, slug, err := ParseStatusPageURL(tc.raw)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.baseURL, baseURL)
			assert.Equal(t, tc.slug, slug)
		})
	}
}

// Kuma returns recent beats oldest-first, so the monitor's current state is the
// newest entry by timestamp rather than by position. The beats are carried
// through as well: the console renders them as a status strip.
func TestFetchGroupDataReportsTheNewestHeartbeat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/status-page/demo":
			_, _ = w.Write([]byte(`{"publicGroupList":[{"id":13,"name":"Example API","monitorList":[{"id":29,"name":"主站"},{"id":37,"name":"源站"}]}]}`))
		case "/api/status-page/heartbeat/demo":
			_, _ = w.Write([]byte(`{"heartbeatList":{` +
				`"29":[{"status":1,"time":"2026-01-01 00:00:00.000"},{"status":0,"time":"2026-01-01 00:05:00.000"}],` +
				`"37":[{"status":1,"time":"2026-01-01 00:05:00.000"},{"status":1,"time":"2026-01-01 00:00:00.000"}]},` +
				`"uptimeList":{"29_24":0.5,"37_24":1}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	group := fetchGroupData(context.Background(), server.Client(), server.URL, "demo", "上游状态")

	require.Equal(t, "上游状态", group.CategoryName)
	require.Len(t, group.Monitors, 2)
	// 主站's beats are oldest-first with the newest at 00:05 and status 0, so the
	// reported status is 0 even though the first element says 1.
	assert.Equal(t, Monitor{
		Name:   "主站",
		Group:  "Example API",
		Status: 0,
		Uptime: 0.5,
		Heartbeats: []UptimeHeartbeat{
			{Status: 1, Time: "2026-01-01 00:00:00.000"},
			{Status: 0, Time: "2026-01-01 00:05:00.000"},
		},
	}, group.Monitors[0])
	assert.Equal(t, Monitor{
		Name:   "源站",
		Group:  "Example API",
		Status: 1,
		Uptime: 1,
		Heartbeats: []UptimeHeartbeat{
			{Status: 1, Time: "2026-01-01 00:05:00.000"},
			{Status: 1, Time: "2026-01-01 00:00:00.000"},
		},
	}, group.Monitors[1])
}

// The group + multiplier pair the operator publishes lives in a Kuma tag, so a
// fetcher that drops tags renders the whole feature as blank. This pins the
// mapping: tag name is the group, tag value is the multiplier.
func TestFetchGroupedDataCarriesMonitorTagsThrough(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/status-page/demo":
			_, _ = w.Write([]byte(`{"publicGroupList":[{"id":1,"name":"线路状态","monitorList":[{"id":5,"name":"线路 B","tags":[{"name":"ClaudeMax","value":"1x","color":"#10b981"},{"name":"Kiro企业","value":"0.4x","color":"#f59e0b"}]}]}]}`))
		case "/api/status-page/heartbeat/demo":
			_, _ = w.Write([]byte(`{"heartbeatList":{"5":[{"status":1,"time":"2026-01-01 00:00:00.000"}]},"uptimeList":{"5_24":1}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	groups := fetchGroupedData(context.Background(), server.Client(), server.URL, "demo", "监控")
	require.Len(t, groups, 1)
	require.Len(t, groups[0].Monitors, 1)
	assert.Equal(t, []MonitorTag{
		{Name: "ClaudeMax", Value: "1x", Color: "#10b981"},
		{Name: "Kiro企业", Value: "0.4x", Color: "#f59e0b"},
	}, groups[0].Monitors[0].Tags)
}

// A tag is free text the provider typed, so it is scrubbed the same way a
// monitor name is: a mirrored page must not republish a supplier's domain.
func TestMaskUpstreamIdentityScrubsTags(t *testing.T) {
	group := UptimeGroupResult{Monitors: []Monitor{{
		Name: "主站",
		Tags: []MonitorTag{{Name: "api.example.com", Value: "1x"}},
	}}}

	masked := maskUpstreamIdentity(group)

	require.Len(t, masked.Monitors[0].Tags, 1)
	assert.Equal(t, maskedHost, masked.Monitors[0].Tags[0].Name)
	assert.Equal(t, "1x", masked.Monitors[0].Tags[0].Value)
}

func TestFetchGroupDataReturnsNoMonitorsWhenTheStatusPageIsUnreachable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	group := fetchGroupData(context.Background(), server.Client(), server.URL, "demo", "上游状态")

	assert.Equal(t, "上游状态", group.CategoryName)
	assert.Empty(t, group.Monitors)
}

func TestFetchGroupDataSkipsAnIncompleteTarget(t *testing.T) {
	group := fetchGroupData(context.Background(), &http.Client{Timeout: time.Second}, "", "", "上游状态")

	assert.Empty(t, group.Monitors)
}

// The mirrored page belongs to a supplier and is republished to every visitor, so
// a monitor named after a real host would advertise where our capacity comes from.
func TestMaskUpstreamIdentityScrubsHostsButKeepsModelNames(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"bare domain", "主站 ai.furry.vg", "主站 •••"},
		{"own domain", "主站 ai.j1ao.vip", "主站 •••"},
		{"full url", "https://status.example.com/status/default", "•••"},
		{"ipv4", "源站 192.168.1.1", "源站 •••"},
		{"subdomain chain", "节点 a.b.c.example.com", "节点 •••"},
		{"model version survives", "Gemini 2.5 Flash", "Gemini 2.5 Flash"},
		{"dashed model survives", "gpt-5.6-luna", "gpt-5.6-luna"},
		{"tagged model survives", "deepseek-v4-pro-0813", "deepseek-v4-pro-0813"},
		{"vendor path survives", "nvidia/nemotron-3.5-content-safety", "nvidia/nemotron-3.5-content-safety"},
		{"plain label survives", "号池", "号池"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			group := maskUpstreamIdentity(UptimeGroupResult{
				CategoryName: "上游状态",
				Monitors:     []Monitor{{Name: tc.in, Group: tc.in}},
			})
			assert.Equal(t, tc.want, group.Monitors[0].Name)
			assert.Equal(t, tc.want, group.Monitors[0].Group)
		})
	}
}

func TestFetchUpstreamGroupServesTheMaskedForm(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/status-page/demo":
			_, _ = w.Write([]byte(`{"publicGroupList":[{"id":13,"name":"ai.furry.vg","monitorList":[{"id":29,"name":"主站 ai.furry.vg"}]}]}`))
		case "/api/status-page/heartbeat/demo":
			_, _ = w.Write([]byte(`{"heartbeatList":{"29":[{"status":1,"time":"2026-01-01 00:05:00.000"}]},"uptimeList":{"29_24":1}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	group := fetchUpstreamGroup(context.Background(), server.Client(), server.URL, "demo", "上游状态")

	require.Len(t, group.Monitors, 1)
	assert.Equal(t, "主站 •••", group.Monitors[0].Name)
	// The provider's own group name is dropped: the monitor files under the
	// console's configured heading instead.
	assert.Empty(t, group.Monitors[0].Group)
	assert.Equal(t, "上游状态", group.CategoryName)
}

// The provider's own group names ("Example API", "号池") must never reach the
// console: they advertise where our capacity comes from. The monitors
// themselves stay, filed under the console's own heading.
func TestFlattenUpstreamGroupsDropsProviderGroupNames(t *testing.T) {
	group := flattenUpstreamGroups(UptimeGroupResult{
		CategoryName: "Example API",
		Monitors: []Monitor{
			{Name: "主站 •••", Group: "Example API"},
			{Name: "Kiro", Group: "号池"},
			{Name: "源站 •••", Group: "号池"},
		},
	}, "上游状态")

	assert.Equal(t, "上游状态", group.CategoryName)
	require.Len(t, group.Monitors, 3)
	for _, monitor := range group.Monitors {
		assert.Empty(t, monitor.Group,
			"the provider's group name must be dropped from every monitor")
	}
	assert.Equal(t, "Kiro", group.Monitors[1].Name,
		"the monitors themselves must survive the flatten")
}

func TestFetchGroupDataReturnsRecentHeartbeatHistory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/status-page/demo":
			_, _ = w.Write([]byte(`{"publicGroupList":[{"id":1,"name":"Core","monitorList":[{"id":7,"name":"Gateway"}]}]}`))
		case "/api/status-page/heartbeat/demo":
			history := make([]map[string]any, heartbeatLimit+2)
			baseTime := time.Date(2026, time.September, 13, 10, 0, 0, 0, time.UTC)
			for index := range heartbeatLimit + 2 {
				status := 1
				if index == heartbeatLimit+1 {
					status = 0
				}
				history[index] = map[string]any{
					"status": status,
					"time":   baseTime.Add(time.Duration(index) * time.Minute).Format("2006-01-02 15:04:05"),
					"msg":    "hidden",
					"ping":   12,
				}
			}
			payload, err := common.Marshal(map[string]any{
				"heartbeatList": map[string]any{
					"7": history,
					"8": []map[string]any{{"status": 1, "time": "ignored"}},
				},
				"uptimeList": map[string]float64{"7_24": 0.975},
			})
			require.NoError(t, err)
			_, _ = w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	result := fetchGroupData(context.Background(), server.Client(), server.URL, "demo", "Production")
	require.Len(t, result.Monitors, 1)
	monitor := result.Monitors[0]
	assert.Equal(t, "Production", result.CategoryName)
	assert.Equal(t, "Gateway", monitor.Name)
	assert.Equal(t, "Core", monitor.Group)
	assert.Equal(t, 0.975, monitor.Uptime)
	assert.Equal(t, 0, monitor.Status, "the newest heartbeat determines current status")
	require.Len(t, monitor.Heartbeats, heartbeatLimit)
	assert.Equal(t, "2026-09-13 10:02:00", monitor.Heartbeats[0].Time)
	assert.Equal(t, "2026-09-13 11:01:00", monitor.Heartbeats[heartbeatLimit-1].Time)

	payload, err := common.Marshal(monitor)
	require.NoError(t, err)
	assert.NotContains(t, string(payload), "msg")
	assert.NotContains(t, string(payload), "ping")
}

func TestFetchGroupDataReturnsEmptyResultWhenUpstreamFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)

	result := fetchGroupData(context.Background(), server.Client(), server.URL, "demo", "Production")
	assert.Equal(t, "Production", result.CategoryName)
	assert.Empty(t, result.Monitors)
}
