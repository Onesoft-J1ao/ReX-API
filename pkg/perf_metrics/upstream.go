package perfmetrics

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
)

// upstreamRefreshInterval is how often the mirror is refreshed. The upstream
// answers in 7-19s (measured against the live instance), far longer than a page
// load can wait, so the fetch never runs on a request path: this loop owns the
// network call and a request serves whatever the loop last stored.
const upstreamRefreshInterval = 5 * time.Minute

// upstreamRetryInterval is how long to wait after a refresh that reached
// nothing. A cold start is the case that matters: the first attempt after a
// restart pays for DNS and TLS against a third-party host and can outlast its
// own timeout, and waiting the full refresh interval would leave the console
// without mirrored numbers for that whole window.
const upstreamRetryInterval = 30 * time.Second

// upstreamMaxBytes caps what a single fetch may buffer. The observed payload is
// ~50KB; anything far past that is not the response we expect.
const upstreamMaxBytes = 8 << 20

// upstreamEnvelope matches the new-api summary response shape, whose data is an
// object holding a models array rather than the bare array other sync endpoints
// return.
type upstreamEnvelope struct {
	Success bool             `json:"success"`
	Message string           `json:"message"`
	Data    SummaryAllResult `json:"data"`
}

type upstreamCacheEntry struct {
	at     time.Time
	result SummaryAllResult
}

type upstreamCacheKey struct {
	source string
	hours  int
}

// upstreamSummaryCache holds the last successful mirror per requested window.
// A window nobody has asked for has no entry, and the request that asks for it
// reports no mirror rather than a different window's numbers.
var upstreamSummaryCache struct {
	sync.Mutex
	entries map[upstreamCacheKey]upstreamCacheEntry
}

var (
	upstreamClientOnce sync.Once
	upstreamClient     *http.Client
)

func getUpstreamClient(timeout time.Duration) *http.Client {
	upstreamClientOnce.Do(func() {
		transport := &http.Transport{
			MaxIdleConns:          10,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   timeout,
			ExpectContinueTimeout: 1 * time.Second,
			ResponseHeaderTimeout: timeout,
		}
		if common.TLSInsecureSkipVerify {
			transport.TLSClientConfig = common.InsecureTLSConfig
		}
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return (&net.Dialer{Timeout: timeout}).DialContext(ctx, network, addr)
		}
		upstreamClient = &http.Client{Transport: transport}
	})
	return upstreamClient
}

// fetchUpstreamSummary returns the mirrored summary for one window. It never
// performs a fetch: the background loop owns the network call, so a page load
// cannot be stalled behind an upstream that takes longer to answer than the
// request is willing to wait.
func fetchUpstreamSummary(hours int) (SummaryAllResult, bool) {
	return fetchUpstreamSummaryFor("furry", hours)
}

func fetchUpstreamSummaryFor(source string, hours int) (SummaryAllResult, bool) {
	if !upstreamConfiguredFor(source) {
		return SummaryAllResult{}, false
	}

	upstreamSummaryCache.Lock()
	defer upstreamSummaryCache.Unlock()
	entry, ok := upstreamSummaryCache.entries[upstreamCacheKey{source: source, hours: hours}]
	if !ok || len(entry.result.Models) == 0 {
		return SummaryAllResult{}, false
	}
	return entry.result, true
}

// upstreamModelSummary looks one model up in the mirrored summary. It backs the
// per-model detail view for models this instance mirrors from upstream but has
// not carried enough traffic for on its own.
// UpstreamCoversModel reports whether the mirrored upstream summary includes a
// model. A model it does not include has no health data unless we probe it
// ourselves.
func UpstreamCoversModel(modelName string) bool {
	_, ok := upstreamModelSummary(modelName, 24)
	return ok
}

func upstreamModelSummary(modelName string, hours int) (ModelSummary, bool) {
	mirrored, ok := fetchUpstreamSummary(hours)
	if !ok {
		return ModelSummary{}, false
	}
	for _, model := range mirrored.Models {
		if model.ModelName == modelName {
			return model, true
		}
	}
	return ModelSummary{}, false
}

func upstreamConfigured() bool {
	return upstreamConfiguredFor("furry")
}

