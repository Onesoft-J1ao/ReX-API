package controller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// Bangumi (bgm.tv) is the anime metadata source behind the /bangumi page.
//
// The upstream is a third party with its own quota, so the console proxies it
// rather than letting every browser call it directly: one place to cache, one
// place to rate limit, and one User-Agent instead of a fleet. Bangumi asks
// callers to identify themselves and rejects generic browser agents.
//
// Only the read-only endpoints the page needs are exposed. The collection
// endpoints need a Bangumi account token, which this instance has no business
// holding on a visitor's behalf.
//
// Watching an anime needs two further upstreams, which also go through here:
//
//   - dandanplay maps a bgm.tv subject id onto an episode list and serves the
//     bullet comments. It authenticates by signature rather than by token.
//   - Mikan Project publishes torrents, not video files, so playback happens in
//     the visitor's browser rather than through this instance. Mikan's feed is
//     parsed into JSON and no video bytes are ever proxied.
//
// Release selection, title parsing and the subject-id mapping are ported from
// Animeko (datasource/api, datasource/bt/mikan, domain/media/selector) so this
// page agrees with it on what "the best release" means. Each rule names its
// reference below.

const (
	bangumiAPIBase   = "https://api.bgm.tv"
	bangumiImageBase = "https://lain.bgm.tv"
	// Sent to third-party anime sites on every upstream request, so it presents a
	// browser rather than this instance: several of them reject a non-browser
	// agent outright, and none of them need to learn who is calling. Mikan in
	// particular answers a bot agent with an empty result set, which then reads
	// as "no release matches" instead of a blocked request.
	bangumiUserAgent  = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"
	bangumiMaxBody    = 4 << 20
	bangumiSearchType = 2 // anime, excluding books and music

	// The ranking is one page of the same subject list, so it reuses the search
	// type and a fixed page size rather than accepting one from the caller: the
	// page size keys the cache, and an unbounded one would let a single request
	// pull the whole catalogue through this instance.
	bangumiRankingPageSize = 24
	bangumiRankingMaxPage  = 100

	// A search or release keyword keys a cache entry and is forwarded upstream,
	// so it is bounded at the point it enters. The media-source search uses the
	// same bound.
	maxBangumiKeywordBytes = 512

	dandanplayAPIBase = "https://api.dandanplay.net"
	mikanAPIBase      = "https://mikanani.me"
	mikanWebBase      = "https://mikanani.me"

	// Animeko sets a 60s timeout on every call, noting the server is slow. The
	// shared client carries no timeout unless RELAY_TIMEOUT is set, so the
	// budget is applied per request.
	dandanplayTimeout = 60 * time.Second
	mikanTimeout      = 60 * time.Second

	// A season pack can list hundreds of episodes and a feed hundreds of
	// releases; both are bounded so a broken upstream cannot make these
	// endpoints allocate without limit.
	maxEpisodesPerSubject = 1000
	maxReleasesPerSubject = 500

	// Mikan's search refuses long queries outright (Animeko caps at 19
	// characters), so the term is bounded before it goes upstream.
	maxMikanSearchLength = 19
)

var bangumiCacheTTL = map[string]time.Duration{
	"calendar": 30 * time.Minute,
	"subject":  6 * time.Hour,
	"search":   10 * time.Minute,
	// Rank is recomputed from every user's score, so it drifts slowly; a page of
	// the ranking is stable enough to cache like the calendar.
	"ranking": 30 * time.Minute,
	// Episode lists change when a season airs, not minute to minute.
	"episodes": 6 * time.Hour,
	// Comments are effectively append-only for a released episode.
	"danmaku": 10 * time.Minute,
	// A release feed grows a few times a day at most.
	"releases": 30 * time.Minute,
}

// bangumiFetch retrieves a Bangumi path, serving a cached copy when it has one.
//
// The cache is what keeps the upstream quota intact: a visitor browsing the
// catalogue re-reads the same subjects, and without it every page view would
// spend upstream budget on data that has not changed in hours.
func bangumiFetch(c *gin.Context, cacheKey string, ttl time.Duration, request func() (string, error)) {
	// RedisGet dereferences the client unconditionally, so an instance running
	// without Redis must not reach it -- that panics rather than erroring.
	if common.RedisEnabled {
		if cached, err := common.RedisGet(cacheKey); err == nil && cached != "" {
			c.Data(http.StatusOK, "application/json; charset=utf-8", []byte(cached))
			return
		}
	}
	body, err := request()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
		return
	}
	if common.RedisEnabled {
		// A cache write failure is not the visitor's problem; the response is
		// already correct without it.
		_ = common.RedisSet(cacheKey, body, ttl)
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", []byte(body))
}

// bangumiGet performs one upstream request with this instance's identity.
func bangumiGet(path string) (string, error) {
	request, err := http.NewRequest(http.MethodGet, bangumiAPIBase+path, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("User-Agent", bangumiUserAgent)
	request.Header.Set("Accept", "application/json")

	response, err := service.GetBangumiHTTPClient().Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, bangumiMaxBody))
	if err != nil {
		return "", err
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("bangumi upstream returned %d", response.StatusCode)
	}
	return string(body), nil
}

// GetBangumiCalendar returns today's airing anime grouped by weekday.
func GetBangumiCalendar(c *gin.Context) {
	bangumiFetch(c, "bangumi:calendar", bangumiCacheTTL["calendar"], func() (string, error) {
		return bangumiGet("/calendar")
	})
}

// GetBangumiSubject returns one subject with its episode list.
func GetBangumiSubject(c *gin.Context) {
	// The id goes into a cache key and an upstream path, so keep it to digits
	// rather than trusting the shape.
	id, ok := animePathID(c, "id", "invalid subject id")
	if !ok {
		return
	}
	bangumiFetch(c, "bangumi:subject:"+id, bangumiCacheTTL["subject"], func() (string, error) {
		return bangumiGet("/v0/subjects/" + id)
	})
}

// GetBangumiSearch searches subjects by keyword.
//
// Bangumi's search is a POST even for a read: the GET form returns 404, so this
// cannot be a plain proxy of a URL the caller supplies.
func GetBangumiSearch(c *gin.Context) {
	keyword := strings.TrimSpace(c.Query("keyword"))
	if keyword == "" {
		// Answer in the same shape the proxied call would, so the client does
		// not need a second code path for the empty case.
		c.Data(http.StatusOK, "application/json; charset=utf-8", []byte(`{"data":[],"total":0,"limit":0,"offset":0}`))
		return
	}
	// The keyword is part of the cache key and is forwarded upstream, so bound
	// it here: an unbounded key is a Redis key-length problem and a way to fill
	// the cache with junk, and the upstream pays for the oversized query too.
	if len(keyword) > maxBangumiKeywordBytes {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "keyword is too long"})
		return
	}
	limit := 20
	if raw := c.Query("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 50 {
			limit = parsed
		}
	}

	body, err := common.Marshal(map[string]any{
		"keyword": keyword,
		"filter":  map[string]any{"type": []int{bangumiSearchType}},
		"limit":   limit,
		"offset":  0,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}

	// The keyword is part of the cache key, so bound it: an unbounded key is
	// both a Redis key-length problem and a way to fill the cache with junk.
	cacheKey := "bangumi:search:" + url.QueryEscape(keyword) + ":" + strconv.Itoa(limit)
	bangumiFetch(c, cacheKey, bangumiCacheTTL["search"], func() (string, error) {
		request, err := http.NewRequest(http.MethodPost,
			bangumiAPIBase+"/v0/search/subjects", bytes.NewReader(body))
		if err != nil {
			return "", err
		}
		request.Header.Set("User-Agent", bangumiUserAgent)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/json")

		response, err := service.GetBangumiHTTPClient().Do(request)
		if err != nil {
			return "", err
		}
		defer response.Body.Close()
		payload, err := io.ReadAll(io.LimitReader(response.Body, bangumiMaxBody))
		if err != nil {
			return "", err
		}
		if response.StatusCode != http.StatusOK {
			return "", fmt.Errorf("bangumi upstream returned %d", response.StatusCode)
		}
		return string(payload), nil
	})
}

// bangumiRankingOffset maps a caller's page onto an upstream offset, clamped to
// the pages this instance is willing to proxy. Anything that is not a page
// number in range reads as the first page rather than an error: the upstream
// list is what the viewer wants, and a bad page should not deny them it.
func bangumiRankingOffset(raw string) int {
	page := 1
	if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= bangumiRankingMaxPage {
		page = parsed
	}
	return (page - 1) * bangumiRankingPageSize
}

