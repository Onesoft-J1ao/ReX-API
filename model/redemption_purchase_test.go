package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A paid code purchase must hand over codes and nothing else: the money bought
// codes, so crediting the wallet as well would give the buyer both.
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
