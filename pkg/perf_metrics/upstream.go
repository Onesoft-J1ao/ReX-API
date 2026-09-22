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

// upstreamSummaryCache holds the last successful mirror per requested window.
// A window nobody has asked for has no entry, and the request that asks for it
// reports no mirror rather than a different window's numbers.
var upstreamSummaryCache struct {
	sync.Mutex
	entries map[int]upstreamCacheEntry
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
	if !upstreamConfigured() {
		return SummaryAllResult{}, false
	}

	upstreamSummaryCache.Lock()
	defer upstreamSummaryCache.Unlock()
	entry, ok := upstreamSummaryCache.entries[hours]
	if !ok || len(entry.result.Models) == 0 {
		return SummaryAllResult{}, false
	}
	return entry.result, true
}

// upstreamModelSummary looks one model up in the mirrored summary. It backs the
// per-model detail view for models this instance mirrors from upstream but has
// not carried enough traffic for on its own.
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
	setting := perf_metrics_setting.GetSetting()
	if !setting.UpstreamEnabled {
		return false
	}
	return strings.TrimSpace(setting.UpstreamSummaryUrl) != ""
}

func upstreamSummaryUrl(hours int) string {
	base := strings.TrimSpace(perf_metrics_setting.GetSetting().UpstreamSummaryUrl)
	return fmt.Sprintf("%s?hours=%d", strings.TrimRight(base, "/"), hours)
}

// upstreamRefreshLoop keeps the mirrored summary warm. It refreshes immediately
// on start so a restart does not leave the console blank for a whole interval.
func upstreamRefreshLoop(hours []int) {
	for _, window := range hours {
		refreshUpstreamSummary(window)
	}
	ticker := time.NewTicker(upstreamRefreshInterval)
	defer ticker.Stop()
	for range ticker.C {
		for _, window := range hours {
			refreshUpstreamSummary(window)
		}
	}
}

func refreshUpstreamSummary(hours int) {
	if !upstreamConfigured() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), perf_metrics_setting.GetUpstreamTimeout())
	defer cancel()

	result, err := requestUpstreamSummary(ctx, upstreamSummaryUrl(hours))
	if err != nil || len(result.Models) == 0 {
		if err != nil {
			common.SysLog(fmt.Sprintf("perf metrics upstream fetch failed, keeping the previous mirror: %s", err.Error()))
		}
		return
	}

	upstreamSummaryCache.Lock()
	if upstreamSummaryCache.entries == nil {
		upstreamSummaryCache.entries = map[int]upstreamCacheEntry{}
	}
	upstreamSummaryCache.entries[hours] = upstreamCacheEntry{at: time.Now(), result: result}
	upstreamSummaryCache.Unlock()
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
