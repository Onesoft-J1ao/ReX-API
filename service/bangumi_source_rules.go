package service

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"html"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/QuantumNous/new-api/common"
	"github.com/andybalholm/cascadia"
	"github.com/dlclark/regexp2/v2"
)

type bangumiSourceConfig struct {
	SearchURL                  string `json:"searchUrl"`
	RawBaseURL                 string `json:"rawBaseUrl"`
	RequestInterval            int64  `json:"requestInterval"`
	SearchCacheTTL             int64  `json:"searchCacheTtl"`
	SearchUseOnlyFirstWord     bool   `json:"searchUseOnlyFirstWord"`
	SearchRemoveSpecial        bool   `json:"searchRemoveSpecial"`
	SearchUseSubjectNamesCount int    `json:"searchUseSubjectNamesCount"`
	SubjectFormatID            string `json:"subjectFormatId"`
	SubjectA                   struct {
		SelectLists       string `json:"selectLists"`
		PreferShorterName bool   `json:"preferShorterName"`
	} `json:"selectorSubjectFormatA"`
	SubjectIndexed struct {
		SelectNames       string `json:"selectNames"`
		SelectLinks       string `json:"selectLinks"`
		PreferShorterName bool   `json:"preferShorterName"`
	} `json:"selectorSubjectFormatIndexed"`
	ChannelFormatID string `json:"channelFormatId"`
	ChannelGrouped  struct {
		SelectChannelNames         string `json:"selectChannelNames"`
		MatchChannelName           string `json:"matchChannelName"`
		SelectEpisodeLists         string `json:"selectEpisodeLists"`
		SelectEpisodesFromList     string `json:"selectEpisodesFromList"`
		SelectEpisodeLinksFromList string `json:"selectEpisodeLinksFromList"`
		MatchEpisodeSortFromName   string `json:"matchEpisodeSortFromName"`
	} `json:"selectorChannelFormatFlattened"`
	ChannelNone struct {
		SelectEpisodes           string `json:"selectEpisodes"`
		SelectEpisodeLinks       string `json:"selectEpisodeLinks"`
		MatchEpisodeSortFromName string `json:"matchEpisodeSortFromName"`
	} `json:"selectorChannelFormatNoChannel"`
	MatchVideo struct {
		EnableNestedURL      bool   `json:"enableNestedUrl"`
		MatchNestedURL       string `json:"matchNestedUrl"`
		MatchVideoURL        string `json:"matchVideoUrl"`
		ScanDOMMediaURLs     bool   `json:"scanDomMediaUrls"`
		ScanInlineScriptURLs bool   `json:"scanInlineScriptUrls"`
		Cookies              string `json:"cookies"`
		AddHeadersToVideo    struct {
			Referer   string `json:"referer"`
			UserAgent string `json:"userAgent"`
		} `json:"addHeadersToVideo"`
	} `json:"matchVideo"`
	DefaultResolution       string                  `json:"defaultResolution"`
	DefaultSubtitleLanguage string                  `json:"defaultSubtitleLanguage"`
	OnlySupportsPlayers     []string                `json:"onlySupportsPlayers"`
	FilterByEpisodeSort     bool                    `json:"filterByEpisodeSort"`
	FilterBySubjectName     bool                    `json:"filterBySubjectName"`
	SelectMedia             json.RawMessage         `json:"selectMedia"`
	AutoMatch               *bangumiSourceAutoMatch `json:"autoMatch"`
}

type bangumiSourceAutoMatch struct {
	Enabled                    bool `json:"enabled"`
	SearchUseOnlyFirstWord     bool `json:"searchUseOnlyFirstWord"`
	SearchRemoveSpecial        bool `json:"searchRemoveSpecial"`
	SearchUseSubjectNamesCount int  `json:"searchUseSubjectNamesCount"`
	PreferShorterName          bool `json:"preferShorterName"`
	FilterByEpisodeSort        bool `json:"filterByEpisodeSort"`
	FilterBySubjectName        bool `json:"filterBySubjectName"`
}

