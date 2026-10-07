package service

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const (
	BangumiSourceIdle            = "idle"
	BangumiSourceWorking         = "working"
	BangumiSourceReady           = "ready"
	BangumiSourceEmpty           = "empty"
	BangumiSourceFailed          = "failed"
	BangumiSourceCaptchaRequired = "captcha_required"
	BangumiSourceRateLimited     = "rate_limited"
	BangumiSourceBrowserRequired = "browser_required"
	BangumiSourceInvalidConfig   = "invalid_config"

	bangumiSourceBodyLimit    = 2 << 20
	bangumiSourceResultLimit  = 128
	bangumiSourceEpisodeLimit = 512
	bangumiSourceCacheLimit   = 128
	bangumiSourceRecordLimit  = 8192
	bangumiSourceTTL          = 10 * time.Minute
	// A clearance cookie is short-lived at the upstream anyway, and it is only
	// useful until the viewer's own challenge window closes.
	bangumiSourceClearanceLimit = 2048
	bangumiSourceClearanceTTL   = 15 * time.Minute
)

//go:embed bangumi_sources_catalog.json
var bangumiSourcesCatalog []byte

// BangumiMediaSource is deliberately metadata-only. Rules and preference cookies
// belong to the server-side catalog, not to the visitor's source picker. The
// website URL is the one exception: when a source answers with a challenge, the
// viewer has to solve it on the source's own site, and that site is not
// otherwise knowable from the picker.
type BangumiMediaSource struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Icon       string `json:"icon"`
	Tier       int    `json:"tier"`
	Enabled    bool   `json:"enabled"`
	WebsiteURL string `json:"websiteUrl,omitempty"`
}

type BangumiSourceStatus struct {
	SourceID    string     `json:"sourceId"`
	State       string     `json:"state"`
	CaptchaKind string     `json:"captchaKind,omitempty"`
	RetryAt     *time.Time `json:"retryAt,omitempty"`
	Message     string     `json:"message,omitempty"`
}

type BangumiSourceSubject struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type BangumiSourceEpisode struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	EpisodeNumber string `json:"episodeNumber"`
}

type BangumiSourceChannel struct {
	ID       string                 `json:"id"`
	Name     string                 `json:"name"`
	Label    string                 `json:"label"`
	Episodes []BangumiSourceEpisode `json:"episodes"`
}

type BangumiSourceSearchResult struct {
	BangumiSourceStatus
	Subjects []BangumiSourceSubject `json:"subjects"`
}

type BangumiSourceBrowseResult struct {
	BangumiSourceStatus
	Channels []BangumiSourceChannel `json:"channels"`
}

type BangumiSourceResolveResult struct {
	BangumiSourceStatus
	VideoURL   string      `json:"-"`
	TorrentURI string      `json:"-"`
	Headers    http.Header `json:"-"`
	Format     string      `json:"format"`
}

// BangumiSourceError carries the same safe, typed state as an API result.
// The underlying cause is accessible to callers but never included in Error().
type BangumiSourceError struct {
	BangumiSourceStatus
	cause error
}

func (e *BangumiSourceError) Error() string { return e.Message }
func (e *BangumiSourceError) Unwrap() error { return e.cause }

// BangumiSourceTorrentRecord is an internal handoff to the BT adapter. None of
// its upstream locations can be serialized into a visitor response.
type BangumiSourceTorrentRecord struct {
	OriginURL  string `json:"-"`
	TorrentURL string `json:"-"`
	Magnet     string `json:"-"`
}

type bangumiSourceDefinition struct {
	MediaSourceID string              `json:"mediaSourceId"`
	Name          string              `json:"name"`
	FactoryID     string              `json:"factoryId"`
	Kind          string              `json:"kind"`
	IconURL       string              `json:"iconUrl"`
	Tier          int                 `json:"tier"`
	ChannelTiers  map[string]int      `json:"channelTiers"`
	Enabled       bool                `json:"enabled"`
	WebsiteURL    string              `json:"websiteUrl"`
	SearchConfig  bangumiSourceConfig `json:"searchConfig"`
}

type bangumiSource struct {
	definition  bangumiSourceDefinition
	rules       *bangumiSourceRules
	configError error
	origin      *url.URL
}

