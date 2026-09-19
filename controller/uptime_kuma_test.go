package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
			raw:     "https://status.s3.fan/status/s3ai-api",
			baseURL: "https://status.s3.fan",
			slug:    "s3ai-api",
		},
		{
			name:    "trailing slash",
			raw:     "https://status.s3.fan/status/s3ai-api/",
			baseURL: "https://status.s3.fan",
			slug:    "s3ai-api",
		},
		{
			name:    "surrounding whitespace",
			raw:     "  https://status.s3.fan/status/s3ai-api  ",
			baseURL: "https://status.s3.fan",
			slug:    "s3ai-api",
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
// newest entry. Reading the first one would report a state from ~100 minutes ago.
func TestFetchGroupDataReportsTheNewestHeartbeat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/status-page/demo":
			_, _ = w.Write([]byte(`{"publicGroupList":[{"id":13,"name":"S3AI API","monitorList":[{"id":29,"name":"主站"},{"id":37,"name":"源站"}]}]}`))
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
	assert.Equal(t, Monitor{Name: "主站", Group: "S3AI API", Status: 0, Uptime: 0.5}, group.Monitors[0])
	assert.Equal(t, Monitor{Name: "源站", Group: "S3AI API", Status: 1, Uptime: 1}, group.Monitors[1])
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
		{"full url", "https://status.s3.fan/status/s3ai-api", "•••"},
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

// The provider's own group names ("S3AI API", "号池") must never reach the
// console: they advertise where our capacity comes from. The monitors
// themselves stay, filed under the console's own heading.
func TestFlattenUpstreamGroupsDropsProviderGroupNames(t *testing.T) {
	group := flattenUpstreamGroups(UptimeGroupResult{
		CategoryName: "S3AI API",
		Monitors: []Monitor{
			{Name: "主站 •••", Group: "S3AI API"},
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
