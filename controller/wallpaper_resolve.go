package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// Resolving a Wallpaper Engine link has to happen here rather than in the
// console: Steam's API sends no CORS headers, so a browser cannot read the
// answer. The lookup is a plain read on a public item, and the only thing the
// caller controls is the item id.
//
// A Workshop item is a video or a scene; Steam publishes a still preview of it,
// and that still is what a CSS background can use.

const (
	steamPublishedFileDetailsURL = "https://api.steampowered.com/ISteamRemoteStorage/GetPublishedFileDetails/v1/"
	steamResolveTimeout          = 12 * time.Second
	maxWorkshopItemIDLength      = 20
	// Steam's own id for Wallpaper Engine.
	wallpaperEngineAppID = 431960
)

// The address handed back is painted into a CSS `url("...")`, so it has to come
// from Steam's image hosts rather than from whatever a response claims.
var steamImageHostSuffixes = []string{
	".steamusercontent.com",
	".steamstatic.com",
	".akamaihd.net",
	".steamcommunity.com",
}

var (
	digitOnlyPattern = regexp.MustCompile(`^[0-9]+$`)
	// Steam serves these two shapes; anything else is not a Workshop item.
	workshopPathPattern = regexp.MustCompile(`^/(sharedfiles|workshop)/filedetails/?$`)
)

// ParseWorkshopItemID accepts a Steam Workshop link or a bare item id and
// returns the id.
//
// Only steamcommunity.com links are accepted and the id is always digits, so
// nothing user-supplied reaches the outbound request beyond a number.
func ParseWorkshopItemID(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("链接不能为空")
	}

	if digitOnlyPattern.MatchString(trimmed) {
		return validateWorkshopItemID(trimmed)
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", errors.New("链接格式无法解析")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("只支持 http(s) 链接")
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "steamcommunity.com" && host != "www.steamcommunity.com" {
		return "", errors.New("只支持 steamcommunity.com 的创意工坊链接")
	}
	if !workshopPathPattern.MatchString(parsed.Path) {
		return "", errors.New("这不是创意工坊物品链接")
	}
	return validateWorkshopItemID(parsed.Query().Get("id"))
}

func validateWorkshopItemID(id string) (string, error) {
	if id == "" {
		return "", errors.New("链接里没有物品 id")
	}
	if !digitOnlyPattern.MatchString(id) {
		return "", errors.New("物品 id 必须是数字")
	}
	if len(id) > maxWorkshopItemIDLength {
		return "", errors.New("物品 id 过长")
	}
	return id, nil
}

// isSteamImageURL keeps the answer on Steam's image hosts.
func isSteamImageURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	for _, suffix := range steamImageHostSuffixes {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

type steamItemDetails struct {
	ItemID        string
	Title         string
	PreviewURL    string
	Banned        bool
	ConsumerAppID int
	CreatorAppID  int
}

func fetchSteamPublishedFileDetails(ctx context.Context, itemID string) (steamItemDetails, error) {
	form := url.Values{}
	form.Set("itemcount", "1")
	form.Set("publishedfileids[0]", itemID)

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		steamPublishedFileDetailsURL,
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return steamItemDetails{}, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: steamResolveTimeout}
	response, err := client.Do(request)
	if err != nil {
		return steamItemDetails{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return steamItemDetails{}, fmt.Errorf("steam returned %d", response.StatusCode)
	}

	// Bound what we are willing to read: the endpoint answers in bytes, and a
	// redirect or a hijacked answer must not stream indefinitely.
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return steamItemDetails{}, err
	}

	var payload struct {
		Response struct {
			PublishedFileDetails []struct {
				PublishedFileID string `json:"publishedfileid"`
				Result          int    `json:"result"`
				Title           string `json:"title"`
				PreviewURL      string `json:"preview_url"`
				Banned          int    `json:"banned"`
				ConsumerAppID   int    `json:"consumer_app_id"`
				CreatorAppID    int    `json:"creator_app_id"`
			} `json:"publishedfiledetails"`
		} `json:"response"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return steamItemDetails{}, err
	}
	if len(payload.Response.PublishedFileDetails) == 0 {
		return steamItemDetails{}, errors.New("steam returned no details")
	}

	entry := payload.Response.PublishedFileDetails[0]
	if entry.Result != 1 {
		return steamItemDetails{}, fmt.Errorf("steam result %d", entry.Result)
	}

	return steamItemDetails{
		ItemID:        entry.PublishedFileID,
		Title:         entry.Title,
		PreviewURL:    entry.PreviewURL,
		Banned:        entry.Banned != 0,
		ConsumerAppID: entry.ConsumerAppID,
		CreatorAppID:  entry.CreatorAppID,
	}, nil
}

// ResolveWallpaperSource turns a Wallpaper Engine Workshop link into the preview
// image the wallpaper field can hold.
func ResolveWallpaperSource(c *gin.Context) {
	var request struct {
		URL string `json:"url"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	itemID, err := ParseWorkshopItemID(request.URL)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}

	details, err := fetchSteamPublishedFileDetails(c.Request.Context(), itemID)
	if err != nil {
		common.SysError("wallpaper source resolve failed: " + err.Error())
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法解析这个创意工坊链接（Steam 未返回该物品）",
		})
		return
	}

	if details.Banned {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "这个创意工坊物品已被下架",
		})
		return
	}
	if !isSteamImageURL(details.PreviewURL) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "这个物品没有可用的预览图",
		})
		return
	}

	// Validate with the same rule the paid change uses, so the console never
	// offers an address the save would then refuse.
	imageURL, err := model.NormalizeWallpaperURL(details.PreviewURL)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "解析出的图片地址不可用",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"item_id":   details.ItemID,
			"title":     details.Title,
			"image_url": imageURL,
			// Reported rather than required, so an item from another app still
			// resolves and the console can say what it is.
			"app_id":           details.ConsumerAppID,
			"wallpaper_engine": details.ConsumerAppID == wallpaperEngineAppID || details.CreatorAppID == wallpaperEngineAppID,
		},
	})
}
