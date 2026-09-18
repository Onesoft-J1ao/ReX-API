package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// Wallet balance split.
//
// User.Quota stays the single spendable wallet balance so every existing fast
// path (pre-consume reservation, Redis cache, batch updates) keeps working
// unchanged. User.GiftQuota is the non-withdrawable portion *inside* that
// balance: gifted credit and plan refunds land here, and only
// quota-minus-gift is treated as withdrawable cash.
//
// The split is therefore an attribution, never a second source of spendable
// quota: crediting gifted quota always raises quota and gift_quota together, so
// the split can mis-attribute a balance but can never create or destroy it.

var ErrInvalidGiftQuotaAmount = errors.New("invalid gift quota amount")

// GetUserGiftQuota reads the non-withdrawable portion of a user's wallet.
func GetUserGiftQuota(id int) (int, error) {
	if id <= 0 {
		return 0, errors.New("invalid userId")
	}
	var giftQuota int
	if err := DB.Model(&User{}).Where("id = ?", id).Select("gift_quota").Scan(&giftQuota).Error; err != nil {
		return 0, err
	}
	return giftQuota, nil
}

// CreditUserGiftQuota adds non-withdrawable balance to the wallet. It moves
// quota and gift_quota in one statement so the gifted portion can never exceed
// the balance it is part of, and fails when the wallet ceiling would be
// exceeded, exactly like IncreaseUserQuota.
func CreditUserGiftQuota(id int, amount int, db bool) error {
	if amount < 0 {
		return ErrInvalidGiftQuotaAmount
	}
	if amount == 0 {
		return nil
	}
	if err := common.ValidateWalletQuota(amount); err != nil {
		return err
	}
	result := DB.Model(&User{}).
		Where("id = ? AND quota <= ?", id, common.MaxWalletQuota-amount).
		Updates(map[string]any{
			"quota":      gorm.Expr("quota + ?", amount),
			"gift_quota": gorm.Expr("gift_quota + ?", amount),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		var count int64
		if err := DB.Model(&User{}).Where("id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return gorm.ErrRecordNotFound
		}
		return ErrWalletQuotaLimitExceeded
	}
	syncCreditUserQuotaCache(id, amount, "gift quota credit")
	return nil
}

// TryConsumeUserGiftQuota burns gifted balance first, returning how much was
// actually taken (never more than the balance available). The caller has
// already deducted the same amount from User.Quota, so this only maintains the
// attribution. Gifted credit burns before cash so a user cannot convert promo
// credit into withdrawable balance by spending against it.
func TryConsumeUserGiftQuota(id int, amount int) (int, error) {
	if amount < 0 {
		return 0, ErrInvalidGiftQuotaAmount
	}
	if amount == 0 {
		return 0, nil
	}
	consumed := 0
	err := DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).Select("gift_quota").Where("id = ?", id).First(&user).Error; err != nil {
			return err
		}
		consumed = min(user.GiftQuota, amount)
		if consumed <= 0 {
			return nil
		}
		return tx.Model(&User{}).Where("id = ?", id).
			Update("gift_quota", gorm.Expr("gift_quota - ?", consumed)).Error
	})
	if err != nil {
		return 0, err
	}
	return consumed, nil
}

// RestoreUserGiftQuota returns gifted credit that was burned by a refunded
// pre-consume. It clamps at the current balance: if the balance cannot carry
// the full amount, everything it can is attributed to the gift bucket, because
// under-attributing would reclassify promo credit as withdrawable cash.
func RestoreUserGiftQuota(id int, amount int, db bool) error {
	if amount < 0 {
		return ErrInvalidGiftQuotaAmount
	}
	if amount == 0 {
		return nil
	}
	result := DB.Model(&User{}).
		Where("id = ? AND gift_quota + ? <= quota", id, amount).
		Update("gift_quota", gorm.Expr("gift_quota + ?", amount))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}
	return DB.Model(&User{}).Where("id = ? AND gift_quota < quota", id).
		Update("gift_quota", gorm.Expr("quota")).Error
}

// RefundableCashQuota is the withdrawable part of a wallet balance: what the
// user paid for in cash, excluding gifted and plan credit.
func RefundableCashQuota(user *User) int {
	if user == nil || user.Quota <= 0 {
		return 0
	}
	cash := user.Quota - user.GiftQuota
	if cash < 0 {
		return 0
	}
	return cash
}
