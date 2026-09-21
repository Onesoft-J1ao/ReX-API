package model

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// PriceSentinelReport is one hourly price check, kept so the console can show
// history and so a mailed edit link knows what it was reporting.
//
// A report is a snapshot, not a queue: the next check writes a new row. The
// change list is stored as JSON because it is only ever read whole.
type PriceSentinelReport struct {
	Id int `json:"id" gorm:"primaryKey"`
	// CheckedAt is the Unix time the upstream document was fetched.
	CheckedAt int64 `json:"checked_at" gorm:"index"`
	// UpstreamUrl records which reference site produced this report, so a
	// report stays interpretable after the setting is repointed.
	UpstreamUrl string `json:"upstream_url" gorm:"type:varchar(255)"`
	// Compared is how many models both sides carry; Changed is how many of
	// those differ beyond the noise floor.
	Compared int `json:"compared"`
	Changed  int `json:"changed"`
	// HigherPrice and LowerPrice split the changes by direction: a model we
	// charge more for risks losing customers, one we charge less for is margin
	// being given away.
	HigherPrice int `json:"higher_price"`
	LowerPrice  int `json:"lower_price"`
	// Changes is the serialized []PriceChange.
	Changes string `json:"changes" gorm:"type:text"`
	// FetchError is non-empty when the upstream could not be read; the row is
	// still written so the operator learns the check is failing.
	FetchError string `json:"fetch_error" gorm:"type:varchar(512)"`
	// EmailSentAt is when the digest went out, 0 when it has not (or failed).
	EmailSentAt int64 `json:"email_sent_at"`
	CreatedAt   int64 `json:"created_at" gorm:"index"`
}

func (PriceSentinelReport) TableName() string { return "price_sentinel_reports" }

// PriceChange is one model whose price differs from the reference site.
type PriceChange struct {
	ModelName string `json:"model_name"`
	// Field is the price dimension that moved: model_ratio, model_price,
	// completion_ratio, cache_ratio.
	Field string `json:"field"`
	// Ours and Theirs are the raw values. Ours may be absent (nil) when we
	// never configured the model, which reads as "upstream charges, we do not".
	Ours   *float64 `json:"ours"`
	Theirs *float64 `json:"theirs"`
	// Percent is the signed change from ours to theirs. Positive means the
	// upstream is more expensive than us.
	Percent float64 `json:"percent"`
	// Groups lists the groups serving this model, so the operator can tell a
	// paid model from a free one at a glance.
	Groups []string `json:"groups,omitempty"`
}

// PriceSentinelEditToken authorizes one price edit from a mailed link.
//
// The token is not an admin credential: it only unlocks the edit page, which
// still requires the operator's access token. It is single-use and expiring,
// and the digest of its payload is stored so the token itself is never
// recoverable from the database.
type PriceSentinelEditToken struct {
	Id        int    `json:"id" gorm:"primaryKey"`
	TokenHash string `json:"-" gorm:"type:varchar(64);uniqueIndex"`
	// ReportId is the report the link came from, so the page can show what was
	// proposed at the time the mail was written.
	ReportId int `json:"report_id" gorm:"index"`
	// ExpiresAt and UsedAt bound the token's life. A used token is kept so a
	// replay is refused rather than silently accepted.
	ExpiresAt int64 `json:"expires_at" gorm:"index"`
	UsedAt    int64 `json:"used_at"`
	CreatedAt int64 `json:"created_at"`
}

func (PriceSentinelEditToken) TableName() string { return "price_sentinel_edit_tokens" }

// SentinelTokenTTLHours mirrors the setting without importing it, so the model
// package stays free of setting dependencies at call sites that only need a
// default.
const SentinelTokenTTLHours = 24

// CreatePriceSentinelEditToken mints a token for one report and returns the
// plaintext, which is the only time it exists outside the mail.
func CreatePriceSentinelEditToken(reportId int, ttlHours int) (string, error) {
	if reportId <= 0 {
		return "", errors.New("invalid report id")
	}
	if ttlHours < 1 {
		ttlHours = SentinelTokenTTLHours
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)
	now := common.GetTimestamp()
	record := &PriceSentinelEditToken{
		TokenHash: HashPriceSentinelToken(token),
		ReportId:  reportId,
		ExpiresAt: now + int64(ttlHours)*3600,
		CreatedAt: now,
	}
	if err := DB.Create(record).Error; err != nil {
		return "", err
	}
	return token, nil
}