// GetBangumiRanking returns the highest-ranked anime, one page at a time.
//
// bgm.tv's ranking is its subject list sorted by rank, so this is the search
// upstream with the sort fixed. The page number is bounded because it keys the
// cache and indexes into the upstream list.
func GetBangumiRanking(c *gin.Context) {
	offset := bangumiRankingOffset(c.Query("page"))
	page := offset/bangumiRankingPageSize + 1
	bangumiFetch(c, "bangumi:ranking:"+strconv.Itoa(page), bangumiCacheTTL["ranking"], func() (string, error) {
		return bangumiGet(fmt.Sprintf("/v0/subjects?type=%d&sort=rank&limit=%d&offset=%d",
			bangumiSearchType, bangumiRankingPageSize, offset))
	})
}

// ProxyBangumiImage streams a cover through this instance.
//
// The path is forwarded verbatim, so the caller owns the size decision: the
// host accepts a `/r/<width>/` prefix only on the `l` (original) path, and
// answers 400 for one on a `c` or `m` thumbnail. The frontend therefore asks
// for the original under the width it needs rather than the thumbnail.
func ProxyBangumiImage(c *gin.Context) {
	// Gin's wildcard keeps the leading slash, and the upstream answers 500 on
	// a doubled one, so it is trimmed rather than concatenated.
	path := strings.Trim(strings.TrimSpace(c.Param("path")), "/")
	if path == "" || strings.Contains(path, "..") || strings.Contains(path, "://") {
		c.Status(http.StatusBadRequest)
		return
	}
	request, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet,
		bangumiImageBase+"/"+path, nil)
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	request.Header.Set("User-Agent", bangumiUserAgent)
	// The host checks this against the site it expects.
	request.Header.Set("Referer", "https://bgm.tv/")

	response, err := service.GetBangumiHTTPClient().Do(request)
	if err != nil {
		c.Status(http.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		c.Status(response.StatusCode)
		return
	}
	// The upstream chooses the content type and this response is served from the
	// gateway's own origin, so a hostile or compromised image host returning
	// text/html would otherwise be a script running on this site. Only image
	// types are forwarded, and the browser is told not to sniff past the header.
	contentType := strings.ToLower(strings.TrimSpace(response.Header.Get("Content-Type")))
	if !strings.HasPrefix(contentType, "image/") {
		contentType = "application/octet-stream"
	}
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Disposition", "inline")
	c.DataFromReader(http.StatusOK, response.ContentLength, contentType,
		response.Body, nil)
}

// dandanplayCredentials returns the client credentials for dandanplay.
//
// dandanplay signs every request with an application id and secret, so an
// instance cannot reach it anonymously. The values are supplied by the
// deployment rather than defaulted here: they identify the calling application,
// and one committed to source is one every clone shares, spending the same
// upstream quota. An instance with them unset still serves the catalogue; the
// playback endpoints report the upstream's rejection.
func dandanplayCredentials() (appID, appSecret string) {
	return common.GetEnvOrDefaultString("DANDANPLAY_APP_ID", ""),
		common.GetEnvOrDefaultString("DANDANPLAY_APP_SECRET", "")
}

// dandanplaySignature computes the X-Signature header for a request.
//
// The signed material is appId + timestamp + path + appSecret, SHA-256 hashed
// and base64 encoded. `path` must be the URL-encoded path WITHOUT the query
// string. This is the single detail that makes or breaks the integration: a
// signature computed over the full request URL is rejected with 403, while the
// same signature with the query appended afterwards is accepted.
func dandanplaySignature(appID, appSecret, timestamp, path string) string {
	digest := sha256.Sum256([]byte(appID + timestamp + path + appSecret))
	return base64.StdEncoding.EncodeToString(digest[:])
}

// animeUpstreamGet performs one upstream request as this instance.
//
// The requests go through the SSRF-protected client like the bgm.tv ones: it
// validates the target and resolves proxies from the environment, which is what
// makes these upstreams reachable from a network that cannot reach them
// directly. The protections are deliberately not bypassed.
func animeUpstreamGet(ctx context.Context, target string, headers map[string]string, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}

	response, err := service.GetBangumiHTTPClient().Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, bangumiMaxBody))
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		return nil, errAnimeUpstream(fmt.Sprintf("upstream returned %d", response.StatusCode))
	}
	return body, nil
}

// dandanplayGet calls dandanplay with a signed request.
//
// The query is passed separately from the path precisely because it must stay
// out of the signature: it is appended to the URL after signing and never
// folded into the signed path.
func dandanplayGet(ctx context.Context, path string, query url.Values) ([]byte, error) {
	appID, appSecret := dandanplayCredentials()
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)

	target := dandanplayAPIBase + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}

	return animeUpstreamGet(ctx, target, map[string]string{
		"X-AppId":     appID,
		"X-Timestamp": timestamp,
		"X-Signature": dandanplaySignature(appID, appSecret, timestamp, path),
		"Accept":      "application/json",
		"User-Agent":  bangumiUserAgent,
	}, dandanplayTimeout)
}

// errAnimeUpstream reports a failure that came from an upstream rather than
// from this instance. The message is passed through to the caller as-is.
func errAnimeUpstream(message string) error {
	return errors.New(message)
}

// animeRelease mirrors one Mikan release, plus the attributes Animeko derives
// from its title. The field names follow Animeko's Topic/TopicDetails
// vocabulary so the two stay comparable.
type animeRelease struct {
	TopicID       string   `json:"topicId"`
	Title         string   `json:"title"`
	Alliance      string   `json:"alliance"`
	Description   string   `json:"description"`
	DownloadLink  string   `json:"downloadLink"`
	EpisodeLink   string   `json:"episodeLink,omitempty"`
	Size          int64    `json:"size"`
	PublishedAt   string   `json:"publishedAt"`
	PublishedMs   int64    `json:"publishedTime"`
	EpisodeNumber int      `json:"episodeNumber"`
	Resolution    string   `json:"resolution"`
	MediaOrigin   string   `json:"mediaOrigin,omitempty"`
	SubtitleLangs []string `json:"subtitleLanguages"`
	SubtitleKind  string   `json:"subtitleKind,omitempty"`
	// VideoCodec and Container are parsed from the title. Playable is derived
	// from both here rather than in the browser, so the rule that decides
	// whether an episode can be watched is written once.
	VideoCodec string `json:"videoCodec,omitempty"`
	Container  string `json:"container,omitempty"`
	Playable   bool   `json:"playable"`
	// MatchKind records how the subject was located. It is always EXACT: a
	// candidate Mikan series is only accepted once its own page links back to
	// this bgm.tv subject, so a name-only match is discarded rather than
	// reported. (Animeko's MatchKind, without the FUZZY case it can produce.)
	MatchKind string `json:"matchKind"`
}

// animeEpisode is one episode, with the release chosen for it when one matches.
type animeEpisode struct {
	EpisodeID     int64         `json:"episodeId"`
	EpisodeTitle  string        `json:"episodeTitle"`
	EpisodeNumber string        `json:"episodeNumber"`
	AirDate       string        `json:"airDate,omitempty"`
	Release       *animeRelease `json:"release,omitempty"`
}

// mikanRSS is the subset of Mikan's RSS 2.0 the page needs.
type mikanRSS struct {
	XMLName xml.Name     `xml:"rss"`
	Channel mikanChannel `xml:"channel"`
}

type mikanChannel struct {
	Title string         `xml:"title"`
	Items []mikanRSSItem `xml:"item"`
}

type mikanRSSItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	GUID        string `xml:"guid"`
	Torrent     struct {
		ContentLength string `xml:"contentLength"`
		PubDate       string `xml:"pubDate"`
	} `xml:"torrent"`
	Enclosure struct {
		URL string `xml:"url,attr"`
	} `xml:"enclosure"`
}