func (a *bangumiSourceAutoMatch) UnmarshalJSON(data []byte) error {
	type plain bangumiSourceAutoMatch
	*a = bangumiSourceAutoMatch{Enabled: true, SearchUseOnlyFirstWord: true, SearchRemoveSpecial: true, SearchUseSubjectNamesCount: 1, PreferShorterName: true, FilterByEpisodeSort: true, FilterBySubjectName: true}
	return common.Unmarshal(data, (*plain)(a))
}

// Decode into defaults so omitted and explicitly empty regexes remain distinct.
// selectorChannelFormatFlattened is the legacy key for index-grouped, not a
// flattened playlist algorithm. autoMatch, when present, owns the old flat keys.
func (c *bangumiSourceConfig) UnmarshalJSON(data []byte) error {
	type plain bangumiSourceConfig
	*c = bangumiSourceConfig{RequestInterval: 3000, SearchCacheTTL: int64(2 * time.Hour / time.Millisecond), SubjectFormatID: "a", ChannelFormatID: "no-channel", SearchUseOnlyFirstWord: true, SearchRemoveSpecial: true, SearchUseSubjectNamesCount: 1, DefaultResolution: "1080P", DefaultSubtitleLanguage: "CHS", FilterByEpisodeSort: true, FilterBySubjectName: true}
	c.SubjectA.SelectLists, c.SubjectA.PreferShorterName = "div.video-info-header > a", true
	c.SubjectIndexed.SelectNames, c.SubjectIndexed.SelectLinks, c.SubjectIndexed.PreferShorterName = ".search-box .thumb-content > .thumb-txt", ".search-box .thumb-menu > a", true
	c.ChannelGrouped.SelectChannelNames = ".anthology-tab > .swiper-wrapper a"
	c.ChannelGrouped.MatchChannelName = `^(?<ch>.+?)(\d+)?$`
	c.ChannelGrouped.SelectEpisodeLists, c.ChannelGrouped.SelectEpisodesFromList = ".anthology-list-box", "a"
	c.ChannelGrouped.MatchEpisodeSortFromName = `第\s*(?<ep>.+)\s*[话集]`
	c.ChannelNone.SelectEpisodes = "#glist-1 > div.module-blocklist.scroll-box.scroll-box-y > div > a"
	c.ChannelNone.MatchEpisodeSortFromName = `第\s*(?<ep>.+)\s*[话集]`
	c.MatchVideo.EnableNestedURL = true
	c.MatchVideo.MatchNestedURL = `^.+(m3u8|vip|xigua\.php).+\?`
	c.MatchVideo.MatchVideoURL = `(^http(s)?:\/\/(?!.*http(s)?:\/\/).+((\.mp4)|(\.mkv)|(m3u8)).*(\?.+)?)|(akamaized)|(bilivideo.com)`
	c.MatchVideo.Cookies = "quality=1080"
	c.MatchVideo.AddHeadersToVideo.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537.3"
	if err := common.Unmarshal(data, (*plain)(c)); err != nil {
		return err
	}
	if c.AutoMatch != nil {
		c.SearchUseOnlyFirstWord, c.SearchRemoveSpecial, c.SearchUseSubjectNamesCount = c.AutoMatch.SearchUseOnlyFirstWord, c.AutoMatch.SearchRemoveSpecial, c.AutoMatch.SearchUseSubjectNamesCount
		c.FilterByEpisodeSort, c.FilterBySubjectName = c.AutoMatch.FilterByEpisodeSort, c.AutoMatch.FilterBySubjectName
	} else {
		preferShorter := c.SubjectA.PreferShorterName
		if c.SubjectFormatID == "indexed" {
			preferShorter = c.SubjectIndexed.PreferShorterName
		}
		c.AutoMatch = &bangumiSourceAutoMatch{Enabled: true, SearchUseOnlyFirstWord: c.SearchUseOnlyFirstWord, SearchRemoveSpecial: c.SearchRemoveSpecial, SearchUseSubjectNamesCount: c.SearchUseSubjectNamesCount, PreferShorterName: preferShorter, FilterByEpisodeSort: c.FilterByEpisodeSort, FilterBySubjectName: c.FilterBySubjectName}
	}
	return nil
}

