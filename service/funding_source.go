package service

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// ---------------------------------------------------------------------------
// FundingSource — 资金来源接口（钱包 or 订阅）
// ---------------------------------------------------------------------------

// FundingSource 抽象了预扣费的资金来源。
type FundingSource interface {
	// Source 返回资金来源标识："wallet" 或 "subscription"
	Source() string
	// PreConsume 从该资金来源预扣 amount 额度
	PreConsume(amount int) error
	// Settle 根据差额调整资金来源（正数补扣，负数退还）
	Settle(delta int) error
	// Refund 退还所有预扣费
	Refund() error
}

// ---------------------------------------------------------------------------
// FreeFunding — 游乐场免费额度资金来源实现
// ---------------------------------------------------------------------------

// BillingSourceFree marks a request funded by the playground free allowance.
const BillingSourceFree = "free"

// FreeFunding funds a playground request the free allowance still covers. Every
// operation is a no-op: the wallet and any subscription are left alone, and the
// allowance itself is recorded separately once the request has produced usage.
type FreeFunding struct{}

func (f *FreeFunding) Source() string              { return BillingSourceFree }
func (f *FreeFunding) PreConsume(amount int) error { return nil }
func (f *FreeFunding) Settle(delta int) error      { return nil }
func (f *FreeFunding) Refund() error               { return nil }

// ---------------------------------------------------------------------------
// WalletFunding — 钱包资金来源实现
// ---------------------------------------------------------------------------

// ErrInsufficientWalletQuota 钱包原子预扣失败（余额不足），未发生任何扣减。
// BillingSession 据此映射为 ErrorCodeInsufficientUserQuota，
// 使 wallet_first 等计费偏好可以回退到订阅。
var ErrInsufficientWalletQuota = errors.New("wallet quota insufficient")

// WalletFunding spends a user's wallet quota, which contains both cash and
// non-withdrawable gifted credit. It tracks how much of this session's charge
// came from the gift bucket so a refund can return that amount to the gift
// bucket instead of silently converting promo credit into withdrawable cash.
type WalletFunding struct {
	userId   int
	consumed int // 实际预扣的用户额度
	// giftConsumed is the gifted portion of consumed, burnt first on every
	// charge and restored first on every refund.
	giftConsumed int
}

func (w *WalletFunding) Source() string { return BillingSourceWallet }

func (w *WalletFunding) PreConsume(amount int) error {
	if amount <= 0 {
		return nil
	}
	reserved, err := model.TryReserveUserQuota(w.userId, amount)
	if err != nil {
		return err
	}
	if !reserved {
		return ErrInsufficientWalletQuota
	}
	w.consumed = amount
	w.burnGiftQuota(amount)
	return nil
}

// ConsumeUnchecked deducts without an availability check, so the balance may go
// negative; this is the settlement path where the charge is already known.
func (w *WalletFunding) ConsumeUnchecked(amount int) error {
	if amount <= 0 {
		return nil
	}
	if err := model.DecreaseUserQuota(w.userId, amount, false); err != nil {
		return err
	}
	w.consumed += amount
	w.burnGiftQuota(amount)
	return nil
}

// RollbackConsume reverses ConsumeUnchecked, returning gifted credit to the
// gift bucket before crediting cash.
func (w *WalletFunding) RollbackConsume(amount int) error {
	if amount <= 0 {
		return nil
	}
	creditGift := w.releaseGiftQuota(amount)
	if err := model.IncreaseUserQuota(w.userId, amount-creditGift, false); err != nil {
		return err
	}
	w.consumed -= amount
	return nil
}

// burnGiftQuota moves the gifted portion of a charge out of the gift bucket.
// Burning is attribution only: User.Quota was already reduced by the caller.
// A failure here is logged, not fatal — it would leave gifted credit counted as
// cash, which the refund path clamps against the real balance.
func (w *WalletFunding) burnGiftQuota(amount int) {
	if amount <= 0 {
		return
	}
	used, err := model.TryConsumeUserGiftQuota(w.userId, amount)
	if err != nil {
		common.SysLog("failed to burn gift quota attribution: " + err.Error())
		return
	}
	w.giftConsumed += used
}