type bangumiSourceRecord struct {
	sourceID string
	kind     string
	name     string
	url      string
	torrent  BangumiSourceTorrentRecord
	expires  time.Time
}

type bangumiSourceCacheEntry struct {
	value     any
	recordIDs []string
	expires   time.Time
}

// BangumiSourceManager owns bounded, configuration-scoped records. All network
// requests go through the injected client; the host supplies its SSRF, proxy and
// redirect policy. No browser, JavaScript runtime, database or global cookie jar
// is involved here.
type BangumiSourceManager struct {
	client      *http.Client
	version     string
	ordered     []*bangumiSource
	sources     map[string]*bangumiSource
	fetchSlots  chan struct{}
	mu          sync.Mutex
	nextRequest map[string]time.Time
	records     map[string]bangumiSourceRecord
	cache       map[string]bangumiSourceCacheEntry
	clearances  map[string]bangumiSourceClearance
}

// bangumiSourceClearance is a cookie a viewer obtained by solving a source's
// challenge in their own browser and handed back to the server.
type bangumiSourceClearance struct {
	cookie  string
	expires time.Time
}

func NewBangumiSourceManager(client *http.Client, configPath string) (*BangumiSourceManager, error) {
	if client == nil {
		return nil, errors.New("bangumi source manager requires an injected HTTP client")
	}
	data := bangumiSourcesCatalog
	if configPath != "" {
		file, err := os.Open(configPath)
		if err != nil {
			return nil, errors.New("cannot open bangumi source catalog")
		}
		defer file.Close()
		data, err = io.ReadAll(io.LimitReader(file, bangumiSourceBodyLimit+1))
		if err != nil || len(data) > bangumiSourceBodyLimit {
			return nil, errors.New("cannot read bangumi source catalog or catalog exceeds 2 MiB")
		}
	}
	var catalog struct {
		Version int                       `json:"version"`
		Sources []bangumiSourceDefinition `json:"sources"`
	}
	if err := common.Unmarshal(data, &catalog); err != nil {
		return nil, errors.New("invalid bangumi source catalog JSON")
	}
	if catalog.Version != 1 || len(catalog.Sources) == 0 || len(catalog.Sources) > 128 {
		return nil, errors.New("unsupported or empty bangumi source catalog")
	}
	digest := sha256.Sum256(data)
	m := &BangumiSourceManager{
		client: client, version: fmt.Sprintf("%x", digest[:16]),
		sources: make(map[string]*bangumiSource), fetchSlots: make(chan struct{}, 2),
		nextRequest: make(map[string]time.Time), records: make(map[string]bangumiSourceRecord),
		cache: make(map[string]bangumiSourceCacheEntry), clearances: make(map[string]bangumiSourceClearance),
	}
	for _, definition := range catalog.Sources {
		id := definition.MediaSourceID
		if id == "" || len(id) > 128 || definition.Name == "" || len(definition.Name) > 512 {
			return nil, errors.New("source catalog has missing or oversized metadata")
		}
		if _, exists := m.sources[id]; exists {
			return nil, errors.New("duplicate mediaSourceId in source catalog")
		}
		source := &bangumiSource{definition: definition}
		source.rules, source.configError = compileBangumiSourceRules(definition)
		if source.configError == nil && definition.FactoryID != "mikan-mikanime-tv" {
			source.origin, source.configError = bangumiHTTPURL(nil, strings.ReplaceAll(definition.SearchConfig.SearchURL, "{keyword}", "test"))
		}
		m.sources[id] = source
		m.ordered = append(m.ordered, source)
	}
	return m, nil
}

func (m *BangumiSourceManager) Sources() []BangumiMediaSource {
	result := make([]BangumiMediaSource, 0, len(m.ordered))
	for _, source := range m.ordered {
		d := source.definition
		// The catalog names a website for only a couple of sources; the rest are
		// reachable at their search URL's origin, which is the page a viewer opens
		// to clear a challenge.
		website := d.WebsiteURL
		if _, err := bangumiHTTPURL(nil, website); err != nil {
			// Absent or non-HTTP: dropped rather than handed to the picker as a
			// link the browser should not follow.
			website = ""
		}
		if website == "" && source.origin != nil {
			website = source.origin.Scheme + "://" + source.origin.Host
		}
		result = append(result, BangumiMediaSource{ID: d.MediaSourceID, Name: d.Name, Kind: d.Kind, Icon: d.IconURL, Tier: d.Tier, Enabled: d.Enabled, WebsiteURL: website})
	}
	return result
}

