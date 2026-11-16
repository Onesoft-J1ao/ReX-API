package controller

import (
	"context"
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

	// heartbeatLimit caps how many recent beats are carried into the response.
	// Kuma returns up to 100 per monitor; the console draws a short history, so
	// the older ones would be dead weight on every dashboard load.
	heartbeatLimit = 60

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
		// Tags are free text the provider typed; a tag named after their own
		// domain would leak as surely as a monitor name would.
		for j := range group.Monitors[i].Tags {
			group.Monitors[i].Tags[j].Name = hostnamePattern.ReplaceAllString(group.Monitors[i].Tags[j].Name, maskedHost)
			group.Monitors[i].Tags[j].Value = hostnamePattern.ReplaceAllString(group.Monitors[i].Tags[j].Value, maskedHost)
		}
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

// UptimeHeartbeat is one Kuma beat: a status code and the timestamp it was
// recorded at. The console renders the recent ones as a history strip.
type UptimeHeartbeat struct {
	Status int    `json:"status"`
	Time   string `json:"time"`
}

// MonitorTag is one Kuma tag attached to a monitor. The operator writes the
// group name into the tag and the multiplier into its value, so the public
// status page can show which model groups a line serves and what each charges
// without a second data source.
type MonitorTag struct {
	Name  string `json:"name"`
	Value string `json:"value,omitempty"`
	Color string `json:"color,omitempty"`
}

type Monitor struct {
	Name       string            `json:"name"`
	Uptime     float64           `json:"uptime"`
	Status     int               `json:"status"`
	Group      string            `json:"group,omitempty"`
	Heartbeats []UptimeHeartbeat `json:"heartbeats,omitempty"`
	// Tags carries the group + multiplier pairs published on the Kuma page.
	Tags []MonitorTag `json:"tags,omitempty"`
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

	return common.DecodeJson(resp.Body, dest)
}

// fetchGroupedData returns one result per Kuma public group instead of
// flattening them.
//
// A single Kuma status page can publish several groups - upstream providers,
// CDN edges, the origin servers - and merging them into one list hides the very
// distinction the page exists to show. Each Kuma group becomes one result, with
// its name preserved; categoryName is only the fallback when Kuma publishes a
// single unnamed group.
func fetchGroupedData(ctx context.Context, client *http.Client, baseURL string, slug string, fallbackName string) []UptimeGroupResult {
	baseURL = strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	if baseURL == "" || slug == "" {
		return nil
	}

	var statusData struct {
		PublicGroupList []struct {
			ID          int    `json:"id"`
			Name        string `json:"name"`
			MonitorList []struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
				// Kuma returns a monitor's tags only when the status page has
				// showTags on. The value field is where the operator puts the
				// multiplier, so it is read verbatim.
				Tags []struct {
					Name  string `json:"name"`
					Value string `json:"value"`
					Color string `json:"color"`
				} `json:"tags"`
			} `json:"monitorList"`
		} `json:"publicGroupList"`
	}

	var heartbeatData struct {
		HeartbeatList map[string][]UptimeHeartbeat `json:"heartbeatList"`
		UptimeList    map[string]float64           `json:"uptimeList"`
	}

	g, gCtx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return getAndDecode(gCtx, client, baseURL+apiStatusPath+slug, &statusData)
	})
	g.Go(func() error {
		return getAndDecode(gCtx, client, baseURL+apiHeartbeatPath+slug, &heartbeatData)
	})
	if g.Wait() != nil {
		return nil
	}

	results := make([]UptimeGroupResult, 0, len(statusData.PublicGroupList))
	for _, pg := range statusData.PublicGroupList {
		if len(pg.MonitorList) == 0 {
			continue
		}
		name := strings.TrimSpace(pg.Name)
		if name == "" {
			name = fallbackName
		}
		group := UptimeGroupResult{CategoryName: name, Monitors: []Monitor{}}
		for _, m := range pg.MonitorList {
			monitor := Monitor{Name: m.Name, Group: name}
			for _, tag := range m.Tags {
				monitor.Tags = append(monitor.Tags, MonitorTag{Name: tag.Name, Value: tag.Value, Color: tag.Color})
			}
			monitorID := strconv.Itoa(m.ID)
			if uptime, exists := heartbeatData.UptimeList[monitorID+uptimeKeySuffix]; exists {
				monitor.Uptime = uptime
			}
			if heartbeats, exists := heartbeatData.HeartbeatList[monitorID]; exists && len(heartbeats) > 0 {
				start := max(0, len(heartbeats)-heartbeatLimit)
				monitor.Heartbeats = append([]UptimeHeartbeat(nil), heartbeats[start:]...)
				latest := monitor.Heartbeats[0]
				for _, beat := range monitor.Heartbeats[1:] {
					if beat.Time > latest.Time {
						latest = beat
					}
				}
				monitor.Status = latest.Status
			}
			group.Monitors = append(group.Monitors, monitor)
		}
		results = append(results, group)
	}
	return results
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
				Tags []struct {
					Name  string `json:"name"`
					Value string `json:"value"`
					Color string `json:"color"`
				} `json:"tags"`
			} `json:"monitorList"`
		} `json:"publicGroupList"`
	}

	var heartbeatData struct {
		HeartbeatList map[string][]UptimeHeartbeat `json:"heartbeatList"`
		UptimeList    map[string]float64           `json:"uptimeList"`
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
			for _, tag := range m.Tags {
				monitor.Tags = append(monitor.Tags, MonitorTag{Name: tag.Name, Value: tag.Value, Color: tag.Color})
			}

			monitorID := strconv.Itoa(m.ID)

			if uptime, exists := heartbeatData.UptimeList[monitorID+uptimeKeySuffix]; exists {
				monitor.Uptime = uptime
			}

			if heartbeats, exists := heartbeatData.HeartbeatList[monitorID]; exists && len(heartbeats) > 0 {
				start := max(0, len(heartbeats)-heartbeatLimit)
				monitor.Heartbeats = append([]UptimeHeartbeat(nil), heartbeats[start:]...)
				// Kuma returns the beats oldest-first (verified against the live
				// API: the last entry is the newest timestamp), so the current
				// state is the newest beat rather than a positional guess.
				latest := monitor.Heartbeats[0]
				for _, beat := range monitor.Heartbeats[1:] {
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

	// One configured status page can publish several groups; keep them apart so
	// the console mirrors the operator's own grouping.
	perTarget := make([][]UptimeGroupResult, len(targets))
	g, gCtx := errgroup.WithContext(ctx)
	for i, target := range targets {
		g.Go(func() error {
			if target.upstream {
				perTarget[i] = []UptimeGroupResult{fetchUpstreamGroup(gCtx, client, target.baseURL, target.slug, target.categoryName)}
			} else {
				perTarget[i] = fetchGroupedData(gCtx, client, target.baseURL, target.slug, target.categoryName)
			}
			return nil
		})
	}

	g.Wait()

	results := make([]UptimeGroupResult, 0, len(targets))
	for _, groups := range perTarget {
		results = append(results, groups...)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": results})
}
