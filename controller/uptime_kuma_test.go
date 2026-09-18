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
