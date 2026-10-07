package controller

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

const bangumiMediaCookie = "bangumi_media_access"

type bangumiMediaResource struct {
	url   string
	depth int
}

type bangumiMediaGrant struct {
	id        string
	secret    [32]byte
	identity  service.AuthIdentity
	expires   time.Time
	headers   http.Header
	origin    string
	resources map[string]bangumiMediaResource
	bytes     int
	client    *http.Client
	// onStart is invoked once per media request before any bytes move, and the
	// returned function is released when that request ends. A torrent backend
	// uses it to keep the file it is serving from being evicted mid-playback.
	onStart func() (func(), error)
}

var bangumiMediaStore = struct {
	sync.Mutex
	grants  map[string]*bangumiMediaGrant
	streams chan struct{}
}{grants: make(map[string]*bangumiMediaGrant), streams: make(chan struct{}, 8)}

// newBangumiMediaGrant registers a same-origin media URL for a resolved source.
// onStart may be nil for plain HTTP sources; a torrent source passes a function
// that takes a reference on the file it is about to stream.
func newBangumiMediaGrant(c *gin.Context, target string, headers http.Header) (string, error) {
	return newBangumiMediaGrantWithStart(c, target, headers, nil)
}

func newBangumiMediaGrantWithStart(c *gin.Context, target string, headers http.Header, onStart func() (func(), error)) (string, error) {
	location, err := url.Parse(target)
	if err != nil || (location.Scheme != "https" && location.Scheme != "http") || location.User != nil {
		return "", errors.New("invalid media target")
	}
	identity, authenticated := middleware.GetSessionAuthIdentity(c)
	if middleware.HeaderNavModuleRequiresAuthentication("bangumi") && !authenticated {
		return "", errors.New("browser login session is required for protected playback")
	}
	client, err := service.NewBangumiPublicHTTPClient(common.BangumiProxyURL)
	if err != nil {
		return "", err
	}
	client.Timeout = 0
	id, secret := rand.Text(), rand.Text()
	grant := &bangumiMediaGrant{
		id: id, secret: sha256.Sum256([]byte(secret)), identity: identity,
		expires: time.Now().Add(time.Hour), headers: headers.Clone(), origin: location.Host,
		resources: map[string]bangumiMediaResource{"root": {url: target}}, bytes: len(target), client: client,
		onStart: onStart,
	}
	bangumiMediaStore.Lock()
	for key, existing := range bangumiMediaStore.grants {
		if time.Now().After(existing.expires) {
			existing.client.CloseIdleConnections()
			delete(bangumiMediaStore.grants, key)
		}
	}
	if len(bangumiMediaStore.grants) >= 64 {
		bangumiMediaStore.Unlock()
		client.CloseIdleConnections()
		return "", errors.New("media playback capacity reached")
	}
	bangumiMediaStore.grants[id] = grant
	bangumiMediaStore.Unlock()
	path := "/api/bangumi/media/" + id + "/"
	// The URL is a lookup ID; the resource-scoped secret remains HttpOnly.
	http.SetCookie(c.Writer, &http.Cookie{
		Name: bangumiMediaCookie, Value: secret, Path: path, HttpOnly: true,
		Secure: common.SessionCookieSecure, SameSite: http.SameSiteLaxMode,
		MaxAge: 3600, Expires: grant.expires,
	})
	return path + "root", nil
}

func ResolveBangumiMediaSource(c *gin.Context) {
	manager, ok := getBangumiSourcesManager(c)
	if !ok {
		return
	}
	sourceID, episodeID := c.Param("sourceId"), c.Query("episodeId")
	if sourceID == "" || len(sourceID) > 128 || episodeID == "" || len(episodeID) > 128 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid source episode"})
		return
	}
	if !applyBangumiClearance(c, manager, sourceID) {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 90*time.Second)
	defer cancel()
	resolved, err := manager.Resolve(ctx, sourceID, episodeID)
	if err != nil {
		c.JSON(http.StatusOK, resolved)
		return
	}
	if resolved.Format == "torrent" {
		resolveBangumiTorrentSource(c, resolved, episodeID)
		return
	}
	streamURL, err := newBangumiMediaGrant(c, resolved.VideoURL, resolved.Headers)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"sourceId": sourceID, "state": "failed", "message": "Media playback could not be authorized or prepared"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"sourceId": sourceID, "state": "ready", "format": resolved.Format, "streamUrl": streamURL})
}