func bangumiSourceFailure(sourceID, state, message string, cause error) *BangumiSourceError {
	return &BangumiSourceError{BangumiSourceStatus: BangumiSourceStatus{SourceID: sourceID, State: state, Message: message}, cause: cause}
}

// SetClearance records a cookie the viewer earned by solving a source's
// challenge in their own browser.
//
// The source pages are fetched by this server, so a clearance the viewer's
// browser obtained does not travel with them; it has to be handed over
// explicitly. It is kept for this source alone and only until it goes stale. An
// empty value clears it.
func (m *BangumiSourceManager) SetClearance(sourceID, cookie string) error {
	if _, exists := m.sources[sourceID]; !exists {
		return bangumiSourceFailure(sourceID, BangumiSourceInvalidConfig, "Unknown media source", nil)
	}
	cookie = strings.TrimSpace(cookie)
	if len(cookie) > bangumiSourceClearanceLimit || strings.ContainsAny(cookie, "\r\n\x00") {
		return bangumiSourceFailure(sourceID, BangumiSourceInvalidConfig, "Invalid clearance cookie", nil)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if cookie == "" {
		delete(m.clearances, sourceID)
		return nil
	}
	m.clearances[sourceID] = bangumiSourceClearance{cookie: cookie, expires: time.Now().Add(bangumiSourceClearanceTTL)}
	return nil
}

// sourceCookie is the Cookie header a source's own host is sent: the imported
// quality preference plus any clearance the viewer supplied by hand. Only that
// host ever sees it -- the callers gate on the host matching the source origin
// -- so a clearance cannot leak to a media CDN on another host.
func (m *BangumiSourceManager) sourceCookie(source *bangumiSource) string {
	parts := make([]string, 0, 2)
	if configured := source.rules.config.MatchVideo.Cookies; configured != "" {
		parts = append(parts, configured)
	}
	m.mu.Lock()
	clearance, ok := m.clearances[source.definition.MediaSourceID]
	m.mu.Unlock()
	if ok && time.Now().Before(clearance.expires) {
		parts = append(parts, clearance.cookie)
	}
	return strings.Join(parts, "; ")
}

func (m *BangumiSourceManager) source(ctx context.Context, id string) (*bangumiSource, error) {
	if err := ctx.Err(); err != nil {
		return nil, bangumiSourceFailure(id, BangumiSourceFailed, "Source request canceled or expired", err)
	}
	source, exists := m.sources[id]
	if !exists {
		return nil, bangumiSourceFailure(id, BangumiSourceInvalidConfig, "Unknown media source", nil)
	}
	if !source.definition.Enabled {
		return nil, bangumiSourceFailure(id, BangumiSourceInvalidConfig, "Media source is disabled", nil)
	}
	if source.configError != nil {
		return nil, bangumiSourceFailure(id, BangumiSourceInvalidConfig, source.configError.Error(), nil)
	}
	if source.definition.FactoryID == "mikan-mikanime-tv" {
		return nil, bangumiSourceFailure(id, BangumiSourceInvalidConfig, "builtin_adapter_required", nil)
	}
	return source, nil
}

func (m *BangumiSourceManager) Search(ctx context.Context, sourceID, keyword string) (BangumiSourceSearchResult, error) {
	result := BangumiSourceSearchResult{BangumiSourceStatus: BangumiSourceStatus{SourceID: sourceID, State: BangumiSourceWorking}, Subjects: []BangumiSourceSubject{}}
	source, err := m.source(ctx, sourceID)
	if err != nil {
		result.BangumiSourceStatus = err.(*BangumiSourceError).BangumiSourceStatus
		return result, err
	}
	keyword = strings.TrimSpace(keyword)
	if keyword == "" || len(keyword) > 512 {
		err := bangumiSourceFailure(sourceID, BangumiSourceFailed, "Search keyword must contain 1–512 bytes", nil)
		result.BangumiSourceStatus = err.BangumiSourceStatus
		return result, err
	}
	key := m.cacheKey(sourceID, "search", keyword)
	if cached, ok := m.cached(key); ok {
		return cached.(BangumiSourceSearchResult), nil
	}
	searchURL, err := source.rules.searchURL(keyword)
	if err != nil {
		return result, m.resultError(&result.BangumiSourceStatus, sourceID, err)
	}
	page, err := m.fetch(ctx, source, searchURL, "", http.MethodGet, false)
	if err != nil {
		return result, m.resultError(&result.BangumiSourceStatus, sourceID, err)
	}
	candidates, parseErr := source.rules.search(ctx, page)
	if len(candidates) == 0 || parseErr != nil || page.status < 200 || page.status >= 300 {
		err = bangumiPageFailure(sourceID, page, parseErr)
		if err != nil {
			return result, m.resultError(&result.BangumiSourceStatus, sourceID, err)
		}
		result.State = BangumiSourceEmpty
	} else {
		result.State = BangumiSourceReady
	}
	expires := time.Now().Add(source.rules.cacheTTL())
	recordIDs := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		id := m.storeRecord(sourceID, "subject", candidate.url, bangumiSourceRecord{sourceID: sourceID, kind: "subject", name: candidate.name, url: candidate.url, torrent: candidate.torrent, expires: expires})
		result.Subjects = append(result.Subjects, BangumiSourceSubject{ID: id, Name: candidate.name})
		recordIDs = append(recordIDs, id)
	}
	m.putCache(key, result, recordIDs, expires, source.rules.config.SearchCacheTTL != 0)
	return cloneBangumiCacheValue(result).(BangumiSourceSearchResult), nil
}