func upstreamSummaryUrl(hours int) string {
	return upstreamSummaryUrlFor("furry", hours)
}

func upstreamConfiguredFor(source string) bool {
	setting := perf_metrics_setting.GetSetting()
	if !setting.UpstreamEnabled {
		return false
	}
	return strings.TrimSpace(upstreamSummaryUrlBase(source)) != ""
}

func upstreamSummaryUrlFor(source string, hours int) string {
	base := strings.TrimSpace(upstreamSummaryUrlBase(source))
	return fmt.Sprintf("%s?hours=%d", strings.TrimRight(base, "/"), hours)
}

func upstreamSummaryUrlBase(source string) string {
	setting := perf_metrics_setting.GetSetting()
	if source == "furry" && strings.TrimSpace(setting.UpstreamSummaryUrl) != "" {
		return setting.UpstreamSummaryUrl
	}
	return setting.UpstreamSummaryUrls[source]
}

// upstreamRefreshLoop keeps the mirrored summary warm. It refreshes on start so
// a restart does not leave the console blank, and retries once soon after if
// that first attempt reached nothing.
func upstreamRefreshLoop(hours []int) {
	if !refreshAllUpstreamSummaries(hours) {
		time.Sleep(upstreamRetryInterval)
		refreshAllUpstreamSummaries(hours)
	}
	ticker := time.NewTicker(upstreamRefreshInterval)
	defer ticker.Stop()
	for range ticker.C {
		refreshAllUpstreamSummaries(hours)
	}
}

// refreshAllUpstreamSummaries reports whether at least one window was stored, so
// the loop knows whether the mirror is still cold.
func refreshAllUpstreamSummaries(hours []int) bool {
	stored := false
	for source := range perf_metrics_setting.GetSetting().UpstreamSummaryUrls {
		for _, window := range hours {
			if refreshUpstreamSummaryFor(source, window) {
				stored = true
			}
		}
	}
	// Keep the legacy single-url setting working when the source map is absent.
	if len(perf_metrics_setting.GetSetting().UpstreamSummaryUrls) == 0 {
		for _, window := range hours {
			if refreshUpstreamSummaryFor("furry", window) {
				stored = true
			}
		}
	}
	return stored
}

// refreshUpstreamSummary fetches and stores one window, reporting whether the
// mirror was updated.
func refreshUpstreamSummary(hours int) bool {
	return refreshUpstreamSummaryFor("furry", hours)
}

func refreshUpstreamSummaryFor(source string, hours int) bool {
	if !upstreamConfiguredFor(source) {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), perf_metrics_setting.GetUpstreamTimeout())
	defer cancel()

	result, err := requestUpstreamSummary(ctx, upstreamSummaryUrlFor(source, hours))
	if err != nil || len(result.Models) == 0 {
		if err != nil {
			common.SysLog(fmt.Sprintf("perf metrics upstream fetch failed, keeping the previous mirror: %s", err.Error()))
		}
		return false
	}

	upstreamSummaryCache.Lock()
	if upstreamSummaryCache.entries == nil {
		upstreamSummaryCache.entries = map[upstreamCacheKey]upstreamCacheEntry{}
	}
	upstreamSummaryCache.entries[upstreamCacheKey{source: source, hours: hours}] = upstreamCacheEntry{at: time.Now(), result: result}
	upstreamSummaryCache.Unlock()
	return true
}

func requestUpstreamSummary(ctx context.Context, url string) (SummaryAllResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return SummaryAllResult{}, err
	}
	resp, err := getUpstreamClient(perf_metrics_setting.GetUpstreamTimeout()).Do(req)
	if err != nil {
		return SummaryAllResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return SummaryAllResult{}, fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, upstreamMaxBytes+1))
	if err != nil {
		return SummaryAllResult{}, err
	}
	if len(body) > upstreamMaxBytes {
		return SummaryAllResult{}, errors.New("upstream summary exceeds size limit")
	}
	var envelope upstreamEnvelope
	if err := common.Unmarshal(body, &envelope); err != nil {
		return SummaryAllResult{}, err
	}
	if !envelope.Success {
		return SummaryAllResult{}, errors.New("upstream reported failure")
	}
	return envelope.Data, nil
}
