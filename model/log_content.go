package model

import (
	"time"

	"github.com/QuantumNous/new-api/common"
)

// LogContent stores the request text and the assistant reply of one relay
// request, for human content review. It is only written when the review switch
// is on, and it is keyed by request_id so the usage-log row (which the user can
// see) carries nothing but ids — the bodies live here, behind admin-only
// endpoints.
type LogContent struct {
	Id        int64  `json:"id" gorm:"primaryKey"`
	RequestId string `json:"request_id" gorm:"type:varchar(64);uniqueIndex"`
	UserId    int    `json:"user_id" gorm:"index"`
	CreatedAt int64  `json:"created_at" gorm:"bigint;index"`
	// RequestText is the prompt exactly as the client sent it (user+system
	// messages serialized), truncated to MaxStoredContentBytes.
	RequestText string `json:"request_text" gorm:"type:mediumtext"`
	// ResponseText is the assistant-visible reply (reasoning and tool payloads
	// are not kept), truncated the same way.
	ResponseText string `json:"response_text" gorm:"type:mediumtext"`
	ModelName    string `json:"model_name" gorm:"type:varchar(191);default:''"`
	TokenName    string `json:"token_name" gorm:"type:varchar(191);default:''"`
	Truncated    bool   `json:"truncated" gorm:"default:false"`
}

const (
	// MaxStoredContentBytes bounds one side of a stored exchange. 64 KiB is
	// enough for review purposes; a full transcript belongs to the caller.
	MaxStoredContentBytes = 64 << 10
)

func (LogContent) TableName() string {
	return "log_contents"
}

func truncateContent(text string) (string, bool) {
	if len(text) <= MaxStoredContentBytes {
		return text, false
	}
	return text[:MaxStoredContentBytes], true
}

// UpsertLogContent stores or replaces the bodies for one request. Failure to
// write is logged, never fatal: billing already happened, and losing a review
// sample is better than failing the request.
func UpsertLogContent(requestId string, userId int64, modelName, tokenName, requestText, responseText string) {
	if requestId == "" {
		return
	}
	reqText, reqTrunc := truncateContent(requestText)
	respText, respTrunc := truncateContent(responseText)
	row := LogContent{
		RequestId:    requestId,
		UserId:       int(userId),
		CreatedAt:    time.Now().Unix(),
		RequestText:  reqText,
		ResponseText: respText,
		ModelName:    modelName,
		TokenName:    tokenName,
		Truncated:    reqTrunc || respTrunc,
	}
	err := LOG_DB.Where("request_id = ?", requestId).
		Assign(row).FirstOrCreate(&row).Error
	if err != nil {
		common.SysError("log_content: upsert failed: " + err.Error())
	}
}

// GetLogContentById returns one stored exchange for an admin viewer.
func GetLogContentById(id int64) (*LogContent, error) {
	var row LogContent
	err := LOG_DB.First(&row, "id = ?", id).Error
	return &row, err
}

// GetLogContentByRequest returns the stored exchange for one request.
func GetLogContentByRequest(requestId string) (*LogContent, error) {
	var row LogContent
	err := LOG_DB.First(&row, "request_id = ?", requestId).Error
	return &row, err
}

// GetLogContentsPage lists stored exchanges, newest first.
func GetLogContentsPage(startIdx, num int, usernameLike string, startTimestamp, endTimestamp int64) ([]*LogContent, int64, error) {
	if startIdx < 0 {
		startIdx = 0
	}
	if num <= 0 || num > 200 {
		num = 50
	}
	query := LOG_DB.Model(&LogContent{})
	if usernameLike != "" {
		query = query.Where("user_id IN (?)",
			LOG_DB.Table("users").Select("id").Where("username LIKE ?", "%"+usernameLike+"%"))
	}
	if startTimestamp > 0 {
		query = query.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp > 0 {
		query = query.Where("created_at <= ?", endTimestamp)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []*LogContent
	err := query.Order("id desc").Offset(startIdx).Limit(num).Find(&rows).Error
	return rows, total, err
}

// DeleteLogContentsBefore prunes rows older than the retention window so the
// table stays small even with the switch left on.
func DeleteLogContentsBefore(cutoff int64) (int64, error) {
	result := LOG_DB.Where("created_at < ?", cutoff).Delete(&LogContent{})
	return result.RowsAffected, result.Error
}
