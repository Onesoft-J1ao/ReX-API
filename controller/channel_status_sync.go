package controller

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/channel_status_sync_setting"

	"github.com/gin-gonic/gin"
)

// channelStatusSyncTimeout bounds one full sync cycle (two remote requests plus
// the channel scan) for both the manual and the scheduled path.
const channelStatusSyncTimeout = 60 * time.Second

// PreviewChannelStatusSync /api/channel/status_sync/preview — fetch the remote
// status page and report the change set without persisting anything, so an
// operator can review what the sync would do before letting it run.
func PreviewChannelStatusSync(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), channelStatusSyncTimeout)
	defer cancel()

	plan, _, err := service.RunChannelStatusSync(ctx, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    plan,
	})
}

// ApplyChannelStatusSync /api/channel/status_sync/apply — run the same fetch and
// match, then persist the change set. It re-fetches instead of accepting a
// client-supplied plan so the applied state always corresponds to what the
// remote page says right now.
func ApplyChannelStatusSync(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), channelStatusSyncTimeout)
	defer cancel()

	plan, changed, err := service.RunChannelStatusSync(ctx, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "channel.status_sync_apply", map[string]any{
		"source":  plan.SourceURL,
		"changed": changed,
		"disable": plan.DisableCount,
		"enable":  plan.EnableCount,
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"changed": changed,
			"plan":    plan,
		},
	})
}

// ------------------- scheduled sync -------------------

type channelStatusSyncHandler struct{}

func (channelStatusSyncHandler) Type() string { return model.SystemTaskTypeChannelStatusSync }

func (channelStatusSyncHandler) Enabled() bool {
	return channel_status_sync_setting.GetSetting().Enabled
}

func (channelStatusSyncHandler) Interval() time.Duration {
	return time.Duration(channel_status_sync_setting.GetSyncIntervalMinutes()) * time.Minute
}

func (channelStatusSyncHandler) NewPayload() any { return nil }

type channelStatusSyncSummary struct {
	SourceURL    string `json:"source_url"`
	MonitorCount int    `json:"monitor_count"`
	ChannelCount int    `json:"channel_count"`
	DisableCount int    `json:"disable_count"`
	EnableCount  int    `json:"enable_count"`
	Changed      int    `json:"changed"`
	Blocked      bool   `json:"blocked"`
	Reason       string `json:"reason,omitempty"`
}

// Run executes one scheduled sync. A blocked or failed plan is recorded as the
// task result rather than as a task failure: the job did what it should by
// refusing, and the reason belongs in the run history where an operator sees it.
func (channelStatusSyncHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	plan, changed, err := service.RunChannelStatusSync(ctx, true)
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	summary := channelStatusSyncSummary{
		SourceURL:    plan.SourceURL,
		MonitorCount: plan.MonitorCount,
		ChannelCount: plan.ChannelCount,
		DisableCount: plan.DisableCount,
		EnableCount:  plan.EnableCount,
		Changed:      changed,
		Blocked:      plan.Blocked,
		Reason:       plan.BlockedReason,
	}
	if changed > 0 {
		common.SysLog(fmt.Sprintf("channel status sync applied %d change(s) from %s (disable=%d enable=%d)",
			changed, plan.SourceURL, plan.DisableCount, plan.EnableCount))
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}
