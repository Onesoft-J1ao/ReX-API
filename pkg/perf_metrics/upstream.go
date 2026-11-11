package perfmetrics

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
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

type upstreamDetailCacheEntry struct {
	at     time.Time
	result QueryResult
}

type upstreamCacheKey struct {
	source string
	model  string
	hours  int
}

// upstreamSummaryCache holds the last successful mirror per requested window.
// A window nobody has asked for has no entry, and the request that asks for it
// reports no mirror rather than a different window's numbers.
var upstreamSummaryCache struct {
	sync.Mutex
	entries map[upstreamCacheKey]upstreamCacheEntry
}

var upstreamDetailCache struct {
	sync.Mutex
	entries map[upstreamCacheKey]upstreamDetailCacheEntry
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

func fetchUpstreamDetailFor(source, model string, hours int) (QueryResult, bool) {
	if !upstreamConfiguredFor(source) || strings.TrimSpace(model) == "" {
		return QueryResult{}, false
	}
	upstreamDetailCache.Lock()
	defer upstreamDetailCache.Unlock()
	entry, ok := upstreamDetailCache.entries[upstreamCacheKey{source: source, model: model, hours: hours}]
	if !ok || entry.result.ModelName != model {
		return QueryResult{}, false
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

// UpstreamCoversModelInGroup reports whether the upstream that serves this local
// group already mirrors the model.
//
// The old check asked the furry mirror for every group. A model present in
// furry but absent from the upstream actually behind the group (SheAPI,
// McApple) was treated as covered and therefore never probed, so those cards
// stayed blank forever. Health has to come from the upstream that serves the
// group, not from whichever mirror happens to be largest.
func UpstreamCoversModelInGroup(modelName string, groups []string) bool {
	if strings.TrimSpace(modelName) == "" {
		return false
	}
	for _, group := range groups {
		source := UpstreamSourceForGroup(group)
		if source == "" {
			// No mirror backs this group, so nothing upstream covers the model:
			// our own probe is the only possible source of a reading.
			continue
		}
		if upstreamSourceHasModel(source, modelName) {
			return true
		}
	}
	return false
}

func upstreamSourceHasModel(source, modelName string) bool {
	mirrored, ok := fetchUpstreamSummaryFor(source, 24)
	if !ok {
		return false
	}
	for _, model := range mirrored.Models {
		if model.ModelName == modelName {
			return true
		}
	}
	return false
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
	go upstreamDetailRefreshLoop()
	ticker := time.NewTicker(upstreamRefreshInterval)
	defer ticker.Stop()
	for range ticker.C {
		refreshAllUpstreamSummaries(hours)
	}
}

// refreshAllUpstreamSummaries reports whether at least one window was stored, so
// the loop knows whether the mirror is still cold.
const upstreamDetailRefreshInterval = 15 * time.Minute

func upstreamDetailRefreshLoop() {
	refreshAllUpstreamDetails()
	ticker := time.NewTicker(upstreamDetailRefreshInterval)
	defer ticker.Stop()
	for range ticker.C {
		refreshAllUpstreamDetails()
	}
}

func refreshAllUpstreamDetails() {
	groups := ratioGroupsForHealthMirror()
	groupURLs, err := model.GetEnabledGroupBaseURLs(groups)
	if err != nil {
		common.SysLog(fmt.Sprintf("perf metrics upstream detail mapping failed: %s", err.Error()))
		return
	}
	modelsBySource := make(map[string]map[string]struct{})
	disabledModel, disabledErr := model.GetDisabledModelMatcher()
	if disabledErr != nil {
		common.SysLog(fmt.Sprintf("perf metrics disabled-model lookup failed: %s", disabledErr.Error()))
		disabledModel = nil
	}
	for group, bases := range groupURLs {
		for _, base := range bases {
			source := upstreamSourceForBaseURL(base)
			upstreamGroup := upstreamHealthGroup(source, group)
			if source == "" || upstreamGroup == "" {
				continue
			}
			if modelsBySource[source] == nil {
				modelsBySource[source] = make(map[string]struct{})
			}
			for _, modelName := range model.GetGroupEnabledModels(group) {
				if disabledModel != nil && disabledModel(modelName) {
					continue
				}
				modelsBySource[source][modelName] = struct{}{}
			}
		}
	}
	jobs := make([]struct {
		source string
		model  string
	}, 0)
	for source, modelSet := range modelsBySource {
		summary, ok := fetchUpstreamSummaryFor(source, 24)
		if !ok {
			continue
		}
		models := make([]string, 0, len(modelSet))
		for _, upstreamModel := range summary.Models {
			if _, served := modelSet[upstreamModel.ModelName]; served {
				models = append(models, upstreamModel.ModelName)
			}
		}
		sort.Strings(models)
		for _, modelName := range models {
			jobs = append(jobs, struct {
				source string
				model  string
			}{source: source, model: modelName})
		}
	}
	semaphore := make(chan struct{}, 4)
	var waitGroup sync.WaitGroup
	// Track per-group coverage so a group that silently mirrors nothing shows
	// up in the log. Adding a channel without an entry in
	// upstream_health_groups is the usual cause, and the symptom is a card
	// with no strip rather than an error.
	coveredGroups := make(map[string]struct{}, len(groupURLs))
	var coveredLock sync.Mutex
	groupsByModel := make(map[string][]string)
	for group, bases := range groupURLs {
		for _, base := range bases {
			source := upstreamSourceForBaseURL(base)
			if source == "" {
				continue
			}
			for _, modelName := range model.GetGroupEnabledModels(group) {
				groupsByModel[source+"\x00"+modelName] = append(groupsByModel[source+"\x00"+modelName], group)
			}
		}
	}
	for _, job := range jobs {
		job := job
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			if refreshUpstreamDetail(job.source, job.model, 24) {
				coveredLock.Lock()
				for _, g := range groupsByModel[job.source+"\x00"+job.model] {
					coveredGroups[g] = struct{}{}
				}
				coveredLock.Unlock()
			}
		}()
	}
	waitGroup.Wait()

	uncovered := make([]string, 0)
	for group, bases := range groupURLs {
		if _, ok := coveredGroups[group]; ok {
			continue
		}
		// Only a group with something to measure can be missing a reading. A
		// group whose channels expose no enabled model has no row to mirror,
		// and one served by an unmirrored upstream (mjiutang, a self-hosted
		// relay) never had one to begin with. Listing either here sent the
		// reader looking for a mapping that cannot exist.
		if len(model.GetGroupEnabledModels(group)) == 0 {
			continue
		}
		mirrored := false
		for _, base := range bases {
			if upstreamSourceForBaseURL(base) != "" {
				mirrored = true
				break
			}
		}
		if mirrored {
			uncovered = append(uncovered, group)
		}
	}
	if len(uncovered) > 0 {
		sort.Strings(uncovered)
		common.SysLog("perf metrics: no mirrored health for groups " + strings.Join(uncovered, ", ") +
			" — check perf_metrics_setting.upstream_health_groups")
	}
}

func ratioGroupsForHealthMirror() []string {
	groups := make([]string, 0, len(ratio_setting.GetGroupRatioCopy()))
	for group := range ratio_setting.GetGroupRatioCopy() {
		groups = append(groups, group)
	}
	return groups
}

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

func refreshUpstreamDetail(source, model string, hours int) bool {
	if !upstreamConfiguredFor(source) || strings.TrimSpace(model) == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), perf_metrics_setting.GetUpstreamTimeout())
	defer cancel()
	result, err := requestUpstreamDetail(ctx, upstreamDetailURL(source, model, hours))
	if err != nil || result.ModelName == "" || len(result.Groups) == 0 {
		return false
	}
	upstreamDetailCache.Lock()
	if upstreamDetailCache.entries == nil {
		upstreamDetailCache.entries = map[upstreamCacheKey]upstreamDetailCacheEntry{}
	}
	upstreamDetailCache.entries[upstreamCacheKey{source: source, model: model, hours: hours}] = upstreamDetailCacheEntry{at: time.Now(), result: result}
	upstreamDetailCache.Unlock()
	return true
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

func upstreamDetailURL(source, model string, hours int) string {
	base := strings.TrimRight(strings.TrimSpace(upstreamSummaryUrlBase(source)), "/")
	return fmt.Sprintf("%s?model=%s&hours=%d", strings.Replace(base, "/summary", "", 1), url.QueryEscape(model), hours)
}

func requestUpstreamDetail(ctx context.Context, url string) (QueryResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return QueryResult{}, err
	}
	resp, err := getUpstreamClient(perf_metrics_setting.GetUpstreamTimeout()).Do(req)
	if err != nil {
		return QueryResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return QueryResult{}, fmt.Errorf("upstream detail returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, upstreamMaxBytes+1))
	if err != nil {
		return QueryResult{}, err
	}
	var envelope struct {
		Success bool        `json:"success"`
		Data    QueryResult `json:"data"`
	}
	if err := common.Unmarshal(body, &envelope); err != nil {
		return QueryResult{}, err
	}
	if !envelope.Success {
		return QueryResult{}, errors.New("upstream detail reported failure")
	}
	return envelope.Data, nil
}