// HashPriceSentinelToken derives the stored form. SHA-256 over the token is
// enough here: the token is 256 bits of randomness, so there is no dictionary
// to attack and no need for a slow KDF.
func HashPriceSentinelToken(token string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(token)))
}

// ConsumePriceSentinelEditToken validates a token and marks it used in one
// transaction, so two concurrent clicks cannot both succeed.
func ConsumePriceSentinelEditToken(token string) (*PriceSentinelEditToken, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, errors.New("token is empty")
	}
	hash := HashPriceSentinelToken(token)
	var result *PriceSentinelEditToken
	err := DB.Transaction(func(tx *gorm.DB) error {
		var record PriceSentinelEditToken
		if err := lockForUpdate(tx).Where("token_hash = ?", hash).First(&record).Error; err != nil {
			return errors.New("edit link is not valid")
		}
		if record.UsedAt > 0 {
			return errors.New("edit link has already been used")
		}
		if record.ExpiresAt > 0 && common.GetTimestamp() > record.ExpiresAt {
			return errors.New("edit link has expired")
		}
		record.UsedAt = common.GetTimestamp()
		if err := tx.Save(&record).Error; err != nil {
			return err
		}
		result = &record
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// GetPriceSentinelReport returns one report by id.
func GetPriceSentinelReport(id int) (*PriceSentinelReport, error) {
	var report PriceSentinelReport
	if err := DB.First(&report, id).Error; err != nil {
		return nil, err
	}
	return &report, nil
}

// InsertPriceSentinelReport writes one check's result.
func InsertPriceSentinelReport(report *PriceSentinelReport) error {
	if report == nil {
		return nil
	}
	if report.CreatedAt == 0 {
		report.CreatedAt = common.GetTimestamp()
	}
	return DB.Create(report).Error
}

// MarkPriceSentinelReportEmailed records that the digest went out.
func MarkPriceSentinelReportEmailed(id int, at int64) error {
	if id <= 0 {
		return nil
	}
	if at == 0 {
		at = common.GetTimestamp()
	}
	return DB.Model(&PriceSentinelReport{}).Where("id = ?", id).
		Update("email_sent_at", at).Error
}

// GetLatestPriceSentinelReport returns the most recent check, or nil when none
// has run.
func GetLatestPriceSentinelReport() (*PriceSentinelReport, error) {
	var report PriceSentinelReport
	err := DB.Order("checked_at DESC").First(&report).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &report, nil
}

// FindPriceSentinelEditToken looks a token up without consuming it, so the edit
// page can reject a dead link before the operator types anything.
func FindPriceSentinelEditToken(token string) (*PriceSentinelEditToken, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, errors.New("token is empty")
	}
	var record PriceSentinelEditToken
	if err := DB.Where("token_hash = ?", HashPriceSentinelToken(token)).First(&record).Error; err != nil {
		return nil, errors.New("edit link is not valid")
	}
	if record.UsedAt > 0 {
		return nil, errors.New("edit link has already been used")
	}
	if record.ExpiresAt > 0 && common.GetTimestamp() > record.ExpiresAt {
		return nil, errors.New("edit link has expired")
	}
	return &record, nil
}

// GetRootUserEmail returns the root account's address, which is where operator
// digests land when no explicit recipient is configured.
func GetRootUserEmail() (string, error) {
	root := GetRootUser()
	if root == nil {
		return "", errors.New("root user not found")
	}
	return strings.TrimSpace(root.Email), nil
}

// DeletePriceSentinelReportsBefore prunes history past the retention window.
func DeletePriceSentinelReportsBefore(cutoff int64) error {
	if cutoff <= 0 {
		return nil
	}
	if err := DB.Where("checked_at < ?", cutoff).Delete(&PriceSentinelReport{}).Error; err != nil {
		return err
	}
	return DB.Where("expires_at < ?", cutoff).Delete(&PriceSentinelEditToken{}).Error
}
