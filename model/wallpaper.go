package model

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// WallpaperImageURLMaxLength bounds the stored address. It is also what rules
// out an inline `data:` payload, the only way a free-form string field could
// smuggle an unbounded blob into the row.
const WallpaperImageURLMaxLength = 2048

// wallpaperURLUnsafeChars are rejected rather than escaped: the console quotes
// the stored value into a CSS `url("...")`, so quotes, parentheses and
// backslashes could otherwise break out of that context.
const wallpaperURLUnsafeChars = "\"'()\\<>`"

var (
	ErrWallpaperDisabled     = errors.New("自定义壁纸未启用")
	ErrWallpaperURLRequired  = errors.New("壁纸地址不能为空")
	ErrWallpaperURLTooLong   = errors.New("壁纸地址过长")
	ErrWallpaperURLInvalid   = errors.New("壁纸地址无效")
	ErrWallpaperUnchanged    = errors.New("壁纸未发生变化")
	ErrWallpaperMonthlyLimit = errors.New("本月自定义壁纸更换次数已用完")
	ErrWallpaperInsufficient = errors.New("余额不足")
)

// UserWallpaper is the console wallpaper one user has chosen plus the paid
// change counter for the calendar month that choice belongs to. Both live in
// one row so that charging the wallet, counting the change and recording the
// new image commit together.
type UserWallpaper struct {
	Id           int    `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId       int    `json:"user_id" gorm:"not null;uniqueIndex:idx_user_wallpaper_user"`
	WallpaperUrl string `json:"wallpaper_url" gorm:"type:varchar(2048);not null"`
	Period       string `json:"period" gorm:"type:varchar(7);not null"` // 格式: YYYY-MM
	ChangeCount  int    `json:"change_count" gorm:"not null"`
	CreatedAt    int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt    int64  `json:"updated_at" gorm:"bigint"`
}

func (UserWallpaper) TableName() string {
	return "user_wallpapers"
}

// UserWallpaperStatus is the read shape the console needs before it offers a
// change: what is currently set, and what one more change would cost and count
// against.
type UserWallpaperStatus struct {
	Enabled          bool    `json:"enabled"`
	PriceYuan        float64 `json:"price_yuan"`
	MonthlyLimit     int     `json:"monthly_limit"`
	WallpaperUrl     string  `json:"wallpaper_url"`
	Period           string  `json:"period"`
	ChangesUsed      int     `json:"changes_used"`
	ChangesRemaining int     `json:"changes_remaining"`
}

// WallpaperPeriod names the calendar month a change is counted in. A new month
// starts a fresh allowance; the already-painted wallpaper survives the reset.
func WallpaperPeriod(t time.Time) string {
	return t.Format("2006-01")
}

// NormalizeWallpaperURL validates a user-supplied wallpaper address and returns
// the value to store. Only absolute http(s) URLs are accepted, and only
// printable ASCII without the CSS-hostile characters above, which keeps the
// value safe both as a stored row and as a CSS `url("...")` on the console.
func NormalizeWallpaperURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", ErrWallpaperURLRequired
	}
	if len(trimmed) > WallpaperImageURLMaxLength {
		return "", ErrWallpaperURLTooLong
	}
	if strings.ContainsAny(trimmed, wallpaperURLUnsafeChars) {
		return "", ErrWallpaperURLInvalid
	}
	for _, r := range trimmed {
		if r < 0x21 || r > 0x7e {
			return "", ErrWallpaperURLInvalid
		}
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", ErrWallpaperURLInvalid
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", ErrWallpaperURLInvalid
	}
	if parsed.Host == "" || parsed.User != nil {
		return "", ErrWallpaperURLInvalid
	}
	return trimmed, nil
}

// WallpaperQuota converts the configured 元 price of one change into quota.
// Mirrors the subscription balance-purchase conversion: decimal arithmetic,
// then the strict wallet-range helper, so a misconfigured price fails loudly
// instead of saturating into a bogus charge.
func WallpaperQuota(priceYuan float64) (int, error) {
	if priceYuan <= 0 {
		return 0, nil
	}
	if common.QuotaPerUnit <= 0 {
		return 0, errors.New("额度单位配置错误")
	}
	if operation_setting.USDExchangeRate <= 0 {
		return 0, errors.New("汇率配置错误")
	}
	quota := decimal.NewFromFloat(priceYuan).
		Div(decimal.NewFromFloat(operation_setting.USDExchangeRate)).
		Mul(decimal.NewFromFloat(common.QuotaPerUnit)).
		Ceil()
	return common.WalletQuotaFromDecimalStrict(quota)
}

// GetUserWallpaperStatus reports the wallpaper and the current period's
// allowance. A row left over from an earlier month reports zero used changes
// while still serving the stored image.
func GetUserWallpaperStatus(userId int) (*UserWallpaperStatus, error) {
	if userId <= 0 {
		return nil, errors.New("invalid userId")
	}
	setting := operation_setting.GetWallpaperSetting()
	status := &UserWallpaperStatus{
		Enabled:      setting.Enabled,
		PriceYuan:    setting.PriceYuan,
		MonthlyLimit: setting.MonthlyChangeLimit,
		Period:       WallpaperPeriod(time.Now()),
	}
	used := 0
	var row UserWallpaper
	err := DB.Where("user_id = ?", userId).First(&row).Error
	switch {
	case err == nil:
		status.WallpaperUrl = row.WallpaperUrl
		if row.Period == status.Period {
			used = row.ChangeCount
		}
	case errors.Is(err, gorm.ErrRecordNotFound):
		// No wallpaper yet: the caller still gets the price and allowance.
	default:
		return nil, err
	}
	status.ChangesUsed = used
	status.ChangesRemaining = -1 // unlimited
	if setting.MonthlyChangeLimit > 0 {
		remaining := setting.MonthlyChangeLimit - used
		status.ChangesRemaining = max(remaining, 0)
	}
	return status, nil
}

// SetUserWallpaper performs one paid change: it charges the owner's wallet and
// records the new wallpaper in a single transaction, so a failed charge never
// consumes a change and a consumed change is never left uncharged. The user row
// is locked first, which serializes concurrent changes for the same account on
// MySQL and PostgreSQL; SQLite serializes writers at the database level.
func SetUserWallpaper(userId int, rawURL string) (*UserWallpaper, error) {
	if userId <= 0 {
		return nil, errors.New("invalid userId")
	}
	setting := operation_setting.GetWallpaperSetting()
	if !setting.Enabled {
		return nil, ErrWallpaperDisabled
	}
	wallpaperURL, err := NormalizeWallpaperURL(rawURL)
	if err != nil {
		return nil, err
	}
	quota, err := WallpaperQuota(setting.PriceYuan)
	if err != nil {
		return nil, err
	}

	now := common.GetTimestamp()
	period := WallpaperPeriod(time.Now())
	var chargedQuota int
	var saved UserWallpaper
	err = DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).Where("id = ?", userId).First(&user).Error; err != nil {
			return err
		}

		var row UserWallpaper
		rowErr := lockForUpdate(tx).Where("user_id = ?", userId).First(&row).Error
		if rowErr != nil && !errors.Is(rowErr, gorm.ErrRecordNotFound) {
			return rowErr
		}
		exists := rowErr == nil
		if exists && row.WallpaperUrl == wallpaperURL {
			return ErrWallpaperUnchanged
		}

		used := 0
		if exists && row.Period == period {
			used = row.ChangeCount
		}
		if setting.MonthlyChangeLimit > 0 && used >= setting.MonthlyChangeLimit {
			return ErrWallpaperMonthlyLimit
		}

		if quota > 0 {
			if user.Quota < quota {
				return ErrWallpaperInsufficient
			}
			result := tx.Model(&User{}).Where("id = ?", userId).
				Update("quota", gorm.Expr("quota - ?", quota))
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return gorm.ErrRecordNotFound
			}
		}

		if !exists {
			row = UserWallpaper{
				UserId:       userId,
				WallpaperUrl: wallpaperURL,
				Period:       period,
				ChangeCount:  1,
				CreatedAt:    now,
				UpdatedAt:    now,
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		} else {
			// Increment in SQL so two writers cannot both store the same
			// count; a rolled-over period restarts the allowance at one.
			countUpdate := any(gorm.Expr("change_count + ?", 1))
			if row.Period != period {
				countUpdate = 1
			}
			result := tx.Model(&UserWallpaper{}).Where("id = ?", row.Id).Updates(map[string]any{
				"wallpaper_url": wallpaperURL,
				"period":        period,
				"change_count":  countUpdate,
				"updated_at":    now,
			})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return gorm.ErrRecordNotFound
			}
			row.WallpaperUrl = wallpaperURL
			row.Period = period
			row.ChangeCount = used + 1
			row.UpdatedAt = now
		}

		chargedQuota = quota
		saved = row
		return nil
	})
	if err != nil {
		return nil, err
	}

	// The wallet cache holds outstanding reservations, so it must be moved by
	// the committed difference only, exactly like the other balance spenders.
	if chargedQuota > 0 {
		if err := cacheDecrUserQuota(userId, int64(chargedQuota)); err != nil {
			common.SysError("failed to decrease user quota cache after wallpaper change: " + err.Error())
		}
	}
	// Logged as a system event, not a consume: this spends the wallet but is
	// not a model request, so it must not distort consume-log statistics.
	RecordLog(userId, LogTypeSystem, fmt.Sprintf("更换自定义壁纸，扣除额度 %d", chargedQuota))
	return &saved, nil
}

// DeleteUserWallpaper clears the stored image without consuming a change: it
// only takes away the caller's own wallpaper, so it cannot be used to gain a
// fresh allowance.
func DeleteUserWallpaper(userId int) error {
	if userId <= 0 {
		return errors.New("invalid userId")
	}
	return DB.Model(&UserWallpaper{}).Where("user_id = ?", userId).
		Updates(map[string]any{
			"wallpaper_url": "",
			"updated_at":    common.GetTimestamp(),
		}).Error
}
