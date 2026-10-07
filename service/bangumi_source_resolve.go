package service

import (
	"context"
	"encoding/base64"
	"errors"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

const (
	bangumiResolveDepthLimit     = 3
	bangumiResolveRequestLimit   = 6
	bangumiResolveCandidateLimit = 32
)

type bangumiStaticCandidate struct {
	url          string
	referer      string
	depth        int
	mediaElement bool
}

func (m *BangumiSourceManager) resolveStatic(ctx context.Context, source *bangumiSource, initialURL string) (BangumiSourceResolveResult, error) {
	id := source.definition.MediaSourceID
	result := BangumiSourceResolveResult{BangumiSourceStatus: BangumiSourceStatus{SourceID: id, State: BangumiSourceWorking}, Format: "unknown"}
	queue := []bangumiStaticCandidate{{url: initialURL}}
	visited := make(map[string]bool)
	requests := 0
	bounded := false
	var lastFailure error
	// A candidate the network cannot reach at all -- a placeholder host printed
	// in the page markup, a dead mirror -- is not evidence that another
	// candidate is unplayable, so it is recorded and skipped. A rate limit is
	// the site's answer about this request rather than about the candidate, so
	// it still ends the attempt.
	rateLimited := func(err error) bool {
		var typed *BangumiSourceError
		return errors.As(err, &typed) && typed.State == BangumiSourceRateLimited
	}
	for len(queue) > 0 && requests < bangumiResolveRequestLimit {
		candidate := queue[0]
		queue = queue[1:]
		if visited[candidate.url] {
			continue
		}
		visited[candidate.url] = true
		if err := ctx.Err(); err != nil {
			return result, m.resultError(&result.BangumiSourceStatus, id, err)
		}
		if candidate.depth > bangumiResolveDepthLimit {
			bounded = true
			continue
		}
		nested := false
		if source.rules.config.MatchVideo.EnableNestedURL && source.rules.nestedURL != nil {
			_, nestedMatch, err := source.rules.findText(ctx, source.rules.nestedURL, candidate.url, "v")
			if err != nil {
				return result, m.resultError(&result.BangumiSourceStatus, id, err)
			}
			nested = nestedMatch
		}
		videoURL, matchesVideo, err := source.rules.findText(ctx, source.rules.videoURL, candidate.url, "v")
		if err != nil {
			return result, m.resultError(&result.BangumiSourceStatus, id, err)
		}
		if source.rules.videoURL == nil {
			matchesVideo = candidate.mediaElement || bangumiMediaFormat(candidate.url, "") != "unknown"
		}
		if !nested && matchesVideo {
			location, urlErr := bangumiHTTPURL(nil, decodeBangumiLiteralURL(videoURL))
			if urlErr != nil {
				lastFailure = bangumiSourceFailure(id, BangumiSourceFailed, "Player returned an unsafe or malformed media location", nil)
				continue
			}
			// Probe through the host's guarded client rather than accepting a
			// discovered URL that has only passed a syntactic parser check.
			requests++
			page, fetchErr := m.fetch(ctx, source, location.String(), candidate.referer, http.MethodHead, false)
			if fetchErr != nil {
				if rateLimited(fetchErr) {
					return result, m.resultError(&result.BangumiSourceStatus, id, fetchErr)
				}
				lastFailure = fetchErr
				continue
			}
			format, verified := bangumiVerifiedMedia(page)
			if !verified && (page.status == http.StatusForbidden || page.status == http.StatusMethodNotAllowed || page.status == http.StatusNotImplemented || (page.status < 400 && !strings.Contains(strings.ToLower(page.contentType), "text/html"))) {
				if requests >= bangumiResolveRequestLimit {
					bounded = true
					break
				}
				requests++
				page, fetchErr = m.fetch(ctx, source, location.String(), candidate.referer, http.MethodGet, true)
				if fetchErr != nil {
					if rateLimited(fetchErr) {
						return result, m.resultError(&result.BangumiSourceStatus, id, fetchErr)
					}
					lastFailure = fetchErr
					continue
				}
				format, verified = bangumiVerifiedMedia(page)
			}
			if verified {
				result.State, result.VideoURL, result.Format = BangumiSourceReady, page.url.String(), format
				result.Headers = m.requestHeaders(source, page.url, candidate.referer)
				if referer := source.rules.config.MatchVideo.AddHeadersToVideo.Referer; referer != "" {
					result.Headers.Set("Referer", referer)
				}
				return result, nil
			}
			if page.status < 200 || page.status >= 300 {
				lastFailure = bangumiPageFailure(id, page, nil)
				var typed *BangumiSourceError
				if errors.As(lastFailure, &typed) && typed.State == BangumiSourceCaptchaRequired {
					return result, m.resultError(&result.BangumiSourceStatus, id, lastFailure)
				}
				continue
			}
			// A generic binary response is not proof that a .mp4-named URL is
			// playable. HTML player wrappers can still be parsed statically.
			if !strings.Contains(strings.ToLower(page.contentType), "text/html") {
				continue
			}
			candidate.url = page.url.String()
		}
		if requests >= bangumiResolveRequestLimit {
			bounded = true
			break
		}
		requests++
		page, fetchErr := m.fetch(ctx, source, candidate.url, candidate.referer, http.MethodGet, false)
		if fetchErr != nil {
			if rateLimited(fetchErr) {
				return result, m.resultError(&result.BangumiSourceStatus, id, fetchErr)
			}
			lastFailure = fetchErr
			continue
		}
		children, parseErr := source.rules.staticCandidates(page)
		if len(children) == 0 || parseErr != nil || page.status < 200 || page.status >= 300 {
			if failure := bangumiPageFailure(id, page, parseErr); failure != nil {
				var typed *BangumiSourceError
				if errors.As(failure, &typed) && (typed.State == BangumiSourceCaptchaRequired || typed.State == BangumiSourceRateLimited) {
					return result, m.resultError(&result.BangumiSourceStatus, id, failure)
				}
				lastFailure = failure
			}
			continue
		}
		for _, child := range children {
			child.referer, child.depth = page.url.String(), candidate.depth+1
			if child.depth > bangumiResolveDepthLimit {
				bounded = true
				continue
			}
			if len(queue) >= bangumiResolveCandidateLimit {
				bounded = true
				break
			}
			queue = append(queue, child)
		}
	}
	if requests >= bangumiResolveRequestLimit && len(queue) > 0 {
		bounded = true
	}
	if lastFailure != nil {
		var typed *BangumiSourceError
		if errors.As(lastFailure, &typed) { // A candidate that failed on its own merits -- rejected upstream, an
			// unreadable page, a broken rule -- is the real reason no media was
			// found, so it is reported as-is. Rewriting it to "needs a browser"
			// told the viewer the source wanted JavaScript when it had actually
			// refused the request.
			return result, m.resultError(&result.BangumiSourceStatus, id, lastFailure)
		}
	}
	message := "Static player parsing found no media URL; browser JavaScript or an unsupported player adapter is required"
	if bounded {
		message = "Static player resolution reached its depth or request limit; a browser or player adapter is required"
	}
	err := bangumiSourceFailure(id, BangumiSourceBrowserRequired, message, nil)
	result.BangumiSourceStatus = err.BangumiSourceStatus
	return result, err
}

func bangumiVerifiedMedia(page bangumiSourcePage) (string, bool) {
	if page.status < 200 || page.status >= 300 {
		return "unknown", false
	}
	mediaType, _, _ := strings.Cut(strings.ToLower(page.contentType), ";")
	mediaType = strings.TrimSpace(mediaType)
	if strings.Contains(mediaType, "mpegurl") {
		return "hls", true
	}
	if mediaType == "video/mp4" {
		return "mp4", true
	}
	if strings.Contains(mediaType, "matroska") {
		return "matroska", true
	}
	if strings.HasPrefix(mediaType, "video/") {
		return "unknown", true
	}
	prefix := strings.TrimSpace(strings.TrimPrefix(string(page.body), "\xef\xbb\xbf"))
	if strings.HasPrefix(prefix, "#EXTM3U") {
		return "hls", true
	}
	if len(page.body) >= 12 && string(page.body[4:8]) == "ftyp" {
		return "mp4", true
	}
	if len(page.body) >= 4 && string(page.body[:4]) == "\x1a\x45\xdf\xa3" {
		return "matroska", true
	}
	return "unknown", false
}

func bangumiMediaFormat(rawURL, contentType string) string {
	location, _ := url.Parse(rawURL)
	path := ""
	if location != nil {
		path = strings.ToLower(location.Path)
	}
	contentType = strings.ToLower(contentType)
	switch {
	case strings.HasSuffix(path, ".m3u8"), strings.Contains(contentType, "mpegurl"):
		return "hls"
	case strings.HasSuffix(path, ".mp4"), strings.Contains(contentType, "video/mp4"):
		return "mp4"
	case strings.HasSuffix(path, ".mkv"), strings.Contains(contentType, "matroska"):
		return "matroska"
	default:
		return "unknown"
	}
}

// These RE2 patterns recognize literals, not JavaScript programs. Expressions,
// functions, concatenation and external scripts are never evaluated or fetched.
var (
	bangumiPlayerObject   = regexp.MustCompile(`(?i)\b(?:player_[a-z0-9_]+|player|config)\s*=\s*(\{[^{}]*\})`)
	bangumiPlayerURLField = regexp.MustCompile(`(?i)(?:["']?(?:url|file|src)["']?)\s*[:=]\s*("(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*')\s*([,};\)\]])`)
	bangumiEncryptField   = regexp.MustCompile(`(?i)["']?encrypt["']?\s*:\s*["']?([012])["']?`)
	bangumiQuotedLiteral  = regexp.MustCompile(`"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'`)
	bangumiAtobLiteral    = regexp.MustCompile(`\batob\(\s*("(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*')\s*\)`)
)

func (r *bangumiSourceRules) staticCandidates(page bangumiSourcePage) ([]bangumiStaticCandidate, error) {
	doc, err := bangumiHTMLDocument(page.body)
	if err != nil {
		return nil, err
	}
	results := []bangumiStaticCandidate{}
	seen := make(map[string]bool)
	add := func(raw string, mediaElement bool) error {
		if len(results) >= bangumiResolveCandidateLimit {
			return nil
		}
		raw = decodeBangumiLiteralURL(raw)
		location, err := bangumiHTTPURL(page.url, raw)
		if err != nil {
			return errors.New("Player markup contains an unsafe or malformed upstream location")
		}
		if !seen[location.String()] {
			seen[location.String()] = true
			results = append(results, bangumiStaticCandidate{url: location.String(), mediaElement: mediaElement})
		}
		return nil
	}
	media := doc.Find("video[src], source[src]")
	for i := range min(media.Length(), bangumiResolveCandidateLimit) {
		src, _ := media.Eq(i).Attr("src")
		if err := add(src, true); err != nil {
			return nil, err
		}
	}
	// DOM scanning is an explicit source capability. Poster/artwork and arbitrary
	// navigation anchors are not player media.
	if r.config.MatchVideo.ScanDOMMediaURLs {
		nodes := doc.Find("[data-src], [data-url], [data-video], a[href]")
		for i := range min(nodes.Length(), 128) {
			for _, attribute := range []string{"data-src", "data-url", "data-video", "href"} {
				raw, ok := nodes.Eq(i).Attr(attribute)
				if !ok || raw == "" {
					continue
				}
				decoded := decodeBangumiLiteralURL(raw)
				if bangumiMediaFormat(decoded, "") == "unknown" {
					continue
				}
				if err := add(decoded, false); err != nil {
					return nil, err
				}
			}
		}
	}
	scripts := doc.Find("script:not([src])")
	for i := range min(scripts.Length(), 32) {
		script := scripts.Eq(i).Text()
		if len(script) > 32768 {
			continue
		}
		for _, objectMatch := range bangumiPlayerObject.FindAllStringSubmatch(script, 8) {
			object := objectMatch[1]
			var literal struct {
				URL     string `json:"url"`
				Encrypt any    `json:"encrypt"`
			}
			if common.Unmarshal([]byte(object), &literal) == nil && literal.URL != "" {
				encrypt := ""
				if match := bangumiEncryptField.FindStringSubmatch(object); len(match) > 0 {
					encrypt = match[1]
				}
				raw := literal.URL
				if encrypt == "2" {
					decoded, err := base64.StdEncoding.DecodeString(raw)
					if err != nil {
						return nil, errors.New("Player contains an invalid base64 URL literal")
					}
					raw = string(decoded)
				}
				if encrypt == "1" || encrypt == "2" {
					if decoded, err := url.PathUnescape(raw); err == nil {
						raw = decoded
					}
				}
				if err := add(raw, false); err != nil {
					return nil, err
				}
				continue
			}
			// Single-quoted JS objects are not JSON, but their individual string
			// values can still be read without interpreting the object as code.
			for _, field := range bangumiPlayerURLField.FindAllStringSubmatch(object, 8) {
				raw, err := bangumiJSString(field[1])
				if err != nil {
					return nil, errors.New("Player contains an invalid URL string literal")
				}
				if encrypt := bangumiEncryptField.FindStringSubmatch(object); len(encrypt) > 0 && encrypt[1] == "2" {
					decoded, err := base64.StdEncoding.DecodeString(raw)
					if err != nil {
						return nil, errors.New("Player contains an invalid base64 URL literal")
					}
					raw = string(decoded)
				}
				if err := add(raw, false); err != nil {
					return nil, err
				}
			}
		}
		for _, field := range bangumiPlayerURLField.FindAllStringSubmatch(script, 16) {
			raw, err := bangumiJSString(field[1])
			if err != nil {
				continue
			}
			decoded := decodeBangumiLiteralURL(raw)
			if !strings.HasPrefix(decoded, "http://") && !strings.HasPrefix(decoded, "https://") && !strings.HasPrefix(decoded, "//") && !strings.HasPrefix(decoded, "/") {
				continue
			}
			if err := add(decoded, false); err != nil {
				return nil, err
			}
		}
		for _, literal := range bangumiAtobLiteral.FindAllStringSubmatch(script, 8) {
			raw, err := bangumiJSString(literal[1])
			if err != nil {
				continue
			}
			decoded, err := base64.StdEncoding.DecodeString(raw)
			if err != nil {
				continue
			}
			if err := add(string(decoded), false); err != nil {
				return nil, err
			}
		}
		if r.config.MatchVideo.ScanInlineScriptURLs {
			for _, literal := range bangumiQuotedLiteral.FindAllString(script, 64) {
				raw, err := bangumiJSString(literal)
				if err != nil {
					continue
				}
				decoded := decodeBangumiLiteralURL(raw)
				if strings.HasPrefix(decoded, "http://") || strings.HasPrefix(decoded, "https://") || strings.HasPrefix(decoded, "//") {
					if err := add(decoded, false); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	frames := doc.Find("iframe[src]")
	for i := range min(frames.Length(), bangumiResolveCandidateLimit) {
		src, _ := frames.Eq(i).Attr("src")
		// Placeholder/ad navigation frames are not media candidates. They are
		// never fetched and must not invalidate a separately located video.
		if _, err := bangumiHTTPURL(page.url, decodeBangumiLiteralURL(src)); err != nil {
			continue
		}
		if err := add(src, false); err != nil {
			return nil, err
		}
	}
	return results, nil
}

func decodeBangumiLiteralURL(raw string) string {
	raw = strings.TrimSpace(html.UnescapeString(raw))
	// Decode the whole URL only when it is a literal encoded URL, not when it
	// already contains a scheme. Signed query-string escaping must stay intact.
	for range 2 {
		if strings.Contains(raw, "://") || strings.HasPrefix(raw, "//") || strings.HasPrefix(raw, "/") {
			break
		}
		decoded, err := url.PathUnescape(raw)
		if err != nil || decoded == raw {
			break
		}
		raw = decoded
	}
	return raw
}

func bangumiJSString(literal string) (string, error) {
	if len(literal) < 2 || len(literal) > 32768 {
		return "", errors.New("invalid literal")
	}
	if literal[0] == '"' {
		var value string
		if err := common.Unmarshal([]byte(literal), &value); err != nil {
			return "", err
		}
		return value, nil
	}
	if literal[0] != '\'' || literal[len(literal)-1] != '\'' {
		return "", errors.New("not a string literal")
	}
	var output strings.Builder
	inner := literal[1 : len(literal)-1]
	for i := 0; i < len(inner); i++ {
		if inner[i] != '\\' {
			output.WriteByte(inner[i])
			continue
		}
		i++
		if i >= len(inner) {
			return "", errors.New("truncated string escape")
		}
		switch inner[i] {
		case '\\', '/', '\'', '"':
			output.WriteByte(inner[i])
		case 'n':
			output.WriteByte('\n')
		case 'r':
			output.WriteByte('\r')
		case 't':
			output.WriteByte('\t')
		case 'u', 'x':
			count := 4
			if inner[i] == 'x' {
				count = 2
			}
			if i+count >= len(inner) {
				return "", errors.New("truncated unicode escape")
			}
			value, err := strconv.ParseUint(inner[i+1:i+1+count], 16, 32)
			if err != nil {
				return "", err
			}
			output.WriteRune(rune(value))
			i += count
		default:
			return "", errors.New("unsupported string escape")
		}
	}
	return output.String(), nil
}