func (m *BangumiSourceManager) Browse(ctx context.Context, sourceID, subjectID string) (BangumiSourceBrowseResult, error) {
	result := BangumiSourceBrowseResult{BangumiSourceStatus: BangumiSourceStatus{SourceID: sourceID, State: BangumiSourceWorking}, Channels: []BangumiSourceChannel{}}
	source, err := m.source(ctx, sourceID)
	if err != nil {
		result.BangumiSourceStatus = err.(*BangumiSourceError).BangumiSourceStatus
		return result, err
	}
	record, err := m.record(sourceID, subjectID, "subject")
	if err != nil {
		return result, m.resultError(&result.BangumiSourceStatus, sourceID, err)
	}
	key := m.cacheKey(sourceID, "browse", subjectID)
	if cached, ok := m.cached(key); ok {
		return cached.(BangumiSourceBrowseResult), nil
	}
	var groups []bangumiParsedChannel
	if source.definition.Kind == "bt" {
		groups = []bangumiParsedChannel{{name: "BT", label: "BT", episodes: []bangumiParsedEpisode{{name: record.name, number: record.name, url: record.url, torrent: record.torrent}}}}
	} else {
		page, fetchErr := m.fetch(ctx, source, record.url, "", http.MethodGet, false)
		if fetchErr != nil {
			return result, m.resultError(&result.BangumiSourceStatus, sourceID, fetchErr)
		}
		var parseErr error
		groups, parseErr = source.rules.browse(ctx, page)
		episodeCount := 0
		for _, group := range groups {
			episodeCount += len(group.episodes)
		}
		if episodeCount == 0 || parseErr != nil || page.status < 200 || page.status >= 300 {
			if err := bangumiPageFailure(sourceID, page, parseErr); err != nil {
				return result, m.resultError(&result.BangumiSourceStatus, sourceID, err)
			}
		}
	}
	expires := time.Now().Add(bangumiSourceTTL)
	recordIDs := []string{subjectID}
	for _, group := range groups {
		channelID := m.opaqueID(sourceID, "channel", fmt.Sprintf("%s\x00%d", subjectID, group.ordinal))
		channel := BangumiSourceChannel{ID: channelID, Name: group.name, Label: group.label, Episodes: []BangumiSourceEpisode{}}
		for i, episode := range group.episodes {
			id := m.storeRecord(sourceID, "episode", fmt.Sprintf("%s\x00%d\x00%s", channelID, i, episode.url), bangumiSourceRecord{sourceID: sourceID, kind: "episode", name: episode.name, url: episode.url, torrent: episode.torrent, expires: expires})
			channel.Episodes = append(channel.Episodes, BangumiSourceEpisode{ID: id, Name: episode.name, EpisodeNumber: episode.number})
			recordIDs = append(recordIDs, id)
		}
		result.Channels = append(result.Channels, channel)
	}
	result.State = BangumiSourceEmpty
	for _, channel := range result.Channels {
		if len(channel.Episodes) > 0 {
			result.State = BangumiSourceReady
			break
		}
	}
	m.putCache(key, result, recordIDs, bangumiEarlierExpiry(expires, record.expires), true)
	return cloneBangumiCacheValue(result).(BangumiSourceBrowseResult), nil
}

