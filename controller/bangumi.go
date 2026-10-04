package controller

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
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

const (
	bangumiAPIBase    = "https://api.bgm.tv"
	bangumiImageBase  = "https://lain.bgm.tv"
	bangumiUserAgent  = "ReX-API/1.0 (https://github.com/QuantumNous/new-api)"
	bangumiMaxBody    = 4 << 20
	bangumiSearchType = 2 // anime, excluding books and music
)

var bangumiCacheTTL = map[string]time.Duration{
	"calendar": 30 * time.Minute,
	"subject":  6 * time.Hour,
	"search":   10 * time.Minute,
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

	response, err := service.GetSSRFProtectedHTTPClient().Do(request)
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
	id := strings.TrimSpace(c.Param("id"))
	// The id goes into a cache key and an upstream path, so keep it to digits
	// rather than trusting the shape.
	for _, r := range id {
		if r < '0' || r > '9' {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid subject id"})
			return
		}
	}
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid subject id"})
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

		response, err := service.GetSSRFProtectedHTTPClient().Do(request)
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

// ProxyBangumiImage streams a cover through this instance.
//
// The upstream hands out two path shapes -- a bare `/pic/cover/l/...` from the
// calendar and an already-sized `/r/400/pic/...` from the subject endpoint --
// and its image host serves the first only when asked for as-is: requesting it
// under `/r/` without a width returns 400. So the path is forwarded verbatim
// and the caller's job is simply to keep whichever shape it was given.
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

	response, err := service.GetSSRFProtectedHTTPClient().Do(request)
	if err != nil {
		c.Status(http.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		c.Status(response.StatusCode)
		return
	}
	contentType := response.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/jpeg"
	}
	c.DataFromReader(http.StatusOK, response.ContentLength, contentType,
		response.Body, nil)
}
