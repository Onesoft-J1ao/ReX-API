package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/console_setting"

	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
)

const (
	requestTimeout   = 30 * time.Second
	httpTimeout      = 10 * time.Second
	uptimeKeySuffix  = "_24"
	apiStatusPath    = "/api/status-page/"
	apiHeartbeatPath = "/api/status-page/heartbeat/"

	// upstreamStatusCacheTTL stops a third-party status page from being scraped
	// once per dashboard load. Our own Kuma is on the LAN and cheap to poll; the
	// upstream is somebody else's server.
	upstreamStatusCacheTTL = 60 * time.Second
)

var statusPageSlugPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// hostnamePattern matches a bare hostname (optionally with a scheme and path), a
// dotted domain, or a bare IPv4 address.
//
// The mirrored page belongs to a supplier, and its monitor names are republished
// on our console to every visitor. A monitor that happens to be named after a
// real host would leak where our capacity comes from, so the mirrored group is
// scrubbed before it is served. Any path is consumed too, since a slug can name
// the provider just as well as the host. Digits-only sequences (model versions
// such as "2.5") do not match: the trailing label must be all letters.
var hostnamePattern = regexp.MustCompile(
	`(?i)\b(?:https?://)?(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+[a-z]{2,}(?:/\S*)?|` +
		`\b\d{1,3}(?:\.\d{1,3}){3}\b`,
)

// maskedHost is what a scrubbed address is replaced with. It keeps the monitor
// readable ("源站 •••") instead of dropping the row.
const maskedHost = "•••"

// maskUpstreamIdentity removes host-shaped text from a mirrored status page.
func maskUpstreamIdentity(group UptimeGroupResult) UptimeGroupResult {
	for i := range group.Monitors {
		group.Monitors[i].Name = hostnamePattern.ReplaceAllString(group.Monitors[i].Name, maskedHost)
		group.Monitors[i].Group = hostnamePattern.ReplaceAllString(group.Monitors[i].Group, maskedHost)
	}
	return group
}

// flattenUpstreamGroups rewrites a mirrored page so every monitor reads as one
// entry under the console's own heading. The provider's own group names
// ("S3AI API", "号池", …) would advertise where our capacity comes from, so they
// are dropped while the monitors themselves stay.
func flattenUpstreamGroups(group UptimeGroupResult, categoryName string) UptimeGroupResult {
	group.CategoryName = categoryName
	for i := range group.Monitors {
		group.Monitors[i].Group = ""
	}
	return group
}

type Monitor struct {
	Name   string  `json:"name"`
	Uptime float64 `json:"uptime"`
	Status int     `json:"status"`
	Group  string  `json:"group,omitempty"`
}

type UptimeGroupResult struct {
	CategoryName string    `json:"categoryName"`
	Monitors     []Monitor `json:"monitors"`
}

// ParseStatusPageURL splits a status page URL into the base the API lives on and
// the page slug: https://status.example.com/status/my-page -> (https://status.example.com, my-page).
//
// Operators paste the URL they see in the browser, so the /status/<slug> shape is
// read rather than asked for as two separate fields.
func ParseStatusPageURL(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", errors.New("status page URL is empty")
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", "", fmt.Errorf("status page URL is unparseable: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", "", fmt.Errorf("status page URL has unsupported scheme %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return "", "", errors.New("status page URL has no host")
	}

	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	marker := -1
	// Walk backwards so a base path that itself contains "status" still resolves.
	for i := len(segments) - 2; i >= 0; i-- {
		if segments[i] == "status" {
			marker = i
			break
		}
	}
	if marker < 0 {
		return "", "", fmt.Errorf("status page URL has no /status/<slug> path: %s", raw)
	}

	slug := segments[marker+1]
	if !statusPageSlugPattern.MatchString(slug) {
		return "", "", fmt.Errorf("status page URL has unexpected slug %q", slug)
	}

	base := parsed.Scheme + "://" + parsed.Host
	if prefix := strings.Join(segments[:marker], "/"); prefix != "" {
		base += "/" + prefix
	}
	return base, slug, nil
}

func getAndDecode(ctx context.Context, client *http.Client, url string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New("non-200 status")
	}

	return json.NewDecoder(resp.Body).Decode(dest)
}

