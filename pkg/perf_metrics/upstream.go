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

// upstreamSummaryTTL stops a third-party instance from being scraped once per
// page load. The payload is ~100KB and the upstream is somebody else's server,
// so the same numbers are reused for a minute.
const upstreamSummaryTTL = 60 * time.Second

// upstreamMaxBytes caps what a single fetch may buffer. The observed payload is
// ~100KB; anything far past that is not the response we expect.
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
	key    string
	at     time.Time
	result SummaryAllResult
}

var upstreamSummaryCache struct {
	sync.Mutex
	entry upstreamCacheEntry
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

// fetchUpstreamSummary returns the mirrored summary, or ok=false when the
// upstream is disabled, unreachable, or answers with something unusable. A
// failed fetch falls back to the last successful response rather than dropping
// the health data the page is showing.
func fetchUpstreamSummary(hours int) (SummaryAllResult, bool) {
	setting := perf_metrics_setting.GetSetting()
	if !setting.UpstreamEnabled {
		return SummaryAllResult{}, false
	}
	base := strings.TrimSpace(setting.UpstreamSummaryUrl)
	if base == "" {
		return SummaryAllResult{}, false
	}
	url := fmt.Sprintf("%s?hours=%d", strings.TrimRight(base, "/"), hours)
	key := url

	upstreamSummaryCache.Lock()
	if upstreamSummaryCache.entry.key == key && time.Since(upstreamSummaryCache.entry.at) < upstreamSummaryTTL {
		cached := upstreamSummaryCache.entry.result
		upstreamSummaryCache.Unlock()
		return cached, len(cached.Models) > 0
	}
	upstreamSummaryCache.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), perf_metrics_setting.GetUpstreamTimeout())
	defer cancel()

	result, err := requestUpstreamSummary(ctx, url)
	if err != nil || len(result.Models) == 0 {
		if err != nil {
			common.SysLog(fmt.Sprintf("perf metrics upstream fetch failed, keeping the previous mirror: %s", err.Error()))
		}
		upstreamSummaryCache.Lock()
		stale := upstreamSummaryCache.entry
		upstreamSummaryCache.Unlock()
		if stale.key == key && len(stale.result.Models) > 0 {
			return stale.result, true
		}
		return SummaryAllResult{}, false
	}

	upstreamSummaryCache.Lock()
	upstreamSummaryCache.entry = upstreamCacheEntry{key: key, at: time.Now(), result: result}
	upstreamSummaryCache.Unlock()
	return result, true
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
