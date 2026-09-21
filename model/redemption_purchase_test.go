package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newRedemptionPurchaseUser(t *testing.T, quota int) *User {
	t.Helper()
	user := &User{Username: "code-buyer-" + t.Name(), Password: "password", Quota: quota, Status: common.UserStatusEnabled, Group: "default"}
	require.NoError(t, DB.Create(user).Error)
	return user
}

func newRedemptionPurchaseOrder(t *testing.T, user *User, tradeNo string, count int, unitQuota int, unitAmount int64) *TopUp {
	t.Helper()
	topUp := &TopUp{
		UserId:           user.Id,
		Amount:           unitAmount * int64(count),
		Money:            float64(unitAmount * int64(count)),
		TradeNo:          tradeNo,
		PaymentMethod:    "alipay",
		PaymentProvider:  PaymentProviderEpay,
		CreateTime:       time.Now().Unix(),
		Status:           common.TopUpStatusPending,
		OrderType:        OrderTypeRedemption,
		RedemptionQuota:  unitQuota,
		RedemptionCount:  count,
		RedemptionAmount: unitAmount,
		RedemptionName:   "Code test",
	}
	require.NoError(t, topUp.Insert())
	return topUp
}

func TestRedemptionPurchaseMintsCodesWithoutCreditingTheWallet(t *testing.T) {
	truncateTables(t)
	const startingQuota = 5000
	user := newRedemptionPurchaseUser(t, startingQuota)
	newRedemptionPurchaseOrder(t, user, "RC-MINT-1", 3, 1234, 10)

	_, err := RechargeEpay("RC-MINT-1", "alipay", "192.0.2.1")
	require.NoError(t, err)

	// The wallet must be exactly as it was: no credit, and no debit either.
	fresh, err := GetUserById(user.Id, true)
	require.NoError(t, err)
	assert.Equal(t, startingQuota, fresh.Quota, "a code purchase must not move wallet balance")

	var codes []Redemption
	require.NoError(t, DB.Where("purchase_trade_no = ?", "RC-MINT-1").Find(&codes).Error)
	require.Len(t, codes, 3, "one code per unit purchased")
	for _, code := range codes {
		assert.Equal(t, 1234, code.Quota)
		assert.Equal(t, user.Id, code.UserId, "the buyer owns the codes")
		assert.Equal(t, common.RedemptionCodeStatusEnabled, code.Status)
		assert.NotEmpty(t, code.Key)
	}

	var stored TopUp
	require.NoError(t, DB.Where("trade_no = ?", "RC-MINT-1").First(&stored).Error)
	assert.Equal(t, common.TopUpStatusSuccess, stored.Status)

	// The codes have to be worth something: redeeming one credits its face value.
	redeemedQuota, err := Redeem(codes[0].Key, user.Id)
	require.NoError(t, err)
	assert.Equal(t, 1234, redeemedQuota)

	after, err := GetUserById(user.Id, true)
	require.NoError(t, err)
	assert.Equal(t, startingQuota+1234, after.Quota)
}

// A payment provider retries callbacks, so the same order must not mint twice.
func TestRedemptionPurchaseIsIdempotent(t *testing.T) {
	truncateTables(t)
	user := newRedemptionPurchaseUser(t, 100)
	newRedemptionPurchaseOrder(t, user, "RC-IDEMPOTENT-1", 2, 500, 5)

	_, err := RechargeEpay("RC-IDEMPOTENT-1", "alipay", "192.0.2.1")
	require.NoError(t, err)

	alreadyDone, err := RechargeEpay("RC-IDEMPOTENT-1", "alipay", "192.0.2.1")
	require.NoError(t, err)
	assert.True(t, alreadyDone, "the second callback is recognised as a duplicate")

	var count int64
	require.NoError(t, DB.Model(&Redemption{}).Where("purchase_trade_no = ?", "RC-IDEMPOTENT-1").Count(&count).Error)
	assert.EqualValues(t, 2, count, "a repeated callback must not mint more codes")
}

// Terms that would mint an unbounded number of codes must fail the whole
// settlement, so the order is not marked paid and nothing is minted.
func TestRedemptionPurchaseWithInvalidTermsMintsNothing(t *testing.T) {
	truncateTables(t)
	user := newRedemptionPurchaseUser(t, 100)
	newRedemptionPurchaseOrder(t, user, "RC-INVALID-1", MaxRedemptionPurchaseCount+1, 500, 5)

	_, err := RechargeEpay("RC-INVALID-1", "alipay", "192.0.2.1")
	require.ErrorIs(t, err, ErrInvalidTopUpQuota)

	var count int64
	require.NoError(t, DB.Model(&Redemption{}).Where("purchase_trade_no = ?", "RC-INVALID-1").Count(&count).Error)
	assert.Zero(t, count)

	var stored TopUp
	require.NoError(t, DB.Where("trade_no = ?", "RC-INVALID-1").First(&stored).Error)
	assert.Equal(t, common.TopUpStatusPending, stored.Status, "a failed mint must leave the order unpaid")

	fresh, err := GetUserById(user.Id, true)
	require.NoError(t, err)
	assert.Equal(t, 100, fresh.Quota)
}