func (m *BangumiSourceManager) Resolve(ctx context.Context, sourceID, episodeID string) (BangumiSourceResolveResult, error) {
	result := BangumiSourceResolveResult{BangumiSourceStatus: BangumiSourceStatus{SourceID: sourceID, State: BangumiSourceWorking}, Format: "unknown"}
	source, err := m.source(ctx, sourceID)
	if err != nil {
		result.BangumiSourceStatus = err.(*BangumiSourceError).BangumiSourceStatus
		return result, err
	}
	record, err := m.record(sourceID, episodeID, "episode")
	if err != nil {
		return result, m.resultError(&result.BangumiSourceStatus, sourceID, err)
	}
	if source.definition.Kind == "bt" {
		result.TorrentURI = record.torrent.TorrentURL
		if result.TorrentURI == "" {
			result.TorrentURI = record.torrent.Magnet
		}
		if result.TorrentURI == "" {
			err := bangumiSourceFailure(sourceID, BangumiSourceInvalidConfig, "BT release has no supported torrent or magnet location", nil)
			result.BangumiSourceStatus = err.BangumiSourceStatus
			return result, err
		}
		result.State, result.Format = BangumiSourceReady, "torrent"
		result.Headers = make(http.Header)
		result.Message = "BT location resolved; playback requires the host's rqbit adapter"
		return result, nil
	}
	key := m.cacheKey(sourceID, "resolve", episodeID)
	if cached, ok := m.cached(key); ok {
		return cached.(BangumiSourceResolveResult), nil
	}
	result, err = m.resolveStatic(ctx, source, record.url)
	if err != nil {
		return result, err
	}
	// Signed player URLs can expire sooner than the catalog records.
	m.putCache(key, result, []string{episodeID}, bangumiEarlierExpiry(time.Now().Add(time.Minute), record.expires), true)
	return cloneBangumiCacheValue(result).(BangumiSourceResolveResult), nil
}

func (m *BangumiSourceManager) TorrentRecord(sourceID, itemID string) (BangumiSourceTorrentRecord, error) {
	source, exists := m.sources[sourceID]
	if !exists || source.definition.Kind != "bt" {
		return BangumiSourceTorrentRecord{}, bangumiSourceFailure(sourceID, BangumiSourceInvalidConfig, "Not a BT media source", nil)
	}
	record, err := m.record(sourceID, itemID, "")
	if err != nil {
		return BangumiSourceTorrentRecord{}, err
	}
	if record.torrent.Magnet == "" && record.torrent.TorrentURL == "" {
		return BangumiSourceTorrentRecord{}, bangumiSourceFailure(sourceID, BangumiSourceBrowserRequired, "Release has no supported torrent or magnet location", nil)
	}
	return record.torrent, nil
}

func (m *BangumiSourceManager) resultError(status *BangumiSourceStatus, sourceID string, err error) error {
	var typed *BangumiSourceError
	if !errors.As(err, &typed) {
		message := "Source parsing failed"
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			message = "Source request canceled or expired"
		}
		typed = bangumiSourceFailure(sourceID, BangumiSourceFailed, message, err)
	}
	*status = typed.BangumiSourceStatus
	return typed
}

func (m *BangumiSourceManager) opaqueID(sourceID, kind, value string) string {
	digest := sha256.Sum256([]byte(m.version + "\x00" + sourceID + "\x00" + kind + "\x00" + value))
	return fmt.Sprintf("%x", digest[:24])
}
func (m *BangumiSourceManager) cacheKey(sourceID, operation, query string) string {
	return m.opaqueID(sourceID, operation, query)
}