type bangumiSourceRules struct {
	sourceID                                           string
	factory                                            string
	config                                             bangumiSourceConfig
	base                                               *url.URL
	subjects, subjectNames, subjectLinks               cascadia.Selector
	channelNames, episodeLists, episodes, episodeLinks cascadia.Selector
	channelName, episodeNumber, nestedURL, videoURL    *regexp2.Regexp
}

func compileBangumiCSS(rule string, optional bool) (cascadia.Selector, error) {
	if rule == "" && optional {
		return nil, nil
	}
	if rule == "" || len(rule) > 2048 {
		return nil, errors.New("Missing or oversized CSS selector")
	}
	// Jsoup accepts unquoted attribute values such as [href^=#playlist].
	// CSS engines require these particular values to be quoted.
	rule = bangumiUnquotedAttribute.ReplaceAllString(rule, `[$1"$2"]`)
	selector, err := cascadia.Compile(rule)
	if err != nil {
		return nil, errors.New("Invalid CSS selector in source configuration")
	}
	return selector, nil
}

var bangumiUnquotedAttribute = regexp.MustCompile(`\[([\w-]+(?:[\^$*|~]?=))([^\]"'\s]+)\]`)

func compileBangumiRegex(rule string) (*regexp2.Regexp, error) {
	if len(rule) > 8192 {
		return nil, errors.New("Oversized source regex")
	}
	if rule == "" {
		return nil, nil
	}
	compiled, err := regexp2.Compile(rule)
	if err != nil {
		return nil, errors.New("Invalid regex in source configuration")
	}
	compiled.MatchTimeout = 100 * time.Millisecond
	return compiled, nil
}