// An ordinary top-up must still credit the wallet: the redemption branch must
// not have captured the normal path.
func TestOrdinaryTopUpStillCreditsTheWallet(t *testing.T) {
	truncateTables(t)
	const startingQuota = 5000
	user := &User{Username: "topup-user", Password: "password", Quota: startingQuota, Status: common.UserStatusEnabled, Group: "default"}
	require.NoError(t, DB.Create(user).Error)

	topUp := &TopUp{
		UserId:          user.Id,
		Amount:          10,
		Money:           10,
		TradeNo:         "RC-PLAIN-1",
		PaymentMethod:   "alipay",
		PaymentProvider: PaymentProviderEpay,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, topUp.Insert())

	_, err := RechargeEpay("RC-PLAIN-1", "alipay", "192.0.2.1")
	require.NoError(t, err)

	expectedQuota, err := common.WalletQuotaFromDecimalStrict(
		decimal.NewFromInt(10).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
	)
	require.NoError(t, err)

	fresh, err := GetUserById(user.Id, true)
	require.NoError(t, err)
	assert.Equal(t, startingQuota+expectedQuota, fresh.Quota)

	var count int64
	require.NoError(t, DB.Model(&Redemption{}).Count(&count).Error)
	assert.Zero(t, count, "a wallet top-up mints no codes")
}

func setupRedemptionPurchaseModelTest(t *testing.T, quota int) *User {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&Redemption{}))

	user := &User{
		Username: fmt.Sprintf("redemption-purchase-%d", time.Now().UnixNano()),
		AffCode:  fmt.Sprintf("redemption-aff-%d", time.Now().UnixNano()),
		Status:   common.UserStatusEnabled,
		Quota:    quota,
	}
	require.NoError(t, DB.Create(user).Error)

	t.Cleanup(func() {
		DB.Where("owner_id = ? OR user_id = ? OR used_user_id = ?", user.Id, user.Id, user.Id).
			Unscoped().Delete(&Redemption{})
		DB.Where("user_id = ?", user.Id).Delete(&TopUp{})
		DB.Where("user_id = ?", user.Id).Delete(&Log{})
		DB.Delete(&User{}, user.Id)
	})
	return user
}

func TestRechargeEpayRedemptionPurchaseMintsCodesWithoutWalletCredit(t *testing.T) {
	user := setupRedemptionPurchaseModelTest(t, 123)
	tradeNo := fmt.Sprintf("redemption-purchase-settlement-%d", time.Now().UnixNano())
	topUp := &TopUp{
		UserId:           user.Id,
		Amount:           100,
		Money:            100,
		TradeNo:          tradeNo,
		PaymentMethod:    "alipay",
		PaymentProvider:  PaymentProviderEpay,
		CreateTime:       common.GetTimestamp(),
		Status:           common.TopUpStatusPending,
		OrderType:        OrderTypeRedemption,
		RedemptionQuota:  500,
		RedemptionCount:  20,
		RedemptionAmount: 5,
		RedemptionName:   "Code 5",
	}
	require.NoError(t, topUp.Insert())
	t.Cleanup(func() {
		DB.Where("trade_no = ?", tradeNo).Delete(&TopUp{})
		DB.Where("purchase_trade_no = ?", tradeNo).Unscoped().Delete(&Redemption{})
	})

	alreadyDone, err := RechargeEpay(tradeNo, "alipay", "127.0.0.1")
	require.NoError(t, err)
	assert.False(t, alreadyDone)
	assert.Equal(t, 123, getRedemptionPurchaseUserQuota(t, user.Id))

	var codes []Redemption
	require.NoError(t, DB.Where("purchase_trade_no = ?", tradeNo).Order("id asc").Find(&codes).Error)
	require.Len(t, codes, 20)
	for _, code := range codes {
		assert.Equal(t, user.Id, code.OwnerId)
		assert.Equal(t, user.Id, code.UserId)
		assert.Equal(t, RedemptionCreatorUser, code.CreatorType)
		assert.Equal(t, 500, code.Quota)
		assert.Equal(t, common.RedemptionCodeStatusEnabled, code.Status)
		assert.Equal(t, int64(5), code.PurchaseAmount)
	}

	alreadyDone, err = RechargeEpay(tradeNo, "alipay", "127.0.0.1")
	require.NoError(t, err)
	assert.True(t, alreadyDone)
	assert.Equal(t, 123, getRedemptionPurchaseUserQuota(t, user.Id))
	var codeCount int64
	require.NoError(t, DB.Model(&Redemption{}).Where("purchase_trade_no = ?", tradeNo).Count(&codeCount).Error)
	assert.Equal(t, int64(20), codeCount)

	settled := GetTopUpByTradeNo(tradeNo)
	require.NotNil(t, settled)
	assert.Equal(t, common.TopUpStatusSuccess, settled.Status)
}

