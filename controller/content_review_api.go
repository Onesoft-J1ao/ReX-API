package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

// Content review endpoints. Admin (not root) is enough: the operator asked for
// a manual moderation workflow, and every path here is admin-only, so a regular
// user can never read their own captured bodies.

// GetContentReviewStatus tells the console whether capture is on.
func GetContentReviewStatus(c *gin.Context) {
	setting := operation_setting.GetContentReviewSetting()
	common.ApiSuccess(c, gin.H{
		"enabled":        setting.Enabled,
		"retention_days": setting.RetentionDays,
	})
}

// GetContentReviewList pages stored exchanges, newest first.
func GetContentReviewList(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	username := c.Query("username")
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)

	rows, total, err := model.GetLogContentsPage(
		pageInfo.GetStartIdx(), pageInfo.GetPageSize(),
		username, startTimestamp, endTimestamp)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// List view carries the metadata only; bodies come one at a time from the
	// detail endpoint so paging stays cheap.
	type row struct {
		Id              int64  `json:"id"`
		RequestId       string `json:"request_id"`
		UserId          int    `json:"user_id"`
		CreatedAt       int64  `json:"created_at"`
		ModelName       string `json:"model_name"`
		TokenName       string `json:"token_name"`
		Truncated       bool   `json:"truncated"`
		RequestPreview  string `json:"request_preview"`
		ResponsePreview string `json:"response_preview"`
	}
	items := make([]row, 0, len(rows))
	for _, r := range rows {
		items = append(items, row{
			Id:              r.Id,
			RequestId:       r.RequestId,
			UserId:          r.UserId,
			CreatedAt:       r.CreatedAt,
			ModelName:       r.ModelName,
			TokenName:       r.TokenName,
			Truncated:       r.Truncated,
			RequestPreview:  preview(r.RequestText),
			ResponsePreview: preview(r.ResponseText),
		})
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

const previewLen = 160

func preview(text string) string {
	runes := []rune(text)
	if len(runes) <= previewLen {
		return text
	}
	return string(runes[:previewLen]) + "…"
}

// GetContentReviewDetail returns one exchange with the full bodies.
func GetContentReviewDetail(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	row, err := model.GetLogContentById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, row)
}

// DownloadContentReviewDetail streams one exchange as a plain-text file.
func DownloadContentReviewDetail(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	row, err := model.GetLogContentById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	body := fmt.Sprintf("=== 请求 (request) ===\n%s\n\n=== 回答 (response) ===\n%s\n",
		row.RequestText, row.ResponseText)
	filename := fmt.Sprintf("log-content-%d-%s.txt", row.Id, time.Unix(row.CreatedAt, 0).Format("20060102-150405"))
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(body))
}

// DownloadContentReviewList streams a whole filtered page range as text, one
// exchange per block, for offline review.
func DownloadContentReviewList(c *gin.Context) {
	username := c.Query("username")
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	// A bounded batch: 500 exchanges is plenty for one manual review pass and
	// keeps the response away from unbounded memory.
	rows, _, err := model.GetLogContentsPage(0, 500, username, startTimestamp, endTimestamp)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var b []byte
	b = append(b, []byte("ReX-API 内容审查导出 "+time.Now().Format("2006-01-02 15:04:05")+"\n\n")...)
	for _, r := range rows {
		header := fmt.Sprintf("──── id=%d user=%d model=%s token=%s time=%s%s\n",
			r.Id, r.UserId, r.ModelName, r.TokenName,
			time.Unix(r.CreatedAt, 0).Format("2006-01-02 15:04:05"),
			map[bool]string{true: " [截断]", false: ""}[r.Truncated])
		b = append(b, []byte(header)...)
		b = append(b, []byte("--- 请求 ---\n"+r.RequestText+"\n--- 回答 ---\n"+r.ResponseText+"\n\n")...)
	}
	filename := "content-review-" + time.Now().Format("20060102-150405") + ".txt"
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Data(http.StatusOK, "text/plain; charset=utf-8", b)
}

// DeleteContentReview removes one stored exchange.
func DeleteContentReview(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.LOG_DB.Delete(&model.LogContent{}, id).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// CleanupContentReview prunes rows past the retention window by hand.
func CleanupContentReview(c *gin.Context) {
	seconds := operation_setting.ContentRetentionSeconds()
	if seconds <= 0 {
		common.ApiErrorMsg(c, "未配置保留天数，跳过清理")
		return
	}
	affected, err := model.DeleteLogContentsBefore(time.Now().Unix() - seconds)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"deleted": affected})
}