func compileBangumiSourceRules(d bangumiSourceDefinition) (*bangumiSourceRules, error) {
	r := &bangumiSourceRules{sourceID: d.MediaSourceID, factory: d.FactoryID, config: d.SearchConfig}
	invalid := func(err error) (*bangumiSourceRules, error) { return r, err }
	switch d.FactoryID {
	case "mikan-mikanime-tv":
		if d.Kind != "bt" {
			return invalid(errors.New("Mikan must have BT kind"))
		}
		return r, nil
	case "web-selector":
		if d.Kind != "web" {
			return invalid(errors.New("Selector source must have web kind"))
		}
	case "rss", "dmhy":
		if d.Kind != "bt" {
			return invalid(errors.New("RSS and dmhy must have BT kind"))
		}
	default:
		return invalid(errors.New("Unsupported media source factory"))
	}
	c := r.config
	if !strings.Contains(c.SearchURL, "{keyword}") {
		return invalid(errors.New("Search URL must contain {keyword}"))
	}
	if c.RequestInterval < 0 || c.RequestInterval > 60000 || c.SearchCacheTTL < 0 {
		return invalid(errors.New("Invalid request interval or cache TTL"))
	}
	base, err := bangumiHTTPURL(nil, strings.ReplaceAll(c.SearchURL, "{keyword}", "test"))
	if err != nil {
		return invalid(errors.New("Invalid source search URL"))
	}
	base.Path, base.RawPath, base.RawQuery, base.Fragment = "/", "", "", ""
	if c.RawBaseURL != "" {
		base, err = bangumiHTTPURL(nil, c.RawBaseURL)
		if err != nil {
			return invalid(errors.New("Invalid source base URL"))
		}
	}
	r.base = base
	for _, header := range []string{c.MatchVideo.Cookies, c.MatchVideo.AddHeadersToVideo.UserAgent, c.MatchVideo.AddHeadersToVideo.Referer} {
		if len(header) > 1024 || strings.ContainsAny(header, "\r\n\x00") {
			return invalid(errors.New("Invalid source header configuration"))
		}
	}
	// The imported snapshot contains preference cookies only. Do not let a
	// replacement catalog smuggle authentication state into this service.
	if c.MatchVideo.Cookies != "" {
		name, value, ok := strings.Cut(c.MatchVideo.Cookies, "=")
		if !ok || name != "quality" || value == "" || strings.ContainsAny(value, "; \t") {
			return invalid(errors.New("Only a single quality preference cookie is supported"))
		}
	}
	if c.MatchVideo.AddHeadersToVideo.Referer != "" {
		if _, err := bangumiHTTPURL(nil, c.MatchVideo.AddHeadersToVideo.Referer); err != nil {
			return invalid(errors.New("Invalid video Referer configuration"))
		}
	}
	if d.FactoryID != "web-selector" {
		r.config.MatchVideo.Cookies = ""
		return r, nil
	}
	switch c.SubjectFormatID {
	case "a":
		r.subjects, err = compileBangumiCSS(c.SubjectA.SelectLists, false)
	case "indexed":
		r.subjectNames, err = compileBangumiCSS(c.SubjectIndexed.SelectNames, false)
		if err == nil {
			r.subjectLinks, err = compileBangumiCSS(c.SubjectIndexed.SelectLinks, false)
		}
	default:
		err = errors.New("Unsupported subject format in source configuration")
	}
	if err != nil {
		return invalid(err)
	}
	var episodePattern string
	switch c.ChannelFormatID {
	case "index-grouped":
		r.channelNames, err = compileBangumiCSS(c.ChannelGrouped.SelectChannelNames, false)
		if err == nil {
			r.episodeLists, err = compileBangumiCSS(c.ChannelGrouped.SelectEpisodeLists, false)
		}
		if err == nil {
			r.episodes, err = compileBangumiCSS(c.ChannelGrouped.SelectEpisodesFromList, false)
		}
		if err == nil {
			r.episodeLinks, err = compileBangumiCSS(c.ChannelGrouped.SelectEpisodeLinksFromList, true)
		}
		if err == nil {
			r.channelName, err = compileBangumiRegex(c.ChannelGrouped.MatchChannelName)
		}
		episodePattern = c.ChannelGrouped.MatchEpisodeSortFromName
	case "no-channel":
		r.episodes, err = compileBangumiCSS(c.ChannelNone.SelectEpisodes, false)
		if err == nil {
			r.episodeLinks, err = compileBangumiCSS(c.ChannelNone.SelectEpisodeLinks, true)
		}
		episodePattern = c.ChannelNone.MatchEpisodeSortFromName
	default:
		err = errors.New("Unsupported channel format in source configuration")
	}
	if err == nil {
		r.episodeNumber, err = compileBangumiRegex(episodePattern)
	}
	if err == nil {
		r.nestedURL, err = compileBangumiRegex(c.MatchVideo.MatchNestedURL)
	}
	if err == nil {
		r.videoURL, err = compileBangumiRegex(c.MatchVideo.MatchVideoURL)
	}
	if err != nil {
		return invalid(err)
	}
	return r, nil
}

func (r *bangumiSourceRules) cacheTTL() time.Duration {
	// Records must outlive a zero-TTL search so its opaque IDs can be browsed.
	if r.config.SearchCacheTTL == 0 {
		return bangumiSourceTTL
	}
	return time.Duration(min(r.config.SearchCacheTTL, int64(bangumiSourceTTL/time.Millisecond))) * time.Millisecond
}

var bangumiSearchSpecial = regexp.MustCompile(`[\p{P}\p{S}]+`)