func TestRefundPurchasedRedemptionEnforcesOwnershipAndUsage(t *testing.T) {
	user := setupRedemptionPurchaseModelTest(t, 100)
	otherUser := setupRedemptionPurchaseModelTest(t, 0)

	owned := createPurchasedRedemptionForTest(t, user.Id, "refund-owned", 500)
	used := createPurchasedRedemptionForTest(t, user.Id, "refund-used", 700)
	require.NoError(t, DB.Model(&Redemption{}).Where("id = ?", used.Id).Updates(map[string]interface{}{
		"status":        common.RedemptionCodeStatusUsed,
		"used_user_id":  otherUser.Id,
		"redeemed_time": common.GetTimestamp(),
	}).Error)
	unowned := createPurchasedRedemptionForTest(t, otherUser.Id, "refund-unowned", 900)
	admin := &Redemption{
		UserId:          user.Id,
		OwnerId:         user.Id,
		CreatorType:     RedemptionCreatorAdmin,
		Key:             common.GetUUID(),
		Status:          common.RedemptionCodeStatusEnabled,
		Quota:           1100,
		PurchaseTradeNo: "admin-created-code",
		PurchaseAmount:  11,
		CreatedTime:     common.GetTimestamp(),
	}
	require.NoError(t, DB.Create(admin).Error)

	refundedQuota, err := RefundPurchasedRedemption(owned.Id, user.Id)
	require.NoError(t, err)
	assert.Equal(t, 500, refundedQuota)
	assert.Equal(t, 600, getRedemptionPurchaseUserQuota(t, user.Id))

	var refunded Redemption
	require.NoError(t, DB.First(&refunded, owned.Id).Error)
	assert.Equal(t, common.RedemptionCodeStatusDisabled, refunded.Status)
	assert.NotZero(t, refunded.RefundedTime)

	_, err = RefundPurchasedRedemption(owned.Id, user.Id)
	assert.ErrorIs(t, err, ErrRedemptionRefundNotAllowed)
	assert.Equal(t, 600, getRedemptionPurchaseUserQuota(t, user.Id))

	_, err = RefundPurchasedRedemption(used.Id, user.Id)
	assert.ErrorIs(t, err, ErrRedemptionRefundNotAllowed)
	_, err = RefundPurchasedRedemption(admin.Id, user.Id)
	assert.ErrorIs(t, err, ErrRedemptionNotOwned)
	_, err = RefundPurchasedRedemption(unowned.Id, user.Id)
	assert.ErrorIs(t, err, ErrRedemptionNotOwned)
}

func TestRedemptionCreatorFilterTreatsLegacyRowsAsAdmin(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Redemption{}))
	rows := []*Redemption{
		{
			Name:        "filter-admin",
			Key:         common.GetUUID(),
			Status:      common.RedemptionCodeStatusEnabled,
			CreatorType: RedemptionCreatorAdmin,
			CreatedTime: common.GetTimestamp(),
		},
		{
			Name:        "filter-legacy",
			Key:         common.GetUUID(),
			Status:      common.RedemptionCodeStatusEnabled,
			CreatedTime: common.GetTimestamp(),
		},
		{
			Name:        "filter-user",
			Key:         common.GetUUID(),
			Status:      common.RedemptionCodeStatusEnabled,
			CreatorType: RedemptionCreatorUser,
			OwnerId:     99001,
			CreatedTime: common.GetTimestamp(),
		},
	}
	require.NoError(t, DB.Create(&rows).Error)
	ids := make([]int, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.Id)
	}
	t.Cleanup(func() {
		DB.Unscoped().Where("id IN ?", ids).Delete(&Redemption{})
	})

	userRows, userTotal, err := SearchRedemptions("filter-", "", 0, 20, RedemptionCreatorUser)
	require.NoError(t, err)
	assert.Equal(t, int64(1), userTotal)
	require.Len(t, userRows, 1)
	assert.Equal(t, RedemptionCreatorUser, userRows[0].CreatorType)

	adminRows, adminTotal, err := SearchRedemptions("filter-", "", 0, 20, RedemptionCreatorAdmin)
	require.NoError(t, err)
	assert.Equal(t, int64(2), adminTotal)
	assert.Len(t, adminRows, 2)
	for _, row := range adminRows {
		assert.NotEqual(t, RedemptionCreatorUser, row.CreatorType)
	}
}

func createPurchasedRedemptionForTest(t *testing.T, userID int, tradeNo string, quota int) *Redemption {
	t.Helper()
	var redemptions []*Redemption
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		redemptions, err = CreatePurchasedRedemptionsTx(tx, userID, "Purchased code", quota, int64(quota/100), 1, tradeNo)
		return err
	})
	require.NoError(t, err)
	require.Len(t, redemptions, 1)
	t.Cleanup(func() {
		DB.Unscoped().Where("purchase_trade_no = ?", tradeNo).Delete(&Redemption{})
	})
	return redemptions[0]
}

func getRedemptionPurchaseUserQuota(t *testing.T, userID int) int {
	t.Helper()
	var user User
	require.NoError(t, DB.Select("quota").First(&user, userID).Error)
	return user.Quota
}