func fetchGroupData(ctx context.Context, client *http.Client, baseURL string, slug string, categoryName string) UptimeGroupResult {
	result := UptimeGroupResult{
		CategoryName: categoryName,
		Monitors:     []Monitor{},
	}

	baseURL = strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	if baseURL == "" || slug == "" {
		return result
	}

	var statusData struct {
		PublicGroupList []struct {
			ID          int    `json:"id"`
			Name        string `json:"name"`
			MonitorList []struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			} `json:"monitorList"`
		} `json:"publicGroupList"`
	}

	var heartbeatData struct {
		HeartbeatList map[string][]struct {
			Status int    `json:"status"`
			Time   string `json:"time"`
		} `json:"heartbeatList"`
		UptimeList map[string]float64 `json:"uptimeList"`
	}

	g, gCtx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return getAndDecode(gCtx, client, baseURL+apiStatusPath+slug, &statusData)
	})
	g.Go(func() error {
		return getAndDecode(gCtx, client, baseURL+apiHeartbeatPath+slug, &heartbeatData)
	})

	if g.Wait() != nil {
		return result
	}

	for _, pg := range statusData.PublicGroupList {
		if len(pg.MonitorList) == 0 {
			continue
		}

		for _, m := range pg.MonitorList {
			monitor := Monitor{
				Name:  m.Name,
				Group: pg.Name,
			}

			monitorID := strconv.Itoa(m.ID)

			if uptime, exists := heartbeatData.UptimeList[monitorID+uptimeKeySuffix]; exists {
				monitor.Uptime = uptime
			}

			// Kuma returns the recent beats oldest-first, so the current state is
			// the newest timestamp rather than the first entry.
			if beats := heartbeatData.HeartbeatList[monitorID]; len(beats) > 0 {
				latest := beats[0]
				for _, beat := range beats[1:] {
					if beat.Time > latest.Time {
						latest = beat
					}
				}
				monitor.Status = latest.Status
			}

			result.Monitors = append(result.Monitors, monitor)
		}
	}

	return result
}

type upstreamStatusCacheEntry struct {
	key   string
	at    time.Time
	group UptimeGroupResult
}

var upstreamStatusCache struct {
	sync.Mutex
	entry upstreamStatusCacheEntry
}

// fetchUpstreamGroup serves the mirrored status page from a short-lived cache so
// that a burst of dashboard loads becomes one scrape of the provider.
func fetchUpstreamGroup(ctx context.Context, client *http.Client, baseURL string, slug string, categoryName string) UptimeGroupResult {
	key := baseURL + "/status/" + slug

	upstreamStatusCache.Lock()
	if upstreamStatusCache.entry.key == key && time.Since(upstreamStatusCache.entry.at) < upstreamStatusCacheTTL {
		cached := upstreamStatusCache.entry.group
		upstreamStatusCache.Unlock()
		return cached
	}
	upstreamStatusCache.Unlock()

	group := fetchGroupData(ctx, client, baseURL, slug, categoryName)
	// Scrub before caching so the masked form is what every later read serves.
	group = maskUpstreamIdentity(group)
	group = flattenUpstreamGroups(group, categoryName)
	// Only an answer with monitors is worth caching; a failed scrape should be
	// retried on the next load instead of pinning an empty panel for a minute.
	if len(group.Monitors) > 0 {
		upstreamStatusCache.Lock()
		upstreamStatusCache.entry = upstreamStatusCacheEntry{key: key, at: time.Now(), group: group}
		upstreamStatusCache.Unlock()
	}
	return group
}

func GetUptimeKumaStatus(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()

	client := &http.Client{Timeout: httpTimeout}

	type fetchTarget struct {
		baseURL      string
		slug         string
		categoryName string
		upstream     bool
	}

	configuredGroups := console_setting.GetUptimeKumaGroups()
	targets := make([]fetchTarget, 0, len(configuredGroups)+1)
	for _, group := range configuredGroups {
		url, _ := group["url"].(string)
		slug, _ := group["slug"].(string)
		categoryName, _ := group["categoryName"].(string)
		targets = append(targets, fetchTarget{baseURL: url, slug: slug, categoryName: categoryName})
	}

	settings := console_setting.GetConsoleSetting()
	if settings.UpstreamStatusEnabled {
		baseURL, slug, err := ParseStatusPageURL(settings.UpstreamStatusUrl)
		if err != nil {
			common.SysError("upstream status page is unusable: " + err.Error())
		} else {
			targets = append(targets, fetchTarget{
				baseURL:      baseURL,
				slug:         slug,
				categoryName: settings.UpstreamStatusName,
				upstream:     true,
			})
		}
	}

	results := make([]UptimeGroupResult, len(targets))
	g, gCtx := errgroup.WithContext(ctx)
	for i, target := range targets {
		g.Go(func() error {
			if target.upstream {
				results[i] = fetchUpstreamGroup(gCtx, client, target.baseURL, target.slug, target.categoryName)
			} else {
				results[i] = fetchGroupData(gCtx, client, target.baseURL, target.slug, target.categoryName)
			}
			return nil
		})
	}

	g.Wait()
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": results})
}
