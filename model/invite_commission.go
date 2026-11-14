package model

import (
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"gorm.io/gorm"
)

// 邀请返利规则（2026-11 起）：
//
//	被邀请人前 10 笔「实付 > 15 元」的法币充值，邀请人拿 7.5%；
//	返利在下单 24 小时后自动转入邀请人余额，只能用于平台消费。
//	赠送额度、兑换码、订阅、加密货币支付、人工充值都不参与。
const (
	InviteCommissionRate        = 0.075
	InviteCommissionMinMoney    = 15.0
	InviteCommissionMaxTopUps   = 10
	InviteCommissionDelaySecond = 24 * 60 * 60

	InviteCommissionStatusPending  = "pending"
	InviteCommissionStatusCredited = "credited"
)

// InviteCommission 是一笔待结算的邀请返利。TopUpId 唯一，保证同一个订单
// 只产生一次返利（回调重放、重复结算都不会多给）。
type InviteCommission struct {
	Id          int     `json:"id" gorm:"primaryKey"`
	InviterId   int     `json:"inviter_id" gorm:"index"`
	InviteeId   int     `json:"invitee_id" gorm:"index"`
	TopUpId     int     `json:"topup_id" gorm:"uniqueIndex"`
	TradeNo     string  `json:"trade_no" gorm:"type:varchar(255);index"`
	Money       float64 `json:"money"`
	Quota       int     `json:"quota"`
	Status      string  `json:"status" gorm:"type:varchar(16);index;default:'pending'"`
	CreatedAt   int64   `json:"created_at" gorm:"index"`
	AvailableAt int64   `json:"available_at" gorm:"index"`
	CreditedAt  int64   `json:"credited_at"`
}

func (InviteCommission) TableName() string { return "invite_commissions" }

// fiatPaymentProvider 判断这笔充值是不是「法币」。加密货币和余额抵扣不参与返利。
func fiatPaymentProvider(provider string) bool {
	switch provider {
	case PaymentProviderEpay, PaymentProviderStripe, PaymentProviderCreem,
		PaymentProviderWaffoPancake:
		return true
	default:
		return false
	}
}

// maybeCreateInviteCommission 在充值结算时登记一笔返利。
//
// 必须放在 settleTopUp 的同一事务里：订单成功与返利登记要么一起成立，
// 要么一起回滚，避免出现「钱到账了但返利丢了」或反过来的情况。
func maybeCreateInviteCommission(tx *gorm.DB, topUp *TopUp, creditedQuota int) error {
	if topUp == nil || topUp.UserId <= 0 || topUp.IsRedemptionPurchase() {
		return nil
	}
	if topUp.Money <= InviteCommissionMinMoney {
		return nil
	}
	if !fiatPaymentProvider(topUp.PaymentProvider) {
		return nil
	}

	var invitee User
	if err := tx.Select("id", "inviter_id").Where("id = ?", topUp.UserId).First(&invitee).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if invitee.InviterId <= 0 || invitee.InviterId == invitee.Id {
		return nil
	}

	var count int64
	if err := tx.Model(&InviteCommission{}).Where("invitee_id = ?", invitee.Id).Count(&count).Error; err != nil {
		return err
	}
	if count >= InviteCommissionMaxTopUps {
		return nil
	}

	base := creditedQuota
	if base <= 0 {
		base = int(topUp.Money * common.QuotaPerUnit)
	}
	reward := int(float64(base) * InviteCommissionRate)
	if reward <= 0 {
		return nil
	}

	now := common.GetTimestamp()
	row := &InviteCommission{
		InviterId:   invitee.InviterId,
		InviteeId:   invitee.Id,
		TopUpId:     topUp.Id,
		TradeNo:     topUp.TradeNo,
		Money:       topUp.Money,
		Quota:       reward,
		Status:      InviteCommissionStatusPending,
		CreatedAt:   now,
		AvailableAt: now + InviteCommissionDelaySecond,
	}
	return tx.Create(row).Error
}

// CreditPendingInviteCommissions 把到期的返利转入邀请人余额。
// 返回本轮到账的笔数。
func CreditPendingInviteCommissions() (int, error) {
	now := common.GetTimestamp()
	var rows []InviteCommission
	if err := DB.Where("status = ? AND available_at <= ?", InviteCommissionStatusPending, now).
		Order("id ASC").Limit(200).Find(&rows).Error; err != nil {
		return 0, err
	}
	credited := 0
	for _, row := range rows {
		var applied bool
		err := DB.Transaction(func(tx *gorm.DB) error {
			res := tx.Model(&InviteCommission{}).
				Where("id = ? AND status = ?", row.Id, InviteCommissionStatusPending).
				Updates(map[string]any{
					"status":      InviteCommissionStatusCredited,
					"credited_at": now,
				})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return nil // 别的实例已经处理过
			}
			if err := creditTopUpQuota(tx, row.InviterId, row.Quota, nil); err != nil {
				return err
			}
			applied = true
			return nil
		})
		if err != nil {
			common.SysError(fmt.Sprintf("邀请返利入账失败 id=%d: %s", row.Id, err.Error()))
			continue
		}
		if applied {
			credited++
			RecordLog(row.InviterId, LogTypeSystem,
				fmt.Sprintf("邀请返利到账 %s（被邀请人实付 %.2f）", logger.LogQuota(row.Quota), row.Money))
		}
	}
	return credited, nil
}

// InviteCommissionLoop 每 5 分钟结算一次到期的返利。
func InviteCommissionLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		if n, err := CreditPendingInviteCommissions(); err != nil {
			common.SysError("邀请返利结算失败: " + err.Error())
		} else if n > 0 {
			common.SysLog(fmt.Sprintf("邀请返利已入账 %d 笔", n))
		}
		<-ticker.C
	}
}
