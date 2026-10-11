package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

// ContentReviewSetting governs the capture of prompt / response bodies for
// human review (the "content review" feature on the usage-log page).
//
// Capture is off by default: bodies are personal data, so an operator opts in,
// and the retention window bounds how long they stay.
type ContentReviewSetting struct {
	// Enabled turns body capture on. When off nothing is written and the
	// review UI hides itself.
	Enabled bool `json:"enabled"`
	// RetentionDays bounds how long a stored exchange is kept. 0 keeps
	// everything until deleted by hand.
	RetentionDays int `json:"retention_days"`
}

var contentReviewSetting = ContentReviewSetting{
	Enabled:       false,
	RetentionDays: 14,
}

func init() {
	config.GlobalConfig.Register("content_review_setting", &contentReviewSetting)
}

func GetContentReviewSetting() *ContentReviewSetting {
	return &contentReviewSetting
}

// ContentCaptureEnabled is the single gate every writer consults.
func ContentCaptureEnabled() bool {
	return contentReviewSetting.Enabled
}

// ContentRetentionSeconds converts the retention window; 0 means forever.
func ContentRetentionSeconds() int64 {
	days := contentReviewSetting.RetentionDays
	if days <= 0 {
		return 0
	}
	return int64(days) * 86400
}