// parseMikanFeed turns a feed into releases.
//
// A feed is not guaranteed to be usable. Mikan answers 200 with an empty channel
// for a subject it has no releases for, and an item with no enclosure cannot be
// played by anyone. Such items are dropped rather than surfaced, because every
// returned release is offered to the visitor as something their browser can
// actually join.
func parseMikanFeed(body []byte, matchKind string) []animeRelease {
	var feed mikanRSS
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil
	}

	releases := make([]animeRelease, 0, len(feed.Channel.Items))
	for _, item := range feed.Channel.Items {
		if len(releases) >= maxReleasesPerSubject {
			break
		}
		download := strings.TrimSpace(item.Enclosure.URL)
		if download == "" {
			continue
		}

		// Animeko takes the topic id from the guid's last path segment, which on
		// Mikan is the info hash. A guid that states none, or whose last segment
		// is the title text rather than a hash, falls back to the enclosure's
		// filename so the release still keeps a stable identity.
		topicID := mikanTopicIDFromGUID(item.GUID)
		if topicID == "" {
			topicID = mikanTopicIDFromDownload(download)
		}

		title := strings.TrimSpace(item.Title)
		release := animeRelease{
			TopicID:       topicID,
			Title:         title,
			Alliance:      mikanAllianceFromTitle(title),
			Description:   strings.TrimSpace(item.Description),
			DownloadLink:  download,
			EpisodeLink:   strings.TrimSpace(item.Link),
			PublishedAt:   strings.TrimSpace(item.Torrent.PubDate),
			PublishedMs:   mikanPublishedMillis(item.Torrent.PubDate),
			MatchKind:     matchKind,
			SubtitleLangs: []string{},
		}
		// Mikan sends contentLength in bytes. A missing or unparsable value
		// stays 0 rather than becoming a bogus size in the UI.
		if size, err := strconv.ParseInt(strings.TrimSpace(item.Torrent.ContentLength), 10, 64); err == nil && size > 0 {
			release.Size = size
		}
		applyAnimeTitleAttributes(&release, title)

		releases = append(releases, release)
	}
	return releases
}

// mikanTopicIDFromGUID reads the info hash from an item's guid.
//
// A Mikan guid is usually the release title with no path separator at all, in
// which case it is not a hash and must not be used as one -- hence the hex
// check, which is what distinguishes a real hash from arbitrary title text.
func mikanTopicIDFromGUID(guid string) string {
	guid = strings.TrimSpace(guid)
	_, last, _ := strings.Cut(guid, "/")
	last = strings.TrimSpace(last)
	if mikanInfoHash.MatchString(last) {
		return last
	}
	return ""
}

// mikanTopicIDFromDownload reads the info hash out of an enclosure URL, whose
// last path segment is the hash with a .torrent suffix.
func mikanTopicIDFromDownload(download string) string {
	name := path.Base(strings.TrimSpace(download))
	name = strings.TrimSuffix(name, ".torrent")
	if mikanInfoHash.MatchString(name) {
		return name
	}
	return ""
}

// mikanInfoHash is a BitTorrent info hash: 40 hex characters. Mikan names its
// torrent files after it, so this identifies a release across a feed.
var mikanInfoHash = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

// mikanAllianceSuffix splits a title at its first closing bracket. Animeko takes
// the text before it and strips the opening bracket, so "[LoliHouse] Some Show
// [1080p]" yields "LoliHouse". Splitting on the OPENING bracket instead would
// return the empty string before it.
var mikanAllianceSuffix = regexp.MustCompile(`[\]】]`)

func mikanAllianceFromTitle(title string) string {
	head := mikanAllianceSuffix.Split(strings.TrimSpace(title), 2)[0]
	head = strings.TrimPrefix(head, "[")
	head = strings.TrimPrefix(head, "【")
	return strings.TrimSpace(head)
}

// mikanPublishedMillis converts Mikan's timestamp to Unix milliseconds.
//
// Mikan sends "2026-07-21T22:37:44.292021" in UTC+8 with no zone marker, which
// is why it cannot be handed to time.Parse as-is.
func mikanPublishedMillis(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	// Drop the sub-second part: it is microseconds and the layout below has no
	// fractional component.
	if dot := strings.IndexByte(value, '.'); dot > 0 {
		value = value[:dot]
	}
	parsed, err := time.ParseInLocation("2006-01-02T15:04:05", value, time.FixedZone("UTC+8", 8*60*60))
	if err != nil {
		return 0
	}
	return parsed.UnixMilli()
}

// animeResolutionTokens are Animeko's resolutions and their spellings. The id
// doubles as the sort key; 1440P and 2160P display as 2K and 4K upstream but
// keep these ids so they compare numerically.
var animeResolutionTokens = []struct {
	ID    string
	Alias string
}{
	{"240P", "x240"},
	{"360P", "x360"},
	{"480P", "x480"},
	{"560P", "x560"},
	{"720P", "x720"},
	{"1080P", "x1080"},
	{"1440P", "x1440"},
	{"2160P", "x2160"},
}

func animeParseResolution(text string) string {
	lowered := strings.ToLower(text)
	for _, entry := range animeResolutionTokens {
		if strings.Contains(lowered, strings.ToLower(entry.ID)) || strings.Contains(lowered, entry.Alias) {
			return entry.ID
		}
	}
	return ""
}

// animeMediaOrigins mirrors Animeko's MediaOrigin enum and its aliases.
var animeMediaOrigins = []struct {
	ID    string
	Names []string
}{
	{"BDRip", nil},
	{"BluRay", []string{"Blu-Ray"}},
	{"WebRip", nil},
	{"Baha", nil},
	{"TVRip", nil},
}

func animeParseMediaOrigin(text string) string {
	lowered := strings.ToLower(text)
	for _, entry := range animeMediaOrigins {
		if strings.Contains(lowered, strings.ToLower(entry.ID)) {
			return entry.ID
		}
		for _, alias := range entry.Names {
			if strings.Contains(lowered, strings.ToLower(alias)) {
				return entry.ID
			}
		}
	}
	return ""
}

// animeVideoCodecs are the encodings a release title names, checked in the
// order written. Only the first is universally decodable in a browser, which is
// the reason this is parsed rather than merely displayed.
var animeVideoCodecs = []struct {
	ID     string
	Tokens []string
}{
	{ID: "H264", Tokens: []string{"h.264", "h264", "avc", "x264"}},
	{ID: "H265", Tokens: []string{"h.265", "h265", "hevc", "x265", "hvc1", "hev1"}},
	{ID: "AV1", Tokens: []string{"av1"}},
	{ID: "VP9", Tokens: []string{"vp9"}},
}

func animeParseVideoCodec(text string) string {
	lowered := strings.ToLower(text)
	for _, entry := range animeVideoCodecs {
		for _, token := range entry.Tokens {
			if strings.Contains(lowered, token) {
				return entry.ID
			}
		}
	}
	return ""
}

// animeContainers are the file containers a release title names.
var animeContainers = []struct {
	ID     string
	Tokens []string
}{
	{ID: "MP4", Tokens: []string{"mp4", "m4v"}},
	{ID: "MKV", Tokens: []string{"mkv"}},
	{ID: "AVI", Tokens: []string{"avi"}},
	{ID: "WebM", Tokens: []string{"webm"}},
}

func animeParseContainer(text string) string {
	lowered := strings.ToLower(text)
	for _, entry := range animeContainers {
		for _, token := range entry.Tokens {
			if strings.Contains(lowered, token) {
				return entry.ID
			}
		}
	}
	return ""
}

// animeIsBrowserPlayable reports whether a browser can decode a release as
// published, without this instance re-encoding anything.
//
// H.264 is the deciding factor and the container is not consulted. Chromium --
// the engine behind Chrome, Edge and the browsers most visitors here use --
// plays H.264 in both MP4 and MKV, and MKV is what most releases ship in.
// Requiring MP4 as well would have marked 12% of releases playable rather than
// 30%, leaving some subjects with nothing to offer. The cost is a viewer on
// Firefox or Safari who picks an MKV, which the release list cannot warn about
// because it does not know the browser.
//
// H.265 is the case this exists for: it is common in the better-looking releases
// and is exactly what a browser usually cannot play.
func animeIsBrowserPlayable(release animeRelease) bool {
	return release.VideoCodec == "H264"
}