func GetBangumiSourceMedia(c *gin.Context) {
	id, resourceID := c.Param("mediaId"), c.Param("resourceId")
	secret, err := c.Cookie(bangumiMediaCookie)
	if err != nil || len(secret) > 128 {
		c.Status(http.StatusForbidden)
		return
	}
	digest := sha256.Sum256([]byte(secret))
	bangumiMediaStore.Lock()
	grant := bangumiMediaStore.grants[id]
	var resource bangumiMediaResource
	if grant != nil {
		resource = grant.resources[resourceID]
	}
	bangumiMediaStore.Unlock()
	if grant == nil || resource.url == "" || time.Now().After(grant.expires) || subtle.ConstantTimeCompare(digest[:], grant.secret[:]) != 1 {
		c.Status(http.StatusForbidden)
		return
	}
	if grant.identity.SessionID != "" {
		_, user, err := service.ValidateLoginSession(grant.identity)
		if err != nil || user.Status != common.UserStatusEnabled || user.Role < common.RoleCommonUser {
			c.Status(http.StatusUnauthorized)
			return
		}
	} else if middleware.HeaderNavModuleRequiresAuthentication("bangumi") {
		c.Status(http.StatusUnauthorized)
		return
	}
	select {
	case bangumiMediaStore.streams <- struct{}{}:
		defer func() { <-bangumiMediaStore.streams }()
	default:
		c.Status(http.StatusTooManyRequests)
		return
	}

	// The reference is taken before any bytes move and released on every exit
	// path below, including the early returns and a failed write. Without the
	// deferred release a torrent backend would keep the file pinned forever.
	if grant.onStart != nil {
		release, err := grant.onStart()
		if err != nil {
			c.Status(http.StatusServiceUnavailable)
			return
		}
		if release != nil {
			defer release()
		}
	}

	request, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, resource.url, nil)
	if err != nil {
		c.Status(http.StatusBadGateway)
		return
	}
	request.Header = grant.headers.Clone()
	if request.URL.Host != grant.origin {
		request.Header.Del("Cookie")
		request.Header.Del("Authorization")
	}
	request.Header.Set("Accept-Encoding", "identity")
	for _, name := range []string{"Range", "If-Range", "If-None-Match", "If-Modified-Since"} {
		if value := c.GetHeader(name); value != "" {
			request.Header.Set(name, value)
		}
	}
	response, err := grant.client.Do(request)
	if err != nil {
		c.Status(http.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	setTaskMediaResponseSecurityHeaders(c.Writer.Header())
	switch response.StatusCode {
	case http.StatusOK, http.StatusPartialContent, http.StatusNotModified, http.StatusRequestedRangeNotSatisfiable:
	default:
		c.Status(http.StatusBadGateway)
		return
	}
	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	if strings.Contains(contentType, "text/html") || strings.Contains(contentType, "javascript") {
		c.Status(http.StatusBadGateway)
		return
	}
	if c.Request.Method == http.MethodHead || response.StatusCode == http.StatusNotModified || response.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		copyTaskMediaResponseHeaders(c.Writer.Header(), response.Header)
		c.Status(response.StatusCode)
		if c.Request.Method != http.MethodHead && response.StatusCode == http.StatusRequestedRangeNotSatisfiable {
			io.Copy(c.Writer, io.LimitReader(response.Body, 4096))
		}
		return
	}
	if strings.Contains(contentType, "mpegurl") || strings.HasSuffix(strings.ToLower(response.Request.URL.Path), ".m3u8") {
		if resource.depth >= 5 {
			c.Status(http.StatusBadGateway)
			return
		}
		body, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
		if err != nil || len(body) > 1<<20 {
			c.Status(http.StatusBadGateway)
			return
		}
		rewritten, err := rewriteBangumiManifest(grant, resource, response.Request.URL, body)
		if err != nil {
			c.Status(http.StatusBadGateway)
			return
		}
		c.Data(http.StatusOK, "application/vnd.apple.mpegurl", rewritten)
		return
	}
	copyTaskMediaResponseHeaders(c.Writer.Header(), response.Header)
	c.Status(response.StatusCode)
	c.Writer.WriteHeaderNow()
	io.CopyBuffer(c.Writer, response.Body, make([]byte, 32<<10))
}

var bangumiHLSURI = regexp.MustCompile(`\bURI="([^"\r\n]*)"`)

func rewriteBangumiManifest(grant *bangumiMediaGrant, resource bangumiMediaResource, base *url.URL, body []byte) ([]byte, error) {
	text := strings.TrimPrefix(string(body), "\xef\xbb\xbf")
	if !strings.HasPrefix(strings.TrimSpace(text), "#EXTM3U") {
		return nil, errors.New("invalid HLS manifest")
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "#") {
			link, err := registerBangumiMediaResource(grant, base, trimmed, resource.depth+1)
			if err != nil {
				return nil, err
			}
			lines[i] = link
			continue
		}
		if !strings.Contains(line, "URI=") {
			continue
		}
		if !bangumiHLSURI.MatchString(line) {
			return nil, errors.New("invalid HLS URI attribute")
		}
		var replaceErr error
		lines[i] = bangumiHLSURI.ReplaceAllStringFunc(line, func(attribute string) string {
			raw := bangumiHLSURI.FindStringSubmatch(attribute)[1]
			link, err := registerBangumiMediaResource(grant, base, raw, resource.depth+1)
			if err != nil {
				replaceErr = err
				return ""
			}
			return `URI="` + link + `"`
		})
		if replaceErr != nil {
			return nil, replaceErr
		}
	}
	return []byte(strings.Join(lines, "\n")), nil
}

func registerBangumiMediaResource(grant *bangumiMediaGrant, base *url.URL, raw string, depth int) (string, error) {
	if len(raw) == 0 || len(raw) > 8192 || strings.ContainsAny(raw, "\x00\r\n\\") || strings.Contains(raw, "{$") {
		return "", errors.New("invalid or unsupported HLS resource")
	}
	location, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	location = base.ResolveReference(location)
	if (location.Scheme != "http" && location.Scheme != "https") || location.User != nil || location.Hostname() == "" {
		return "", errors.New("unsafe HLS target")
	}
	location.Fragment = ""
	target := location.String()
	digest := sha256.Sum256([]byte(target))
	id := hex.EncodeToString(digest[:16])
	bangumiMediaStore.Lock()
	defer bangumiMediaStore.Unlock()
	if _, exists := grant.resources[id]; !exists {
		if len(grant.resources) >= 2048 || grant.bytes+len(target) > 512<<10 {
			return "", errors.New("HLS resource budget reached")
		}
		grant.resources[id] = bangumiMediaResource{url: target, depth: depth}
		grant.bytes += len(target)
	}
	return "/api/bangumi/media/" + grant.id + "/" + id, nil
}