func (r *bangumiSourceRules) searchURL(keyword string) (string, error) {
	if r.factory == "web-selector" {
		if r.config.SearchRemoveSpecial {
			keyword = strings.ReplaceAll(strings.ReplaceAll(keyword, "剧场版", ""), "劇場版", "")
			keyword = bangumiSearchSpecial.ReplaceAllString(keyword, " ")
		}
		if r.config.SearchUseOnlyFirstWord {
			words := strings.Fields(keyword)
			if len(words) > 0 {
				keyword = words[0]
			}
		}
	}
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return "", bangumiSourceFailure(r.sourceID, BangumiSourceFailed, "Search keyword is empty after source normalization", nil)
	}
	// Encode reserved query characters as well as path separators. A catalog
	// placeholder may occur in either a path segment or a query value.
	encodedKeyword := strings.ReplaceAll(url.QueryEscape(keyword), "+", "%20")
	location := strings.ReplaceAll(r.config.SearchURL, "{keyword}", encodedKeyword)
	parsed, err := bangumiHTTPURL(nil, location)
	if err != nil {
		return "", bangumiSourceFailure(r.sourceID, BangumiSourceInvalidConfig, "Invalid source search URL", nil)
	}
	return parsed.String(), nil
}

// findText implements the selector engine's find + named-group-or-full-label
// semantics. Neither episode numbers nor Chinese/special labels are numeric.
func (r *bangumiSourceRules) findText(ctx context.Context, expression *regexp2.Regexp, text, groupName string) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	if len(text) > 32768 {
		return "", false, bangumiSourceFailure(r.sourceID, BangumiSourceFailed, "Source regex input exceeds limit", nil)
	}
	if expression == nil {
		return text, true, nil
	}
	match, err := expression.FindStringMatch(text)
	if err != nil {
		return "", false, bangumiSourceFailure(r.sourceID, BangumiSourceInvalidConfig, "Source regex timed out or could not be evaluated", nil)
	}
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	if match == nil {
		return "", false, nil
	}
	if group := match.GroupByName(groupName); group != nil && len(group.Captures) > 0 {
		return group.String(), true, nil
	}
	return text, true, nil
}

type bangumiParsedSubject struct {
	name, url string
	torrent   BangumiSourceTorrentRecord
}
type bangumiParsedEpisode struct {
	name, number, url string
	torrent           BangumiSourceTorrentRecord
}
type bangumiParsedChannel struct {
	ordinal     int
	name, label string
	episodes    []bangumiParsedEpisode
}

func bangumiHTMLDocument(body []byte) (*goquery.Document, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var decoded string
		if err := common.Unmarshal(trimmed, &decoded); err == nil {
			trimmed = []byte(decoded)
		}
	}
	if !bytes.Contains(trimmed, []byte("<")) {
		return nil, errors.New("Upstream did not return an HTML page")
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(trimmed))
	if err != nil {
		return nil, errors.New("Cannot parse upstream HTML")
	}
	return doc, nil
}

func (r *bangumiSourceRules) search(ctx context.Context, page bangumiSourcePage) ([]bangumiParsedSubject, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.factory == "rss" {
		return r.searchRSS(page)
	}
	doc, err := bangumiHTMLDocument(page.body)
	if err != nil {
		return nil, err
	}
	if r.factory == "dmhy" {
		return r.searchDMHY(doc, page.url)
	}
	var names, links *goquery.Selection
	if r.config.SubjectFormatID == "a" {
		names = doc.FindMatcher(r.subjects)
		links = names
	} else {
		names = doc.FindMatcher(r.subjectNames)
		links = doc.FindMatcher(r.subjectLinks)
	}
	results := make([]bangumiParsedSubject, 0, min(names.Length(), bangumiSourceResultLimit))
	for i := range min(names.Length(), links.Length(), bangumiSourceResultLimit) {
		name := strings.TrimSpace(names.Eq(i).Text())
		if r.config.SubjectFormatID == "a" {
			if title, _ := names.Eq(i).Attr("title"); strings.TrimSpace(title) != "" {
				name = strings.TrimSpace(title)
			}
		}
		href, _ := links.Eq(i).Attr("href")
		if name == "" || href == "" {
			continue
		}
		location, err := bangumiHTTPURL(r.base, html.UnescapeString(href))
		if err != nil {
			return nil, errors.New("Search results contain malformed or unsafe subject links")
		}
		results = append(results, bangumiParsedSubject{name: bangumiDisplayText(name), url: location.String()})
	}
	return results, nil
}