// animeSubtitleLanguages mirrors Animeko's matchable entries in its parse
// order: CHS, CHT, CHC, JPN, ENG. The order matters: "简繁内封字幕" matches
// both Chinese variants and Animeko keeps both, which is what makes it a CLOSED
// (internal-subtitle) release.
var animeSubtitleLanguages = []struct {
	ID      string
	Tokens  []string
	Exclude []string
}{
	{ID: "CHS", Tokens: []string{"简中", "GB", "GBK", "简体中文", "中文", "中字", "簡", "简", "CHS", "Zh-Hans", "Zh_Hans", "zh_cn", "zh"}, Exclude: []string{"繁體中文"}},
	{ID: "CHT", Tokens: []string{"繁中", "BIG5", "BIG 5", "繁", "Chinese", "CHT", "TC"}},
	{ID: "CHC", Tokens: []string{"粤", "粵", "Cantonese", "CHC", "Yue"}},
	{ID: "JPN", Tokens: []string{"日", "Japanese", "JP"}},
	{ID: "ENG", Tokens: []string{"英", "English"}},
}

func animeParseSubtitleLanguages(text string) []string {
	lowered := strings.ToLower(text)
	var found []string
	for _, language := range animeSubtitleLanguages {
		if slices.ContainsFunc(language.Exclude, func(excluded string) bool {
			return strings.Contains(lowered, strings.ToLower(excluded))
		}) {
			continue
		}
		for _, token := range language.Tokens {
			if strings.Contains(lowered, strings.ToLower(token)) {
				found = append(found, language.ID)
				break
			}
		}
	}
	return found
}

// animeParseFrameRate mirrors Animeko's FrameRate, which only represents 60fps
// and detects it by exactly these spellings.
func animeParseFrameRate(text string) string {
	lowered := strings.ToLower(text)
	for _, marker := range []string{"@60", "1080p60", "2160p60", "60fps", "60 fps"} {
		if strings.Contains(lowered, marker) {
			return "60"
		}
	}
	return ""
}

// animeParseSubtitleKind derives the subtitle kind in Animeko's priority order:
// an explicit 内嵌/内封/外挂 marker wins, otherwise a release carrying two or
// more non-Japanese languages counts as CLOSED, since multi-language releases
// are nearly always muxed rather than hardcoded.
func animeParseSubtitleKind(text string, languages []string) string {
	switch {
	case strings.Contains(text, "内嵌"), strings.Contains(text, "內嵌"):
		return "EMBEDDED"
	case strings.Contains(text, "内封"), strings.Contains(text, "內封"):
		return "CLOSED"
	case strings.Contains(text, "外挂"), strings.Contains(text, "外掛"):
		return "EXTERNAL_DISCOVER"
	}
	nonJapanese := 0
	for _, language := range languages {
		if language != "JPN" {
			nonJapanese++
		}
	}
	if nonJapanese >= 2 {
		return "CLOSED"
	}
	return ""
}

// applyAnimeTitleAttributes fills a release's parsed attributes from its title,
// which is where Mikan carries them: an item has no resolution or subtitle
// field, only the raw release name.
func applyAnimeTitleAttributes(release *animeRelease, title string) {
	release.EpisodeNumber = animeEpisodeNumberInTitle(title)
	release.Resolution = animeParseResolution(title)
	release.MediaOrigin = animeParseMediaOrigin(title)
	release.VideoCodec = animeParseVideoCodec(title)
	release.Container = animeParseContainer(title)
	release.Playable = animeIsBrowserPlayable(*release)

	// Animeko checks this before anything else: a release that says it has no
	// Chinese subtitles states exactly what it has, so the token match below --
	// which reads 中文字幕 as CHS -- must not be allowed to claim otherwise.
	// That leaves a foreign-language release carrying only its own language,
	// which is what keeps it from outranking a real subtitled release.
	if strings.Contains(title, "无中文字幕") {
		release.SubtitleLangs = animeSubtitleLanguagesWithoutChinese(title)
		return
	}

	release.SubtitleLangs = animeParseSubtitleLanguages(title)
	release.SubtitleKind = animeParseSubtitleKind(title, release.SubtitleLangs)
}

// animeSubtitleLanguagesWithoutChinese parses languages while excluding the
// Chinese tokens, for a release that declares itself subtitled in none.
func animeSubtitleLanguagesWithoutChinese(text string) []string {
	var found []string
	for _, language := range animeSubtitleLanguages {
		if language.ID == "CHS" || language.ID == "CHT" || language.ID == "CHC" {
			continue
		}
		lowered := strings.ToLower(text)
		for _, token := range language.Tokens {
			if strings.Contains(lowered, strings.ToLower(token)) {
				found = append(found, language.ID)
				break
			}
		}
	}
	return found
}

// Choosing which release to play, ported from Animeko's MediaSelectionDecider.
//
// Animeko does not score candidates. It walks fixed preference axes in order --
// resolution, then subtitle language, then release group, then source -- and
// takes the first candidate surviving every axis, backing off one axis at a
// time. That ordering is deliberate (MediaSelectionDecider.kt:83):
//
//	1. resolution  2. subtitle language  3. release group  4. data source
//	- the highest resolution wins (1080P >> 720P), but not at the cost of a
//	  wanted subtitle language
//	- a preferred release group is not worth giving up a better language for
//
// Two consequences are load-bearing and easy to lose in a rewrite: an axis is
// relaxed only once the whole level below it came up empty, and the release
// group is relaxed INSIDE the language loop rather than after it. A plain
// "sort by resolution, then language" comparator gets both wrong.

// animeAnyFilter is Animeko's ANY_FILTER: match anything on this axis instead
// of narrowing to one value.
const animeAnyFilter = "ANY"

// Animeko's default fallbacks (MediaPreference.kt:42-62): 4K down to 720p, then
// simplified and traditional Chinese. The release group and source default to
// matching anything.
var (
	animeResolutionFallbacks = []string{"2160P", "1440P", "1080P", "720P"}
	animeLanguageFallbacks   = []string{"CHS", "CHT"}
	animeMediaSourceFallback = []string{animeAnyFilter}
)

// animeOrderReleases returns the candidates best-first, which is Animeko's
// sortMediaList reduced to the terms that apply to a single BitTorrent source.
//
// Kept: newer releases first (publishedTime descending), with the release id as
// a stable tiebreak so equal timestamps do not reorder between requests.
//
// Deliberately not ported: Animeko's leading terms rank a locally cached copy
// above a remote one and a cheaper-to-download source above an expensive one.
// This page has no local cache and one source kind, so those terms are constant
// across every candidate and cannot change the outcome. Codec and file size are
// not ranking inputs in Animeko either -- MediaProperties.size appears only in a
// progress UI -- so a smaller or newer encode is not preferred here either.
func animeOrderReleases(candidates []animeRelease) []animeRelease {
	ordered := slices.Clone(candidates)
	slices.SortStableFunc(ordered, func(a, b animeRelease) int {
		if a.PublishedMs != b.PublishedMs {
			// Descending: the newer release sorts first.
			return int(b.PublishedMs - a.PublishedMs)
		}
		return strings.Compare(a.TopicID, b.TopicID)
	})
	return ordered
}

// animeSelectRelease picks the release for one episode.
//
// It returns nil only when there is nothing to choose from.
// animeSelectRelease picks the release for one episode.
//
// Candidates a browser cannot play are dropped first, whenever any playable one
// exists. Filtering rather than ranking keeps the quality order meaningful: a
// 480p H.264 release should not beat a 1080p one merely for being decodable, but
// it should beat a 1080p HEVC that will not play at all.
func animeSelectRelease(candidates []animeRelease) *animeRelease {
	ordered := animeOrderReleases(animePreferPlayable(candidates))
	if len(ordered) == 0 {
		return nil
	}

	selected := animeSelectImpl(ordered,
		animeResolutionFallbacks, animeLanguageFallbacks,
		[]string{animeAnyFilter}, animeMediaSourceFallback)
	if selected == nil {
		// Animeko's selectAny fallback. The list is non-empty here because it was
		// sorted above, so this takes the best-ordered candidate rather than
		// nothing.
		return &ordered[0]
	}
	return selected
}

// animePreferPlayable narrows the candidates to those a browser can play, and
// returns them all when none can. Returning the full list rather than nothing
// keeps a subject whose releases are all H.265 available to a viewer who will
// play it somewhere else.
func animePreferPlayable(candidates []animeRelease) []animeRelease {
	playable := animeFilterReleases(candidates, func(release animeRelease) bool {
		return release.Playable
	})
	if len(playable) > 0 {
		return playable
	}
	return candidates
}

