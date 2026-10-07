/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/

package service

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
)

const imageURLProxyPathPrefix = "/v1/artifacts/"

type imageURLProxyEntry struct {
	URL       string
	ExpiresAt time.Time
}

var imageURLProxyStore = struct {
	sync.Mutex
	entries map[string]imageURLProxyEntry
}{entries: make(map[string]imageURLProxyEntry)}

func shouldProxyGeneratedImageURLs(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	typeID := common.GetContextKeyInt(c, constant.ContextKeyChannelType)
	if typeID != constant.ChannelTypeOpenAI && typeID != constant.ChannelTypeGemini {
		return false
	}
	settings, ok := common.GetContextKeyType[dto.ChannelSettings](c, constant.ContextKeyChannelSetting)
	if !ok || !settings.ProxyImageURLs {
		return false
	}
	path := c.Request.URL.Path
	return path == "/v1/images/generations" || path == "/v1/images/edits" ||
		path == "/pg/images/generations" || path == "/pg/images/edits" ||
		(strings.HasPrefix(path, "/v1beta/models/") &&
			(strings.Contains(path, ":generateContent") || strings.Contains(path, ":streamGenerateContent")))
}

func proxyImageResponseOrOriginal(c *gin.Context, body []byte) []byte {
	if !shouldProxyGeneratedImageURLs(c) {
		return body
	}
	var payload any
	if err := common.Unmarshal(body, &payload); err != nil {
		return body
	}
	changed := rewriteGeneratedImageURLs(c, payload)
	if !changed {
		return body
	}
	rewritten, err := common.Marshal(payload)
	if err != nil {
		return body
	}
	return rewritten
}

func rewriteGeneratedImageURLs(c *gin.Context, value any) bool {
	switch current := value.(type) {
	case []any:
		changed := false
		for _, item := range current {
			if rewriteGeneratedImageURLs(c, item) {
				changed = true
			}
		}
		return changed
	case map[string]any:
		changed := false
		for _, key := range []string{"url", "fileUri", "file_uri"} {
			if raw, ok := current[key].(string); ok && isProxyableImageURL(raw) {
				current[key] = saveImageURLProxy(c, raw)
				changed = true
			}
		}
		if raw, ok := current["image_url"].(string); ok && isProxyableImageURL(raw) {
			current["image_url"] = saveImageURLProxy(c, raw)
			changed = true
		}
		for _, child := range current {
			if rewriteGeneratedImageURLs(c, child) {
				changed = true
			}
		}
		return changed
	default:
		return false
	}
}

func isProxyableImageURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func saveImageURLProxy(c *gin.Context, originalURL string) string {
	// The artifact id is the only thing guarding the image, because the browser
	// loads it from an <img> that cannot carry an Authorization header. A
	// request-id-plus-weak-random id is therefore not enough: it is derived from
	// a value the caller already sees, and the generator behind it is
	// math/rand/v2, so a few observed ids are enough to enumerate other callers'
	// artifacts. The id is a 128-bit crypto-random token instead, which cannot be
	// guessed or derived from the response the caller holds.
	artifactID, err := common.GenerateRandomCharsKey(22)
	if err != nil {
		// Falling back to the request id would silently restore the guessable
		// scheme, so a failure to draw entropy is a hard failure.
		common.SysError("image artifact token generation failed: " + err.Error())
		return originalURL
	}
	imageURLProxyStore.Lock()
	imageURLProxyStore.entries[artifactID] = imageURLProxyEntry{URL: originalURL, ExpiresAt: time.Now().Add(30 * time.Minute)}
	imageURLProxyStore.Unlock()
	baseAddress := strings.TrimRight(strings.TrimSpace(system_setting.ServerAddress), "/")
	if baseAddress == "" && c != nil && c.Request != nil {
		scheme := "http"
		if strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") || c.Request.TLS != nil {
			scheme = "https"
		}
		baseAddress = scheme + "://" + c.Request.Host
	}
	if parsed, err := url.Parse(baseAddress); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		parsed.Path = strings.TrimRight(parsed.Path, "/") + imageURLProxyPathPrefix + artifactID + ".png"
		parsed.RawQuery = ""
		parsed.Fragment = ""
		return parsed.String()
	}
	return imageURLProxyPathPrefix + artifactID + ".png"
}

// ImageURLProxy serves a short-lived URL that hides the upstream image URL.
func ImageURLProxy(c *gin.Context) {
	artifactID := strings.TrimSuffix(strings.TrimSpace(c.Param("artifact_id")), ".png")
	imageURLProxyStore.Lock()
	entry, ok := imageURLProxyStore.entries[artifactID]
	if ok && time.Now().After(entry.ExpiresAt) {
		delete(imageURLProxyStore.entries, artifactID)
		ok = false
	}
	imageURLProxyStore.Unlock()
	if !ok || !isProxyableImageURL(entry.URL) {
		c.Status(http.StatusNotFound)
		return
	}
	if err := ValidateSSRFProtectedFetchURL(entry.URL); err != nil {
		c.Status(http.StatusBadGateway)
		return
	}
	request, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, entry.URL, nil)
	if err != nil {
		c.Status(http.StatusBadGateway)
		return
	}
	response, err := GetSSRFProtectedHTTPClient().Do(request)
	if err != nil {
		c.Status(http.StatusBadGateway)
		return
	}
	defer CloseResponseBodyGracefully(response)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		c.Status(http.StatusBadGateway)
		return
	}
	// The upstream chooses the content type, and this response is served from
	// the gateway's own origin, so a hostile or compromised upstream returning
	// text/html would otherwise be a script running on this site. Only image
	// types are forwarded, and the browser is told not to sniff past the header.
	contentType := response.Header.Get("Content-Type")
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(contentType)), "image/") {
		contentType = "application/octet-stream"
	}
	c.Header("Content-Type", contentType)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Disposition", "inline")
	c.Header("Cache-Control", "private, max-age=1800")
	c.Status(response.StatusCode)
	_, _ = io.Copy(c.Writer, response.Body)
}

func RewriteGeneratedImageURLs(c *gin.Context, body []byte) []byte {
	return proxyImageResponseOrOriginal(c, body)
}