func (m *BangumiSourceManager) storeRecord(sourceID, kind, value string, record bangumiSourceRecord) string {
	id := m.opaqueID(sourceID, kind, value)
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for key, entry := range m.records {
		if !entry.expires.After(now) {
			delete(m.records, key)
		}
	}
	if _, exists := m.records[id]; !exists && len(m.records) >= bangumiSourceRecordLimit {
		oldestID := ""
		var oldest time.Time
		for key, entry := range m.records {
			if oldestID == "" || entry.expires.Before(oldest) {
				oldestID, oldest = key, entry.expires
			}
		}
		delete(m.records, oldestID)
	}
	m.records[id] = record
	return id
}

func (m *BangumiSourceManager) record(sourceID, id, kind string) (bangumiSourceRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	record, exists := m.records[id]
	if !exists || record.sourceID != sourceID || (kind != "" && record.kind != kind) || !record.expires.After(time.Now()) {
		return bangumiSourceRecord{}, bangumiSourceFailure(sourceID, BangumiSourceFailed, "Unknown or expired source item; search or browse again", nil)
	}
	return record, nil
}

func (m *BangumiSourceManager) cached(key string) (any, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.cache[key]
	now := time.Now()
	if !ok {
		return nil, false
	}
	if !entry.expires.After(now) {
		delete(m.cache, key)
		return nil, false
	}
	for _, id := range entry.recordIDs {
		if record, ok := m.records[id]; !ok || !record.expires.After(now) {
			delete(m.cache, key)
			return nil, false
		}
	}
	return cloneBangumiCacheValue(entry.value), true
}