// animeSelectImpl is Animeko's selectImpl: a nested loop over the preference
// axes returning the first candidate that survives all of them.
//
// The release-group loop sits inside the language loop, and when it comes up
// empty the source loop runs again over the LANGUAGE-filtered list rather than
// the group-filtered one. That is what stops a wanted language from being traded
// away for a wanted release group.
func animeSelectImpl(candidates []animeRelease, resolutions, languages, alliances, sources []string) *animeRelease {
	for _, resolution := range animeWithAnyFilter(resolutions) {
		byResolution := animeFilterReleases(candidates, func(release animeRelease) bool {
			return resolution == animeAnyFilter || resolution == release.Resolution
		})
		if len(byResolution) == 0 {
			continue
		}

		for _, language := range animeWithAnyFilter(languages) {
			byLanguage := animeFilterReleases(byResolution, func(release animeRelease) bool {
				return language == animeAnyFilter || slices.Contains(release.SubtitleLangs, language)
			})
			if len(byLanguage) == 0 {
				continue
			}

			for _, alliance := range animeWithAnyFilter(alliances) {
				byAlliance := animeFilterReleases(byLanguage, func(release animeRelease) bool {
					return alliance == animeAnyFilter || alliance == release.Alliance
				})
				if len(byAlliance) == 0 {
					continue
				}
				if picked := animeSelectBySource(byAlliance, sources); picked != nil {
					return picked
				}
			}

			// The release group did not match. Fall through to the source axis
			// over the LANGUAGE-filtered list, not the group-filtered one: a
			// better language beats a preferred group.
			if picked := animeSelectBySource(byLanguage, sources); picked != nil {
				return picked
			}
		}
	}
	return nil
}

func animeSelectBySource(candidates []animeRelease, sources []string) *animeRelease {
	for _, source := range animeWithAnyFilter(sources) {
		bySource := animeFilterReleases(candidates, func(animeRelease) bool {
			return source == animeAnyFilter
		})
		if len(bySource) == 0 {
			continue
		}
		return &bySource[0]
	}
	return nil
}

// animeWithAnyFilter appends Animeko's terminal ANY_FILTER, which every axis
// sequence ends with so a level that matched nothing still yields a candidate.
func animeWithAnyFilter(values []string) []string {
	return append(slices.Clone(values), animeAnyFilter)
}

func animeFilterReleases(releases []animeRelease, keep func(animeRelease) bool) []animeRelease {
	filtered := make([]animeRelease, 0, len(releases))
	for _, release := range releases {
		if keep(release) {
			filtered = append(filtered, release)
		}
	}
	return filtered
}

// Mapping a bgm.tv subject onto Mikan's releases.
//
// Mikan has its own id space, and the only link between the two is a bgm.tv
// subject link that Mikan prints on its own series page. That link is resolved
// the way Animeko resolves it (findMikanSubjectIdByName): search by name, read
// the subject id back out of each candidate's page, and accept only an exact
// match. A name search alone is never trusted, because Mikan's titles routinely
// cover several seasons and an unchecked match would attach the wrong season's
// releases to this subject.

var (
	mikanBangumiLinkPattern    = regexp.MustCompile(`(?i)href="([^"]*/Home/Bangumi/\d+)"`)
	mikanSubjectIDPattern      = regexp.MustCompile(`/Home/Bangumi/(\d+)`)
	mikanBangumiSubjectPattern = regexp.MustCompile(`/subject/(\d+)`)
)

// animeFindMikanID returns the Mikan series id whose page links to this bgm.tv
// subject, or "" when none does.
func animeFindMikanID(ctx context.Context, subjectID, keyword string) (string, error) {
	// Mikan refuses long search terms, so the query is cut to the length
	// Animeko uses.
	term, _, _ := strings.Cut(strings.TrimSpace(keyword), " ")
	if runes := []rune(term); len(runes) > maxMikanSearchLength {
		term = string(runes[:maxMikanSearchLength])
	}

	searchURL := mikanWebBase + "/Home/Search?searchstr=" + url.QueryEscape(term)
	body, err := animeUpstreamGet(ctx, searchURL, mikanHTMLHeaders(), mikanTimeout)
	if err != nil {
		return "", err
	}

	seen := map[string]bool{}
	var candidates []string
	for _, match := range mikanBangumiLinkPattern.FindAllStringSubmatch(string(body), -1) {
		id := strings.TrimPrefix(match[1], "/Home/Bangumi/")
		if !seen[id] {
			seen[id] = true
			candidates = append(candidates, id)
		}
	}
	if len(candidates) > 5 {
		// Animeko reads a handful concurrently and takes the first exact match.
		// The bound keeps one over-broad search from fanning out over Mikan.
		candidates = candidates[:5]
	}

	for _, mikanID := range candidates {
		page, err := animeUpstreamGet(ctx, mikanWebBase+"/Home/Bangumi/"+mikanID, mikanHTMLHeaders(), mikanTimeout)
		if err != nil {
			continue
		}
		if animePageLinksToSubject(page, subjectID) {
			return mikanID, nil
		}
	}
	return "", nil
}

// animePageLinksToSubject reports whether a Mikan series page names this
// bgm.tv subject.
func animePageLinksToSubject(page []byte, subjectID string) bool {
	matches := mikanBangumiSubjectPattern.FindAllStringSubmatch(string(page), -1)
	return slices.ContainsFunc(matches, func(match []string) bool {
		return match[1] == subjectID
	})
}

func mikanHTMLHeaders() map[string]string {
	return map[string]string{
		"Accept":     "text/html,application/xhtml+xml",
		"User-Agent": bangumiUserAgent,
	}
}

// ProxyBangumiTorrent streams a .torrent file through this instance.
//
// The browser cannot fetch it directly: Mikan serves the file without CORS
// headers, so a fetch from the page is blocked before WebTorrent ever sees it.
// The upstream host is fixed and only the /Download/ path is accepted, which
// keeps this an allowlisted fetch rather than a proxy for arbitrary URLs.
// validMikanDownloadPath reports whether a path names one of Mikan's own torrent
// downloads. The host is fixed by the caller, so accepting only this shape keeps
// both the torrent proxy and the stream an allowlisted fetch rather than a proxy
// for arbitrary URLs.
func validMikanDownloadPath(torrentPath string) bool {
	return torrentPath != "" &&
		strings.HasPrefix(torrentPath, "Download/") &&
		!strings.Contains(torrentPath, "..") &&
		!strings.Contains(torrentPath, "://")
}

func ProxyBangumiTorrent(c *gin.Context) {
	torrentPath := strings.Trim(strings.TrimSpace(c.Param("path")), "/")
	if !validMikanDownloadPath(torrentPath) {
		c.Status(http.StatusBadRequest)
		return
	}

	body, err := animeUpstreamGet(c.Request.Context(), mikanAPIBase+"/"+torrentPath, mikanHTMLHeaders(), mikanTimeout)
	if err != nil {
		c.Status(http.StatusBadGateway)
		return
	}
	c.Data(http.StatusOK, "application/x-bittorrent", body)
}

// --- Playback: streaming an episode through the local BitTorrent backend -----
//
// Mikan publishes torrents, and a browser cannot join a swarm of ordinary
// BitTorrent peers -- it speaks HTTP, they speak TCP and UDP. Something has to
// sit in the data path and translate, and this instance is that something: rqbit
// runs beside it, joins the swarm, and serves the file over HTTP, which the
// browser's own player then reads.
//
// The bytes are copied through without buffering, so serving a two gigabyte
// episode costs a buffer rather than the episode, and rqbit's own footprint is
// tens of megabytes. What it does cost is disk and bandwidth: the torrent's
// selected files are written to disk for as long as the torrent is held, which
// is why an idle one is evicted.

const (
	// rqbitAPIBase is the local BitTorrent backend. A loopback address, because
	// the process runs beside this one rather than somewhere on the network.
	rqbitAPIBase = "http://127.0.0.1:3031"
	// rqbitAddTimeout bounds fetching the .torrent and registering it, which is
	// the slow part before any bytes move.
	rqbitAddTimeout = 60 * time.Second
	// rqbitControlTimeout bounds the control calls that are not streaming:
	// registering a torrent and evicting one. The shared client carries no
	// overall timeout because a stream stays open for as long as the viewer
	// watches, so every non-streaming call sets its own deadline.
	rqbitControlTimeout = 30 * time.Second
	// rqbitIdleTimeout is how long a torrent is kept after its last request.
	// It holds the episode's own size on disk the whole time, so eviction has to
	// be prompt on a volume this instance shares with the database.
	rqbitIdleTimeout = 20 * time.Minute
)