// releaseGiftQuota returns up to amount of previously burnt gifted credit to
// the gift bucket and reports how much was returned, so the caller credits only
// the remainder as cash.
func (w *WalletFunding) releaseGiftQuota(amount int) int {
	if amount <= 0 || w.giftConsumed <= 0 {
		return 0
	}
	creditGift := min(amount, w.giftConsumed)
	if err := model.RestoreUserGiftQuota(w.userId, creditGift, false); err != nil {
		common.SysLog("failed to restore gift quota attribution: " + err.Error())
		return 0
	}
	w.giftConsumed -= creditGift
	return creditGift
}

func (w *WalletFunding) Settle(delta int) error {
	if delta == 0 {
		return nil
	}
	if delta > 0 {
		return w.ConsumeUnchecked(delta)
	}
	return w.RollbackConsume(-delta)
}

func (w *WalletFunding) Refund() error {
	if w.consumed <= 0 {
		return nil
	}
	// IncreaseUserQuota 是 quota += N 的非幂等操作，不能重试，否则会多退额度。
	// 订阅的 RefundSubscriptionPreConsume 有 requestId 幂等保护所以可以重试。
	return w.RollbackConsume(w.consumed)
}

// ---------------------------------------------------------------------------
// SubscriptionFunding — 订阅资金来源实现
// ---------------------------------------------------------------------------

type SubscriptionFunding struct {
	requestId      string
	userId         int
	modelName      string
	amount         int64 // 预扣的订阅额度（subConsume）
	subscriptionId int
	preConsumed    int64
	// 以下字段在 PreConsume 成功后填充，供 RelayInfo 同步使用
	AmountTotal     int64
	AmountUsedAfter int64
	PlanId          int
	PlanTitle       string
}

func (s *SubscriptionFunding) Source() string { return BillingSourceSubscription }

func (s *SubscriptionFunding) PreConsume(_ int) error {
	// amount 参数被忽略，使用内部 s.amount（已在构造时根据 preConsumedQuota 计算）
	res, err := model.PreConsumeUserSubscription(s.requestId, s.userId, s.modelName, 0, s.amount)
	if err != nil {
		return err
	}
	s.subscriptionId = res.UserSubscriptionId
	s.preConsumed = res.PreConsumed
	s.AmountTotal = res.AmountTotal
	s.AmountUsedAfter = res.AmountUsedAfter
	// 获取订阅计划信息
	if planInfo, err := model.GetSubscriptionPlanInfoByUserSubscriptionId(res.UserSubscriptionId); err == nil && planInfo != nil {
		s.PlanId = planInfo.PlanId
		s.PlanTitle = planInfo.PlanTitle
	}
	return nil
}

func (s *SubscriptionFunding) Settle(delta int) error {
	if delta == 0 {
		return nil
	}
	return model.PostConsumeUserSubscriptionDelta(s.subscriptionId, int64(delta))
}

func (s *SubscriptionFunding) Refund() error {
	if s.preConsumed <= 0 {
		return nil
	}
	return refundWithRetry(func() error {
		return model.RefundSubscriptionPreConsume(s.requestId)
	})
}

// refundWithRetry 尝试多次执行退款操作以提高成功率，只能用于基于事务的退款函数！！！！！！
// try to refund with retries, only for refund functions based on transactions!!!
func refundWithRetry(fn func() error) error {
	if fn == nil {
		return nil
	}
	const maxAttempts = 3
	var lastErr error
	for i := range maxAttempts {
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if i < maxAttempts-1 {
			time.Sleep(time.Duration(200*(i+1)) * time.Millisecond)
		}
	}
	return lastErr
}