func (m *BangumiSourceManager) putCache(key string, value any, ids []string, expires time.Time, enabled bool) {
	if !enabled {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, entry := range m.cache {
		if !entry.expires.After(time.Now()) {
			delete(m.cache, key)
		}
	}
	if len(m.cache) >= bangumiSourceCacheLimit {
		oldestKey := ""
		var oldest time.Time
		for key, entry := range m.cache {
			if oldestKey == "" || entry.expires.Before(oldest) {
				oldestKey, oldest = key, entry.expires
			}
		}
		delete(m.cache, oldestKey)
	}
	m.cache[key] = bangumiSourceCacheEntry{value: cloneBangumiCacheValue(value), recordIDs: ids, expires: expires}
}

func bangumiEarlierExpiry(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func cloneBangumiCacheValue(value any) any {
	switch result := value.(type) {
	case BangumiSourceSearchResult:
		result.Subjects = append([]BangumiSourceSubject{}, result.Subjects...)
		return result
	case BangumiSourceBrowseResult:
		result.Channels = append([]BangumiSourceChannel{}, result.Channels...)
		for i := range result.Channels {
			result.Channels[i].Episodes = append([]BangumiSourceEpisode{}, result.Channels[i].Episodes...)
		}
		return result
	case BangumiSourceResolveResult:
		result.Headers = result.Headers.Clone()
		return result
	}
	return value
}

// Every location, including iframe and literal player URLs, must first pass this
// syntax check. Host/IP trust is the injected client's policy, not a parser rule.
func bangumiHTTPURL(base *url.URL, raw string) (*url.URL, error) {
	if len(raw) == 0 || len(raw) > 8192 || strings.ContainsAny(raw, "\r\n\x00\\") {
		return nil, errors.New("invalid upstream location")
	}
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, errors.New("invalid upstream location")
	}
	if base != nil {
		parsed = base.ResolveReference(parsed)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil {
		return nil, errors.New("upstream location must be HTTP(S) without userinfo")
	}
	parsed.Fragment = ""
	return parsed, nil
}

type bangumiSourcePage struct {
	body        []byte
	url         *url.URL
	status      int
	contentType string
}

func (m *BangumiSourceManager) requestHeaders(source *bangumiSource, target *url.URL, referer string) http.Header {
	headers := make(http.Header)
	headers.Set("User-Agent", source.rules.config.MatchVideo.AddHeadersToVideo.UserAgent)
	if referer != "" {
		headers.Set("Referer", referer)
	}
	if source.origin != nil && strings.EqualFold(target.Host, source.origin.Host) {
		if cookies := m.sourceCookie(source); cookies != "" {
			headers.Set("Cookie", cookies)
		}
	}
	return headers
}

func (m *BangumiSourceManager) fetch(ctx context.Context, source *bangumiSource, rawURL, referer, method string, rangeProbe bool) (bangumiSourcePage, error) {
	id := source.definition.MediaSourceID
	target, err := bangumiHTTPURL(nil, rawURL)
	if err != nil {
		return bangumiSourcePage{}, bangumiSourceFailure(id, BangumiSourceFailed, "Source returned an unsafe or malformed upstream location", nil)
	}
	select {
	case m.fetchSlots <- struct{}{}:
	case <-ctx.Done():
		return bangumiSourcePage{}, bangumiSourceFailure(id, BangumiSourceFailed, "Source request canceled or expired", ctx.Err())
	}
	defer func() { <-m.fetchSlots }()
	m.mu.Lock()
	// Multiple definitions at the same site (for example quality variants) share
	// the site's pacing window rather than bypassing it with different IDs.
	site := source.origin.Host
	start := time.Now()
	if next := m.nextRequest[site]; next.After(start) {
		start = next
	}
	m.nextRequest[site] = start.Add(time.Duration(source.rules.config.RequestInterval) * time.Millisecond)
	m.mu.Unlock()
	if delay := time.Until(start); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return bangumiSourcePage{}, bangumiSourceFailure(id, BangumiSourceFailed, "Source request canceled or expired", ctx.Err())
		}
	}
	requestCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, method, target.String(), nil)
	if err != nil {
		return bangumiSourcePage{}, bangumiSourceFailure(id, BangumiSourceFailed, "Cannot construct source request", nil)
	}
	request.Header = m.requestHeaders(source, target, referer)
	if method == http.MethodHead || rangeProbe {
		if configured := source.rules.config.MatchVideo.AddHeadersToVideo.Referer; configured != "" {
			request.Header.Set("Referer", configured)
		}
	}
	if rangeProbe {
		request.Header.Set("Range", "bytes=0-0")
	}
	request.Header.Set("Accept", "text/html, application/rss+xml, application/xml, */*;q=0.8")
	client := *m.client
	// A shared visitor/global jar must never contaminate source sessions.
	client.Jar = nil
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 8 {
			return errors.New("source redirect limit exceeded")
		}
		if _, err := bangumiHTTPURL(nil, req.URL.String()); err != nil {
			return err
		}
		if m.client.CheckRedirect != nil {
			if err := m.client.CheckRedirect(req, via); err != nil {
				return err
			}
		}
		req.Header.Del("Cookie")
		if strings.EqualFold(req.URL.Host, source.origin.Host) {
			if cookies := m.sourceCookie(source); cookies != "" {
				req.Header.Set("Cookie", cookies)
			}
		}
		return nil
	}
	response, err := client.Do(request)
	if err != nil {
		cause := err
		if ctx.Err() != nil {
			cause = ctx.Err()
		}
		return bangumiSourcePage{}, bangumiSourceFailure(id, BangumiSourceFailed, "Upstream source request failed or was blocked by network policy", cause)
	}
	defer response.Body.Close()
	finalURL, err := bangumiHTTPURL(nil, response.Request.URL.String())
	if err != nil {
		return bangumiSourcePage{}, bangumiSourceFailure(id, BangumiSourceFailed, "Invalid upstream redirect location", nil)
	}
	page := bangumiSourcePage{url: finalURL, status: response.StatusCode, contentType: response.Header.Get("Content-Type")}
	if response.StatusCode == http.StatusTooManyRequests {
		retry := time.Now().Add(time.Minute)
		if seconds, err := strconv.ParseInt(response.Header.Get("Retry-After"), 10, 64); err == nil && seconds >= 0 {
			retry = time.Now().Add(time.Duration(min(seconds, 86400)) * time.Second)
		} else if parsed, err := http.ParseTime(response.Header.Get("Retry-After")); err == nil {
			retry = parsed
			now := time.Now()
			if retry.Before(now) {
				retry = now
			}
			if limit := now.Add(24 * time.Hour); retry.After(limit) {
				retry = limit
			}
		}
		limited := bangumiSourceFailure(id, BangumiSourceRateLimited, "Upstream source is rate limited", nil)
		limited.RetryAt = &retry
		return page, limited
	}
	if rangeProbe {
		// A server may ignore Range. Read at most a signature-sized prefix and
		// cancel/close immediately, rather than downloading the whole media.
		page.body, err = io.ReadAll(io.LimitReader(response.Body, 512))
		cancel()
		if err != nil {
			return page, bangumiSourceFailure(id, BangumiSourceFailed, "Cannot read media probe response", err)
		}
		return page, nil
	}
	if method != http.MethodHead {
		page.body, err = io.ReadAll(io.LimitReader(response.Body, bangumiSourceBodyLimit+1))
		if err != nil {
			return page, bangumiSourceFailure(id, BangumiSourceFailed, "Cannot read upstream response", err)
		}
		if len(page.body) > bangumiSourceBodyLimit {
			return page, bangumiSourceFailure(id, BangumiSourceFailed, "Upstream response exceeds 2 MiB", nil)
		}
	}
	return page, nil
}

