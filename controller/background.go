/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/console_setting"

	"github.com/gin-gonic/gin"
)

const (
	unsplashAPIBase = "https://api.unsplash.com"
	// The demo tier allows 50 calls per hour. One cached lookup per five
	// minutes keeps a busy site far below that while still rotating the
	// backdrop often enough to feel alive.
	unsplashCacheTTL = 5 * time.Minute
	unsplashTimeout  = 15 * time.Second
)

// randomBackground is what the SPA renders. It carries the attribution the
// Unsplash API guidelines require alongside the image URL.
type randomBackground struct {
	URL        string `json:"url"`
	AuthorName string `json:"author_name"`
	AuthorLink string `json:"author_link"`
	PhotoLink  string `json:"photo_link"`
}

// unsplashPhoto mirrors the fields of GET /photos/random that we consume. The
// response carries far more, and decoding only what is used keeps this from
// breaking when Unsplash adds fields.
type unsplashPhoto struct {
	URLs struct {
		Regular string `json:"regular"`
		Full    string `json:"full"`
	} `json:"urls"`
	User struct {
		Name  string `json:"name"`
		Links struct {
			HTML string `json:"html"`
		} `json:"links"`
	} `json:"user"`
	Links struct {
		HTML string `json:"html"`
	} `json:"links"`
}

var randomBackgroundCache = struct {
	sync.Mutex
	photo     *randomBackground
	fetchedAt time.Time
}{}

// GetRandomBackground serves one Unsplash photo for the site backdrop.
//
// The access key never leaves the server: the browser asks this endpoint, which
// holds the credential and the shared cache. An unconfigured site answers with
// an empty payload so the client falls back to its bundled images.
func GetRandomBackground(c *gin.Context) {
	query := console_setting.GetUnsplashQuery()
	accessKey := console_setting.GetUnsplashAccessKey()
	if query == "" || accessKey == "" {
		common.ApiSuccess(c, nil)
		return
	}

	if cached := readCachedRandomBackground(); cached != nil {
		common.ApiSuccess(c, cached)
		return
	}

	photo, err := fetchUnsplashRandom(c, query, accessKey)
	if err != nil {
		// A stale entry beats no backdrop, so a transient upstream failure
		// still renders the last good image.
		if stale := readStaleRandomBackground(); stale != nil {
			common.ApiSuccess(c, stale)
			return
		}
		logger.LogError(c.Request.Context(), fmt.Sprintf("[Background] Unsplash lookup failed: %s", err.Error()))
		common.ApiSuccess(c, nil)
		return
	}

	storeRandomBackground(photo)
	common.ApiSuccess(c, photo)
}

func readCachedRandomBackground() *randomBackground {
	randomBackgroundCache.Lock()
	defer randomBackgroundCache.Unlock()
	if randomBackgroundCache.photo == nil || time.Since(randomBackgroundCache.fetchedAt) > unsplashCacheTTL {
		return nil
	}
	return randomBackgroundCache.photo
}

func readStaleRandomBackground() *randomBackground {
	randomBackgroundCache.Lock()
	defer randomBackgroundCache.Unlock()
	return randomBackgroundCache.photo
}

func storeRandomBackground(photo *randomBackground) {
	randomBackgroundCache.Lock()
	defer randomBackgroundCache.Unlock()
	randomBackgroundCache.photo = photo
	randomBackgroundCache.fetchedAt = time.Now()
}

func fetchUnsplashRandom(c *gin.Context, query, accessKey string) (*randomBackground, error) {
	requestURL := strings.TrimRight(unsplashAPIBase, "/") + "/photos/random?" + url.Values{
		"query":          {query},
		"orientation":    {"landscape"},
		"content_filter": {"high"},
	}.Encode()

	ctx, cancel := context.WithTimeout(c.Request.Context(), unsplashTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	// Unsplash authenticates with the access key as the Client-ID. The secret
	// key belongs to the user-authorization flow and is not used here.
	req.Header.Set("Authorization", "Client-ID "+accessKey)
	req.Header.Set("Accept-Version", "v1")

	res, err := service.GetSSRFProtectedHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Unsplash returned status %d", res.StatusCode)
	}

	var photo unsplashPhoto
	if err := common.DecodeJson(res.Body, &photo); err != nil {
		return nil, err
	}

	imageURL := photo.URLs.Regular
	if imageURL == "" {
		imageURL = photo.URLs.Full
	}
	if imageURL == "" {
		return nil, fmt.Errorf("Unsplash returned no image URL")
	}

	return &randomBackground{
		URL:        imageURL,
		AuthorName: photo.User.Name,
		AuthorLink: withUTM(photo.User.Links.HTML),
		PhotoLink:  withUTM(photo.Links.HTML),
	}, nil
}

// withUTM appends the attribution parameters Unsplash asks for on links back
// to the photographer and the photo.
func withUTM(raw string) string {
	if raw == "" {
		return ""
	}
	separator := "?"
	if strings.Contains(raw, "?") {
		separator = "&"
	}
	return raw + separator + "utm_source=rex_api&utm_medium=referral"
}