func bangumiDisplayText(text string) string {
	text = strings.TrimSpace(text)
	runes := []rune(text)
	if len(runes) > 512 {
		return string(runes[:512])
	}
	return strings.Clone(text)
}

func (r *bangumiSourceRules) browse(ctx context.Context, page bangumiSourcePage) ([]bangumiParsedChannel, error) {
	doc, err := bangumiHTMLDocument(page.body)
	if err != nil {
		return nil, err
	}
	var names, lists *goquery.Selection
	if r.config.ChannelFormatID == "no-channel" {
		lists = doc.Selection
	} else {
		names, lists = doc.FindMatcher(r.channelNames), doc.FindMatcher(r.episodeLists)
	}
	groups := []bangumiParsedChannel{}
	count := lists.Length()
	if names != nil {
		count = min(count, names.Length())
	}
	total := 0
	for i := range min(count, 32) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		group := bangumiParsedChannel{ordinal: i, episodes: []bangumiParsedEpisode{}}
		if names != nil {
			group.label = strings.TrimSpace(names.Eq(i).Text())
			if group.label == "" {
				continue
			}
			var matched bool
			group.name, matched, err = r.findText(ctx, r.channelName, group.label, "ch")
			if err != nil {
				return nil, err
			}
			if !matched {
				continue
			}
		}
		list := lists.Eq(i)
		episodes := list.FindMatcher(r.episodes)
		links := episodes
		if r.episodeLinks != nil {
			links = list.FindMatcher(r.episodeLinks)
		}
		for j := range min(episodes.Length(), links.Length(), bangumiSourceEpisodeLimit-total) {
			name := strings.TrimSpace(episodes.Eq(j).Text())
			href, _ := links.Eq(j).Attr("href")
			if name == "" || href == "" {
				continue
			}
			location, err := bangumiHTTPURL(page.url, html.UnescapeString(href))
			if err != nil {
				return nil, errors.New("Episode list contains malformed or unsafe links")
			}
			number, matched, err := r.findText(ctx, r.episodeNumber, name, "ep")
			if err != nil {
				return nil, err
			}
			if !matched {
				number = name
			}
			group.episodes = append(group.episodes, bangumiParsedEpisode{name: bangumiDisplayText(name), number: bangumiDisplayText(number), url: location.String()})
			total++
		}
		group.name, group.label = bangumiDisplayText(group.name), bangumiDisplayText(group.label)
		groups = append(groups, group)
		if total >= bangumiSourceEpisodeLimit {
			break
		}
	}
	return groups, nil
}

func (r *bangumiSourceRules) searchDMHY(doc *goquery.Document, base *url.URL) ([]bangumiParsedSubject, error) {
	table := doc.Find("table.tablesorter")
	if table.Length() != 1 {
		return nil, errors.New("dmhy response is missing its release table")
	}
	rows := table.Find("tbody > tr")
	results := []bangumiParsedSubject{}
	for i := range min(rows.Length(), bangumiSourceResultLimit) {
		cells := rows.Eq(i).ChildrenFiltered("td")
		if cells.Length() < 4 {
			return nil, errors.New("dmhy release table has an incomplete row")
		}
		link := cells.Eq(2).Find("a").Last()
		name := strings.TrimSpace(link.Text())
		href, _ := link.Attr("href")
		origin, err := bangumiHTTPURL(base, href)
		if err != nil || name == "" {
			return nil, errors.New("dmhy release row has an invalid title or origin")
		}
		magnet, _ := cells.Eq(3).Find("a").First().Attr("href")
		if !bangumiValidMagnet(magnet) {
			return nil, errors.New("dmhy release row has no supported magnet location")
		}
		results = append(results, bangumiParsedSubject{name: bangumiDisplayText(name), url: origin.String(), torrent: BangumiSourceTorrentRecord{OriginURL: origin.String(), Magnet: magnet}})
	}
	return results, nil
}