func bangumiPageFailure(sourceID string, page bangumiSourcePage, parseErr error) error {
	if parseErr != nil {
		var typed *BangumiSourceError
		if errors.As(parseErr, &typed) && typed.State == BangumiSourceInvalidConfig {
			return typed
		}
		if errors.Is(parseErr, context.Canceled) || errors.Is(parseErr, context.DeadlineExceeded) {
			return bangumiSourceFailure(sourceID, BangumiSourceFailed, "Source request canceled or expired", parseErr)
		}
	}
	if kind := bangumiCaptchaKind(page); kind != "" {
		err := bangumiSourceFailure(sourceID, BangumiSourceCaptchaRequired, "Upstream requires server-side source verification; visitor verification does not update server clearance", nil)
		err.CaptchaKind = kind
		return err
	}
	if page.status < 200 || page.status >= 300 {
		return bangumiSourceFailure(sourceID, BangumiSourceFailed, fmt.Sprintf("Upstream returned HTTP %d", page.status), nil)
	}
	if parseErr != nil {
		return bangumiSourceFailure(sourceID, BangumiSourceFailed, parseErr.Error(), nil)
	}
	return nil
}

// Classification runs only after the normal parser cannot produce a result.
func bangumiCaptchaKind(page bangumiSourcePage) string {
	body := strings.ToLower(string(page.body))
	location := strings.ToLower(page.url.String())
	if strings.Contains(body, "cf-turnstile") || strings.Contains(body, "turnstile.render") || strings.Contains(body, "challenges.cloudflare.com/turnstile") {
		return "cloudflare_turnstile"
	}
	if strings.Contains(body, "puzzle-container") {
		return "slider"
	}
	if strings.Contains(body, "需要输入验证码") {
		return "unknown"
	}
	for _, marker := range []string{"__cf_chl_", "/cdn-cgi/challenge-platform/h/", "/cdn-cgi/challenge-platform/orchestrate/"} {
		if strings.Contains(body, marker) || strings.Contains(location, marker) {
			return "cloudflare"
		}
	}
	for _, marker := range []string{"challenge-error-text", "enable javascript and cookies to continue", "cf-browser-verification", "id=\"cf-challenge\"", "id='cf-challenge'", "window._cf_chl_opt"} {
		if strings.Contains(body, marker) {
			return "cloudflare"
		}
	}
	if (strings.Contains(body, "<title>just a moment") || strings.Contains(body, "checking your browser before accessing")) && (strings.Contains(body, "challenge-platform") || strings.Contains(body, "cf-ray") || strings.Contains(body, "window.__cf$cv$params")) {
		return "cloudflare"
	}
	if strings.Contains(body, "/.safeline/static/favicon.png") || strings.Contains(body, "id=\"slg-box\"") || strings.Contains(body, "id=\"slg-title\"") || (strings.Contains(body, "window.product_data") && strings.Contains(body, "slg-bg")) {
		return "unknown"
	}
	input := strings.Contains(body, "name=\"verify\"") || strings.Contains(body, "name='verify'") || strings.Contains(body, "请输入验证码") || strings.Contains(body, "請輸入驗證碼")
	image := strings.Contains(body, "ds-verify-img") || strings.Contains(body, "/verify/index.html")
	submit := strings.Contains(body, "verify-submit") || strings.Contains(body, "data-type=\"search\"") || strings.Contains(body, "data-type='search'") || strings.Contains(body, "提交验证") || strings.Contains(body, "提交驗證")
	if input && image && submit {
		return "image"
	}
	return ""
}
