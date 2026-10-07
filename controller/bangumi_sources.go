package controller

import (
	"context"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

var bangumiSourcesHost struct {
	once    sync.Once
	manager *service.BangumiSourceManager
	err     error
}

func getBangumiSourcesManager(c *gin.Context) (*service.BangumiSourceManager, bool) {
	bangumiSourcesHost.once.Do(func() {
		client, err := service.NewBangumiPublicHTTPClient(common.BangumiProxyURL)
		if err != nil {
			bangumiSourcesHost.err = err
			return
		}
		bangumiSourcesHost.manager, bangumiSourcesHost.err = service.NewBangumiSourceManager(client, os.Getenv("BANGUMI_SOURCE_CONFIG_PATH"))
	})
	if bangumiSourcesHost.err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "Media source configuration is unavailable"})
		return nil, false
	}
	return bangumiSourcesHost.manager, true
}

func GetBangumiMediaSources(c *gin.Context) {
	manager, ok := getBangumiSourcesManager(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"sources":             manager.Sources(),
		"legacyMikanSourceId": "mikan-mikanime-tv",
	})
}

// applyBangumiClearance hands the manager a cookie the viewer solved a source's
// challenge to obtain, so their retry gets past it.
//
// Source pages are fetched by this server, so the clearance the viewer's own
// browser earned does not travel with them; the picker collects it and sends it
// here. The value is never echoed back, and the manager drops it when it goes
// stale. A request that omits the parameter leaves whatever is already held
// alone; sending it empty clears it.
func applyBangumiClearance(c *gin.Context, manager *service.BangumiSourceManager, sourceID string) bool {
	raw, present := c.GetQuery("clearance")
	if !present {
		return true
	}
	if err := manager.SetClearance(sourceID, raw); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid clearance value"})
		return false
	}
	return true
}

func SearchBangumiMediaSource(c *gin.Context) {
	manager, ok := getBangumiSourcesManager(c)
	if !ok {
		return
	}
	sourceID := strings.TrimSpace(c.Param("sourceId"))
	keyword := strings.TrimSpace(c.Query("keyword"))
	if sourceID == "" || len(sourceID) > 128 || keyword == "" || len(keyword) > 512 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid source or search keyword"})
		return
	}
	if !applyBangumiClearance(c, manager, sourceID) {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 45*time.Second)
	defer cancel()
	result, _ := manager.Search(ctx, sourceID, keyword)
	// The typed state preserves CAPTCHA, rate-limit and empty results independently.
	c.JSON(http.StatusOK, result)
}

func BrowseBangumiMediaSource(c *gin.Context) {
	manager, ok := getBangumiSourcesManager(c)
	if !ok {
		return
	}
	sourceID := strings.TrimSpace(c.Param("sourceId"))
	subjectID := strings.TrimSpace(c.Param("sourceSubjectId"))
	if sourceID == "" || len(sourceID) > 128 || subjectID == "" || len(subjectID) > 128 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid source item"})
		return
	}
	if !applyBangumiClearance(c, manager, sourceID) {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 45*time.Second)
	defer cancel()
	result, _ := manager.Browse(ctx, sourceID, subjectID)
	c.JSON(http.StatusOK, result)
}