type bangumiRSSItem struct {
	Title      string `xml:"title"`
	Link       string `xml:"link"`
	GUID       string `xml:"guid"`
	Enclosures []struct {
		URL  string `xml:"url,attr"`
		Type string `xml:"type,attr"`
	} `xml:"enclosure"`
	MagnetURI string `xml:"magnetURI"`
	InfoHash  string `xml:"infoHash"`
}

func bangumiValidMagnet(raw string) bool {
	if len(raw) > 8192 {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "magnet" || parsed.User != nil {
		return false
	}
	for _, xt := range parsed.Query()["xt"] {
		if strings.HasPrefix(xt, "urn:btih:") && len(strings.TrimPrefix(xt, "urn:btih:")) >= 32 {
			return true
		}
	}
	return false
}

func (r *bangumiSourceRules) searchRSS(page bangumiSourcePage) ([]bangumiParsedSubject, error) {
	var feed struct {
		XMLName xml.Name
		Channel struct {
			Items []bangumiRSSItem `xml:"item"`
		} `xml:"channel"`
	}
	if err := xml.Unmarshal(page.body, &feed); err != nil || feed.XMLName.Local != "rss" {
		return nil, errors.New("Upstream returned an invalid RSS feed")
	}
	results := []bangumiParsedSubject{}
	for i, item := range feed.Channel.Items {
		if i >= bangumiSourceResultLimit {
			break
		}
		if strings.TrimSpace(item.Title) == "" {
			return nil, errors.New("RSS release is missing its title")
		}
		record := BangumiSourceTorrentRecord{}
		if bangumiValidMagnet(item.Link) {
			record.Magnet = item.Link
		} else if item.Link != "" {
			origin, err := bangumiHTTPURL(page.url, item.Link)
			if err != nil {
				return nil, errors.New("RSS release has an unsafe origin link")
			}
			record.OriginURL = origin.String()
			if strings.HasSuffix(strings.ToLower(origin.Path), ".torrent") {
				record.TorrentURL = origin.String()
			}
		}
		for _, enclosure := range item.Enclosures {
			if bangumiValidMagnet(enclosure.URL) {
				record.Magnet = enclosure.URL
				continue
			}
			location, err := bangumiHTTPURL(page.url, enclosure.URL)
			if err != nil {
				return nil, errors.New("RSS release has an unsafe enclosure")
			}
			if strings.HasSuffix(strings.ToLower(location.Path), ".torrent") || enclosure.Type == "application/x-bittorrent" {
				record.TorrentURL = location.String()
			}
		}
		if bangumiValidMagnet(item.MagnetURI) {
			record.Magnet = item.MagnetURI
		}
		if record.Magnet == "" && bangumiValidMagnet(item.GUID) {
			record.Magnet = item.GUID
		}
		if record.Magnet == "" && len(item.InfoHash) == 40 && bangumiHexHash.MatchString(item.InfoHash) {
			record.Magnet = "magnet:?xt=urn:btih:" + item.InfoHash
		}
		if record.Magnet == "" && record.TorrentURL == "" {
			return nil, errors.New("RSS release has no supported torrent or magnet location")
		}
		location := record.OriginURL
		if location == "" {
			location = record.TorrentURL
		}
		if location == "" {
			location = record.Magnet
		}
		results = append(results, bangumiParsedSubject{name: bangumiDisplayText(item.Title), url: location, torrent: record})
	}
	return results, nil
}

var bangumiHexHash = regexp.MustCompile(`^[a-fA-F0-9]{40}$`)