// rqbitFile is the part of a torrent's file list this needs.
type rqbitFile struct {
	Name     string `json:"name"`
	Length   int64  `json:"length"`
	Included bool   `json:"included"`
}

type rqbitTorrentDetail struct {
	ID    int         `json:"id"`
	Files []rqbitFile `json:"files"`
}

// rqbitEntry is one torrent this instance is serving, the file chosen for the
// episode being watched, and when it was last asked for.
type rqbitEntry struct {
	id        int
	files     []rqbitFile
	fileIndex int
	lastUsed  time.Time
}

var (
	rqbitMu      sync.Mutex
	rqbitEntries = map[string]*rqbitEntry{}

	rqbitSweeperOnce sync.Once

	// rqbitHTTPClient has no overall timeout: a stream stays open for as long as
	// the viewer is watching, and the request's own context is what ends it.
	rqbitHTTPClient = &http.Client{}
)

// GetBangumiStream serves an episode from the torrent named in the path.
//
// The browser is handed a same-origin URL and asks for byte ranges as it plays;
// each range is forwarded to the backend and copied straight through. That is
// also why the episode is chosen here rather than in the page: the backend's
// file order is the only thing that can be streamed, and picking the wrong entry
// of a season pack plays the wrong episode.
func GetBangumiStream(c *gin.Context) {
	torrentPath := strings.Trim(strings.TrimSpace(c.Param("path")), "/")
	if !validMikanDownloadPath(torrentPath) {
		c.Status(http.StatusBadRequest)
		return
	}

	episodeNumber, _ := strconv.Atoi(c.Query("episode"))

	startRqbitSweeper()

	entry, err := ensureRqbitTorrent(c.Request.Context(), torrentPath, episodeNumber)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Anime stream could not be prepared: %v", err))
		c.Status(http.StatusBadGateway)
		return
	}

	proxyRqbitFile(c, entry.id, entry.fileIndex)
}

// proxyRqbitFile forwards one request to the backend and copies the response.
func proxyRqbitFile(c *gin.Context, torrentID, fileIndex int) {
	target := fmt.Sprintf("%s/torrents/%d/stream/%d", rqbitAPIBase, torrentID, fileIndex)
	request, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, target, nil)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	// The range is the whole point: without it the browser cannot start in the
	// middle of an episode, which is what seeking is.
	if rangeHeader := c.GetHeader("Range"); rangeHeader != "" {
		request.Header.Set("Range", rangeHeader)
	}

	response, err := rqbitHTTPClient.Do(request)
	if err != nil {
		c.Status(http.StatusBadGateway)
		return
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent {
		c.Status(http.StatusBadGateway)
		return
	}

	for _, header := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges"} {
		if value := response.Header.Get(header); value != "" {
			c.Writer.Header().Set(header, value)
		}
	}
	c.Status(response.StatusCode)
	c.Writer.WriteHeaderNow()

	if _, err := io.Copy(c.Writer, response.Body); err != nil {
		// A viewer closing the tab mid-stream is the ordinary cause, so this is
		// logged rather than surfaced.
		logger.LogError(c.Request.Context(), fmt.Sprintf("Anime stream ended early: %v", err))
	}
}

// ensureRqbitTorrent returns the backend's id for the torrent and the index of
// the file to play, adding the torrent if the backend does not have it.
func ensureRqbitTorrent(ctx context.Context, torrentPath string, episodeNumber int) (*rqbitEntry, error) {
	rqbitMu.Lock()
	cached, ok := rqbitEntries[torrentPath]
	if ok {
		cached.lastUsed = time.Now()
	}
	rqbitMu.Unlock()

	if ok {
		index, err := rqbitVideoFileIndex(cached.files, episodeNumber)
		if err == nil {
			// Copy, so the caller is not reading a struct the sweeper may replace.
			return &rqbitEntry{id: cached.id, files: cached.files, fileIndex: index, lastUsed: cached.lastUsed}, nil
		}
		rqbitMu.Lock()
		delete(rqbitEntries, torrentPath)
		rqbitMu.Unlock()
	}

	addCtx, cancel := context.WithTimeout(ctx, rqbitAddTimeout)
	defer cancel()

	body, err := animeUpstreamGet(addCtx, mikanAPIBase+"/"+torrentPath, mikanHTMLHeaders(), mikanTimeout)
	if err != nil {
		return nil, err
	}

	// Listed before it is added so the file to download is known in advance:
	// rqbit's only_files is set at add time, and adding a season pack first would
	// start pulling all of it. The whole pack is fifty gigabytes; one episode is
	// one.
	files, err := rqbitListFiles(addCtx, body)
	if err != nil {
		return nil, err
	}
	index, err := rqbitVideoFileIndex(files, episodeNumber)
	if err != nil {
		return nil, err
	}

	id, err := rqbitAddTorrent(addCtx, body, index)
	if err != nil {
		return nil, err
	}

	entry := &rqbitEntry{id: id, files: files, fileIndex: index, lastUsed: time.Now()}
	rqbitMu.Lock()
	rqbitEntries[torrentPath] = entry
	rqbitMu.Unlock()

	return entry, nil
}

// rqbitListFiles asks the backend what a .torrent contains without adding it.
func rqbitListFiles(ctx context.Context, torrent []byte) ([]rqbitFile, error) {
	detail, err := rqbitPostTorrent(ctx, torrent, "?list_only=true")
	if err != nil {
		return nil, err
	}
	if len(detail.Files) == 0 {
		return nil, errors.New("the torrent lists no files")
	}
	return detail.Files, nil
}

// rqbitAddTorrent registers the torrent, downloading only the chosen file.
func rqbitAddTorrent(ctx context.Context, torrent []byte, fileIndex int) (int, error) {
	detail, err := rqbitPostTorrent(ctx, torrent, fmt.Sprintf("?only_files=%d", fileIndex))
	if err != nil {
		return 0, err
	}
	return detail.ID, nil
}

func rqbitPostTorrent(ctx context.Context, torrent []byte, query string) (*rqbitTorrentDetail, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, rqbitAPIBase+"/torrents"+query, bytes.NewReader(torrent))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/octet-stream")

	response, err := rqbitHTTPClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the torrent backend answered %d", response.StatusCode)
	}

	var added struct {
		Details rqbitTorrentDetail `json:"details"`
	}
	if err := common.DecodeJson(response.Body, &added); err != nil {
		return nil, err
	}
	return &added.Details, nil
}

// rqbitDeleteTorrent drops the torrent and its files.
//
// The backend's delete rather than its forget: forget leaves the downloaded
// files on disk, which is the opposite of what an eviction is for.
func rqbitDeleteTorrent(ctx context.Context, id int) error {
	// Bounded, because the sweeper calls this with a background context and the
	// shared client has no timeout: a hung rqbit would otherwise park the
	// sweeper goroutine for good and stop every later eviction, filling the disk.
	ctx, cancel := context.WithTimeout(ctx, rqbitControlTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/torrents/%d/delete", rqbitAPIBase, id), nil)
	if err != nil {
		return err
	}
	response, err := rqbitHTTPClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("the torrent backend answered %d", response.StatusCode)
	}
	return nil
}

// startRqbitSweeper evicts torrents nobody is watching any more.
//
// Started on first use rather than at boot so an instance that never plays
// anything never runs the goroutine.
func startRqbitSweeper() {
	rqbitSweeperOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			for range ticker.C {
				evictIdleRqbitTorrents()
			}
		}()
	})
}

func evictIdleRqbitTorrents() {
	rqbitMu.Lock()
	var expired []*rqbitEntry
	for key, entry := range rqbitEntries {
		if time.Since(entry.lastUsed) >= rqbitIdleTimeout {
			expired = append(expired, entry)
			delete(rqbitEntries, key)
		}
	}
	rqbitMu.Unlock()

	for _, entry := range expired {
		if err := rqbitDeleteTorrent(context.Background(), entry.id); err != nil {
			logger.LogError(context.Background(), fmt.Sprintf("Failed to evict an idle torrent: %v", err))
		}
	}
}

// animeVideoExtensions are the container suffixes a release ships its episodes
// in.
var animeVideoExtensions = []string{".mkv", ".mp4", ".m4v", ".avi", ".mov", ".webm", ".ts", ".m2ts", ".flv", ".wmv"}

func isAnimeVideoFile(name string) bool {
	lowered := strings.ToLower(name)
	for _, extension := range animeVideoExtensions {
		if strings.HasSuffix(lowered, extension) {
			return true
		}
	}
	return false
}

// animeExtrasPatterns match the openings, endings and previews that sit beside
// the episodes in a release, so a season pack plays an episode rather than its
// NCOP.
var animeExtrasPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bNC(OP|ED)\b`),
	regexp.MustCompile(`(?i)\bOP\d`),
	regexp.MustCompile(`(?i)\bED\d`),
	regexp.MustCompile(`(?i)\bCM\d`),
	regexp.MustCompile(`(?i)\bPV\d?`),
	regexp.MustCompile(`(?i)\bSP\d?\b`),
	regexp.MustCompile(`(?i)\bOVA\b`),
}

func isAnimeExtraFile(name string) bool {
	for _, pattern := range animeExtrasPatterns {
		if pattern.MatchString(name) {
			return true
		}
	}
	return false
}

var (
	// animeEpisodeMarkerPattern finds a number introduced by an episode marker,
	// as in "S01E03" or "第3话".
	animeEpisodeMarkerPattern = regexp.MustCompile(`(?i)(?:ep|e|第)\s*0*(\d+)`)
	// animeDigitRunPattern finds any run of digits.
	animeDigitRunPattern = regexp.MustCompile(`\d+`)
)

// animeFileNamesEpisode reports whether a filename names the given episode.
//
// The number must stand on its own, so "Show 03" and "S01E03" match episode 3
// while the 1080 of a resolution does not. Go's regexp engine has no lookahead to
// express that in the pattern, so whole runs of digits are compared instead of
// searching for the number as a substring.
//
// A name carrying an explicit marker is judged only by the marked number. That
// is what stops "S01E03" from reading as episode 1 through its season number.
func animeFileNamesEpisode(name string, episodeNumber int) bool {
	wanted := strconv.Itoa(episodeNumber)

	if marked := animeEpisodeMarkerPattern.FindAllStringSubmatch(name, -1); len(marked) > 0 {
		for _, match := range marked {
			if strings.TrimLeft(match[1], "0") == wanted {
				return true
			}
		}
		return false
	}

	for _, run := range animeDigitRunPattern.FindAllString(name, -1) {
		if strings.TrimLeft(run, "0") == wanted {
			return true
		}
	}
	return false
}

// rqbitVideoFileIndex picks the file of a torrent to play.
//
// A release is as often a season pack as a single episode, so the file naming
// the wanted episode wins; failing that the largest remaining video does, which
// in a pack is a real episode rather than a sample or a menu. Extras are demoted
// rather than dropped, so a torrent containing nothing else still plays one.
func rqbitVideoFileIndex(files []rqbitFile, episodeNumber int) (int, error) {
	videos := make([]int, 0, len(files))
	for i := range files {
		if isAnimeVideoFile(files[i].Name) {
			videos = append(videos, i)
		}
	}
	if len(videos) == 0 {
		return 0, errors.New("the torrent contains no video file")
	}

	episodes := make([]int, 0, len(videos))
	for _, i := range videos {
		if !isAnimeExtraFile(files[i].Name) {
			episodes = append(episodes, i)
		}
	}
	pool := videos
	if len(episodes) > 0 {
		pool = episodes
	}

	if episodeNumber > 0 {
		matches := make([]int, 0, len(pool))
		for _, i := range pool {
			if animeFileNamesEpisode(files[i].Name, episodeNumber) {
				matches = append(matches, i)
			}
		}
		if len(matches) > 0 {
			pool = matches
		}
	}

	// The largest of what is left, which is the rule for both cases: among files
	// naming the episode it prefers the real one over a short special, and with
	// none naming it, the largest is a real episode rather than a sample or a
	// menu. Taking the first match instead picked whichever the pack happened to
	// list first -- for one season pack that was its fourteen megabyte S00E01
	// rather than the two gigabyte episode.
	largest := pool[0]
	for _, i := range pool {
		if files[i].Length > files[largest].Length {
			largest = i
		}
	}
	return largest, nil
}

// animeFetchReleases returns the releases for a subject, resolving the Mikan id
// first. The keyword is needed because Mikan cannot be searched by bgm.tv id.
func animeFetchReleases(ctx context.Context, subjectID, keyword string) ([]animeRelease, string, error) {
	mikanID, err := animeFindMikanID(ctx, subjectID, keyword)
	if err != nil {
		return nil, "", err
	}
	if mikanID == "" {
		return nil, "", errAnimeUpstream("no Mikan series matches this anime")
	}

	// Animeko fetches this without a subgroup id. Adding `subgroupid=1` returns
	// an empty channel for most series, which looks like "no releases" rather
	// than the truth.
	feedURL := mikanAPIBase + "/RSS/Bangumi?bangumiId=" + mikanID
	body, err := animeUpstreamGet(ctx, feedURL, mikanHTMLHeaders(), mikanTimeout)
	if err != nil {
		return nil, "", err
	}
	return parseMikanFeed(body, "EXACT"), "EXACT", nil
}

// animeEpisodeExplicitPattern matches the unambiguous episode spellings:
// S01E02 and 第05话.
var animeEpisodeExplicitPattern = regexp.MustCompile(`(?i)(?:s\d+[._ ]?)?e(\d{1,4})|第\s*(\d{1,4})\s*[话話集]`)

// animeEpisodeRangePattern matches a batch range such as 01-28, which covers
// many episodes and therefore identifies a pack rather than one episode.
var animeEpisodeRangePattern = regexp.MustCompile(`\b\d{1,4}\s*[-–~]\s*\d{1,4}\b`)

// animeWordSplit splits a release title the way Animeko's splitWords does:
// bracket contents and the text between brackets both become words, and each is
// further split on separators. Episode detection then runs per word, which is
// what stops a number inside "[1080p]" from being read as an episode.
var animeWordSplit = regexp.MustCompile(`[[\]()（）【】「」『』\s_.,;:/\\|+*]+`)

// animeResolutionNumbers are bare numbers that name a resolution rather than an
// episode, so they are skipped when scanning words. Animeko excludes these for
// the same reason (its resolutionNumbers set); 720 is included here as well
// because a bare "720" is far more likely to be a resolution than episode 720.
var animeResolutionNumbers = map[int]bool{
	240: true, 360: true, 480: true, 560: true, 720: true,
	848: true, 1080: true, 1440: true, 1920: true, 2160: true,
}

// animeEpisodeNumberInTitle extracts the episode number a release covers, or 0
// when it covers none (a season pack) or several (a batch range).
func animeEpisodeNumberInTitle(title string) int {
	if matches := animeEpisodeExplicitPattern.FindStringSubmatch(title); matches != nil {
		for _, group := range matches[1:] {
			if group == "" {
				continue
			}
			if number, err := strconv.Atoi(group); err == nil && number > 0 {
				return number
			}
		}
	}

	// A range such as 01-28 spans many episodes, so the release is a pack. It is
	// reported as covering none rather than as its first episode, which would
	// otherwise make a whole-season batch claim episode 1.
	if animeEpisodeRangePattern.MatchString(title) {
		return 0
	}

	for _, word := range animeWordSplit.Split(title, -1) {
		word = strings.TrimSpace(word)
		number, err := strconv.Atoi(word)
		if err != nil {
			continue
		}
		if animeResolutionNumbers[number] {
			continue
		}
		// Years and other large standalone numbers are not episode numbers.
		if number < 1 || number > 999 {
			continue
		}
		return number
	}
	return 0
}

// animeMatchRelease picks the release covering an episode.
//
// A release covering exactly that episode wins. A season pack states no episode
// at all, so it is the fallback rather than a competitor -- otherwise a single
// 48GB batch would outrank a correct per-episode release on every episode.
// animeMatchRelease picks the release covering an episode.
//
// The candidates that name the episode are considered first, and a season pack
// only when none does. Within either group the choice is animeSelectRelease, so
// a browser-playable release wins and the resolution and language order decides
// among equals -- rather than the first candidate the feed happened to list,
// which is what this picked before.
func animeMatchRelease(candidates []animeRelease, episodeNumber int) *animeRelease {
	var named, packs []animeRelease
	for i := range candidates {
		release := candidates[i]
		number := animeEpisodeNumberInTitle(release.Title)
		switch {
		case number == episodeNumber:
			named = append(named, release)
		case number == 0:
			packs = append(packs, release)
		}
	}

	if len(named) > 0 {
		return animeSelectRelease(named)
	}
	return animeSelectRelease(packs)
}

// animePathID validates a numeric path parameter.
//
// These ids go into a cache key and an upstream path, so they are held to
// digits rather than trusted to arrive shaped correctly.
func animePathID(c *gin.Context, param, message string) (string, bool) {
	id := strings.TrimSpace(c.Param(param))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": message})
		return "", false
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": message})
			return "", false
		}
	}
	return id, true
}

// GetBangumiEpisodes returns the episode list for a bgm.tv subject.
//
// dandanplay indexes the same subject ids bgm.tv uses, so this is the bridge
// from the page's existing data onto everything else: one subject id in, the
// watchable episodes out.
func GetBangumiEpisodes(c *gin.Context) {
	subjectID, ok := animePathID(c, "id", "invalid subject id")
	if !ok {
		return
	}

	cacheKey := "bangumi:episodes:" + subjectID
	bangumiFetch(c, cacheKey, bangumiCacheTTL["episodes"], func() (string, error) {
		body, err := dandanplayGet(c.Request.Context(), "/api/v2/bangumi/bgmtv/"+subjectID, nil)
		if err != nil {
			return "", err
		}
		return animeEpisodesPayload(body)
	})
}

// GetBangumiDanmaku returns the comment payload for one episode.
//
// The comments are forwarded as-is rather than re-shaped: each `p` is a compact
// "time,mode,color,userHash" tuple that only the renderer can split
// meaningfully, and re-encoding it here would mean interpreting a value this
// side cannot use. The mode selects the rendering (1 scrolls right-to-left, 5
// pins to the bottom) and the colour is a packed RGB integer.
func GetBangumiDanmaku(c *gin.Context) {
	episodeID, ok := animePathID(c, "episodeId", "invalid episode id")
	if !ok {
		return
	}

	// The query is appended AFTER signing, never folded into the signed path.
	query := url.Values{"chConvert": {"0"}, "withRelated": {"true"}}
	cacheKey := "bangumi:danmaku:" + episodeID
	bangumiFetch(c, cacheKey, bangumiCacheTTL["danmaku"], func() (string, error) {
		body, err := dandanplayGet(c.Request.Context(), "/api/v2/comment/"+episodeID, query)
		if err != nil {
			return "", err
		}
		return animeDanmakuPayload(body)
	})
}

// GetBangumiReleases returns the releases backing one subject, with the chosen
// release attached to each episode.
//
// keyword is required because Mikan has no subject-id search: the page passes
// the title it already holds, and the mapping is then confirmed against
// Mikan's own page rather than assumed from the name.
func GetBangumiReleases(c *gin.Context) {
	subjectID, ok := animePathID(c, "subjectId", "invalid subject id")
	if !ok {
		return
	}
	keyword := strings.TrimSpace(c.Query("keyword"))
	if keyword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "keyword is required"})
		return
	}
	// Bounded for the same reason as the search keyword: it keys the cache and
	// travels upstream.
	if len(keyword) > maxBangumiKeywordBytes {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "keyword is too long"})
		return
	}

	cacheKey := "bangumi:releases:" + subjectID + ":" + url.QueryEscape(keyword)
	bangumiFetch(c, cacheKey, bangumiCacheTTL["releases"], func() (string, error) {
		releases, err := animeReleasesPayload(c, subjectID, keyword)
		if err != nil {
			return "", err
		}
		return releases, nil
	})
}

// animeEpisodesPayload reshapes dandanplay's season record into this page's
// episode shape.
//
// A failed lookup is an error rather than an empty list: "this anime has no
// episodes" and "the upstream failed" are different answers, and collapsing
// them would show a viewer an empty season for a transient outage.
func animeEpisodesPayload(body []byte) (string, error) {
	var response struct {
		Success bool               `json:"success"`
		Message string             `json:"errorMessage"`
		Bangumi *dandanplayBangumi `json:"bangumi"`
	}
	if err := common.Unmarshal(body, &response); err != nil {
		return "", err
	}
	if !response.Success || response.Bangumi == nil {
		message := response.Message
		if message == "" {
			message = "no episodes for this anime"
		}
		return "", errAnimeUpstream(message)
	}

	episodes := response.Bangumi.Episodes
	if len(episodes) > maxEpisodesPerSubject {
		episodes = episodes[:maxEpisodesPerSubject]
	}

	payload := struct {
		AnimeID    int64          `json:"animeId"`
		AnimeTitle string         `json:"animeTitle"`
		Episodes   []animeEpisode `json:"episodes"`
	}{
		AnimeID:    response.Bangumi.AnimeID,
		AnimeTitle: response.Bangumi.AnimeTitle,
		Episodes:   make([]animeEpisode, 0, len(episodes)),
	}
	for _, episode := range episodes {
		payload.Episodes = append(payload.Episodes, animeEpisode{
			EpisodeID:     episode.EpisodeID,
			EpisodeTitle:  episode.EpisodeTitle,
			EpisodeNumber: episode.EpisodeNumber,
			AirDate:       episode.AirDate,
		})
	}

	encoded, err := common.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// dandanplayBangumi is the part of the /bangumi/bgmtv payload the page uses.
type dandanplayBangumi struct {
	AnimeID    int64               `json:"animeId"`
	AnimeTitle string              `json:"animeTitle"`
	Episodes   []dandanplayEpisode `json:"episodes"`
}

type dandanplayEpisode struct {
	EpisodeID     int64  `json:"episodeId"`
	EpisodeTitle  string `json:"episodeTitle"`
	EpisodeNumber string `json:"episodeNumber"`
	AirDate       string `json:"airDate"`
}

// animeDanmakuPayload validates the comment response and returns it unchanged.
func animeDanmakuPayload(body []byte) (string, error) {
	// The comment endpoint is the one dandanplay resource that does not carry
	// the `success` flag the others do: it answers with `count` and `comments`
	// alone, so its shape is what gets checked. An episode with no comments is a
	// legitimate empty list rather than a failure, and the viewer renders it as
	// one.
	var response struct {
		Comments []json.RawMessage `json:"comments"`
	}
	if err := common.Unmarshal(body, &response); err != nil {
		return "", err
	}
	if response.Comments == nil {
		return "", errAnimeUpstream("malformed danmaku response")
	}
	return string(body), nil
}

// animeReleasesPayload resolves a subject to its releases and attaches one
// release per episode.
func animeReleasesPayload(c *gin.Context, subjectID, keyword string) (string, error) {
	releases, matchKind, err := animeFetchReleases(c.Request.Context(), subjectID, keyword)
	if err != nil {
		return "", err
	}

	subject, _ := strconv.ParseInt(subjectID, 10, 64)
	episodes, err := animeEpisodesForSubject(c, subjectID)
	if err != nil {
		// The releases are already fetched and usable on their own; a failed
		// episode lookup should not discard them.
		episodes = nil
	}

	for i := range episodes {
		number, convErr := strconv.Atoi(episodes[i].EpisodeNumber)
		if convErr != nil {
			continue
		}
		episodes[i].Release = animeMatchRelease(releases, number)
	}

	payload := struct {
		SubjectID int64          `json:"subjectId"`
		MatchKind string         `json:"matchKind"`
		Releases  []animeRelease `json:"releases"`
		Episodes  []animeEpisode `json:"episodes"`
	}{
		SubjectID: subject,
		MatchKind: matchKind,
		Releases:  animeOrderReleases(releases),
		Episodes:  episodes,
	}
	if payload.Episodes == nil {
		payload.Episodes = []animeEpisode{}
	}

	encoded, err := common.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// animeEpisodesForSubject reads the episode list so releases can be matched to
// episodes. It fetches dandanplay directly instead of reading the cache the
// episodes endpoint fills, so it costs one upstream call each time the caller's
// releases response is rebuilt.
func animeEpisodesForSubject(c *gin.Context, subjectID string) ([]animeEpisode, error) {
	body, err := dandanplayGet(c.Request.Context(), "/api/v2/bangumi/bgmtv/"+subjectID, nil)
	if err != nil {
		return nil, err
	}
	payload, err := animeEpisodesPayload(body)
	if err != nil {
		return nil, err
	}

	var decoded struct {
		Episodes []animeEpisode `json:"episodes"`
	}
	if err := common.UnmarshalJsonStr(payload, &decoded); err != nil {
		return nil, err
	}
	return decoded.Episodes, nil
}
