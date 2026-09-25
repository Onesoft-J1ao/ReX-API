package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// Refund kinds.
const (
	// RefundKindPlan converts a subscription's remaining allowance back into
	// wallet balance at principal proportion. No money leaves the platform.
	RefundKindPlan = "plan"
	// RefundKindCash pays a share of the user's remaining unused cash balance
	// out of the platform, so it keeps a handling fee.
	RefundKindCash = "cash"
)

// Refund request lifecycle. A request is created by an administrator (the
// general refund is administrator-only) and then advances through the workflow
// the user can watch on the wallet page.
const (
	RefundStatusPending  = "pending"  // 待处理
	RefundStatusApproved = "approved" // 已处理
	RefundStatusCredited = "credited" // 已到账
	RefundStatusRejected = "rejected" // 已驳回
)

var (
	ErrRefundRequestNotFound       = errors.New("refund request not found")
	ErrRefundRequestStatus         = errors.New("refund request status invalid")
	ErrRefundNotAllowed            = errors.New("refund not allowed")
	ErrNothingToRefund             = errors.New("nothing to refund")
	ErrRefundDisabled              = errors.New("refund disabled")
	ErrRefundRequestTerminal       = errors.New("refund request already settled")
	ErrRefundRequestNotOwned       = errors.New("refund request does not belong to this user")
	ErrRefundSubscriptionNotActive = errors.New("subscription is no longer active")
)

// RefundRequest records one refund and its progress.
//
// Money fields are 元 and quota fields are quota units. PrincipalQuota is the
// balance the refund was computed from, Quota is what was actually refunded
// after the applicable ratio, and the difference is the retained fee.
type RefundRequest struct {
	Id     int    `json:"id"`
	UserId int    `json:"user_id" gorm:"index"`
	Kind   string `json:"kind" gorm:"type:varchar(16);index"`
	Status string `json:"status" gorm:"type:varchar(16);index"`

	// Plan refunds reference the subscription being converted.
	UserSubscriptionId int `json:"user_subscription_id" gorm:"index;default:0"`

	// PrincipalQuota is the remaining balance the refund was computed from and
	// DeductedQuota is the balance removed from the user. They differ only when
	// a fee applies.
	PrincipalQuota int `json:"principal_quota" gorm:"type:int;not null;default:0"`
	DeductedQuota  int `json:"deducted_quota" gorm:"type:int;not null;default:0"`
	// Quota is the amount refunded, in quota units, and Amount is its 元 value.
	Quota  int     `json:"quota" gorm:"type:int;not null;default:0"`
	Amount float64 `json:"amount" gorm:"type:decimal(10,6);not null;default:0"`

	Remark  string `json:"remark" gorm:"type:varchar(255);default:''"`
	AdminId int    `json:"admin_id" gorm:"default:0"`

	CreatedAt   int64 `json:"created_at" gorm:"bigint;index"`
	UpdatedAt   int64 `json:"updated_at" gorm:"bigint"`
	ProcessedAt int64 `json:"processed_at" gorm:"bigint;default:0"`
	CreditedAt  int64 `json:"credited_at" gorm:"bigint;default:0"`
}

func (r *RefundRequest) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	r.CreatedAt = now
	r.UpdatedAt = now
	return nil
}

func (r *RefundRequest) BeforeUpdate(tx *gorm.DB) error {
	r.UpdatedAt = common.GetTimestamp()
	return nil
}

// quotaAsMoney converts a quota amount to its 元 value.
func quotaAsMoney(quota int) (float64, error) {
	if quota <= 0 {
		return 0, nil
	}
	if common.QuotaPerUnit <= 0 {
		return 0, errors.New("额度单位配置错误")
	}
	return decimal.NewFromInt(int64(quota)).
		Div(decimal.NewFromFloat(common.QuotaPerUnit)).
		InexactFloat64(), nil
}

// PlanRefundQuota converts a subscription's remaining allowance into the wallet
// balance a refund returns.
//
// The owner's rule is "remaining x converts back at x/60*40" — that is, the
// refund returns the same *proportion of principal* that is left unused, with no
// discount. In general terms, with total allowance T and plan price P:
//
//	refundQuota = x / T * P * QuotaPerUnit
//
// which for the Go Plan (T = 60, P = 40) is exactly x/60*40 元.
func PlanRefundQuota(remaining int64, total int64, priceAmount float64) (int, error) {
	if remaining <= 0 || total <= 0 || priceAmount <= 0 {
		return 0, ErrNothingToRefund
	}
	if remaining > total {
		remaining = total
	}
	if common.QuotaPerUnit <= 0 {
		return 0, errors.New("额度单位配置错误")
	}
	quota := decimal.NewFromInt(remaining).
		Div(decimal.NewFromInt(total)).
		Mul(decimal.NewFromFloat(priceAmount)).
		Mul(decimal.NewFromFloat(common.QuotaPerUnit))
	return common.WalletQuotaFromDecimalStrict(quota)
}

// CashRefundQuota applies the configured refund ratio to the withdrawable part
// of a wallet balance. The result is the quota value of the payout; the caller
// keeps the retained remainder as a handling fee.
func CashRefundQuota(cashQuota int, ratio float64) (int, error) {
	if cashQuota <= 0 {
		return 0, ErrNothingToRefund
	}
	if ratio <= 0 || ratio > 1 {
		return 0, ErrRefundNotAllowed
	}
	quota := decimal.NewFromInt(int64(cashQuota)).
		Mul(decimal.NewFromFloat(ratio))
	return common.WalletQuotaFromDecimalStrict(quota)
}

func GetRefundRequestById(id int) (*RefundRequest, error) {
	if id <= 0 {
		return nil, ErrRefundRequestNotFound
	}
	var req RefundRequest
	if err := DB.Where("id = ?", id).First(&req).Error; err != nil {
		return nil, err
	}
	return &req, nil
}

// ListUserRefundRequests returns a user's refund requests, newest first, so the
// user can watch their progress.
func ListUserRefundRequests(userId int, pageInfo *common.PageInfo) ([]*RefundRequest, int64, error) {
	if userId <= 0 {
		return nil, 0, errors.New("invalid userId")
	}
	var total int64
	query := DB.Model(&RefundRequest{}).Where("user_id = ?", userId)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var requests []*RefundRequest
	if err := query.Order("id desc").
		Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).
		Find(&requests).Error; err != nil {
		return nil, 0, err
	}
	return requests, total, nil
}

// ListRefundRequests returns every refund request for the admin view.
func ListRefundRequests(status string, pageInfo *common.PageInfo) ([]*RefundRequest, int64, error) {
	query := DB.Model(&RefundRequest{})
	if status = strings.TrimSpace(status); status != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var requests []*RefundRequest
	if err := query.Order("id desc").
		Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).
		Find(&requests).Error; err != nil {
		return nil, 0, err
	}
	return requests, total, nil
}

// CreatePlanRefundRequest records a pending Go Plan refund. The plan is settled
// only when the request is approved, so a pending request does not strand the
// user's remaining allowance. Returns the request and the resulting group
// downgrade target, if any.
func CreatePlanRefundRequest(userId int, userSubscriptionId int, remark string, adminId int) (*RefundRequest, string, error) {
	if userId <= 0 || userSubscriptionId <= 0 {
		return nil, "", ErrRefundNotAllowed
	}
	var created *RefundRequest
	downgradeGroup := ""
	err := DB.Transaction(func(tx *gorm.DB) error {
		var sub UserSubscription
		if err := lockForUpdate(tx).Where("id = ?", userSubscriptionId).First(&sub).Error; err != nil {
			return ErrRefundSubscriptionNotActive
		}
		if sub.UserId != userId {
			return ErrRefundRequestNotOwned
		}
		if sub.Status != "active" {
			return ErrRefundSubscriptionNotActive
		}
		plan, err := getSubscriptionPlanByIdTx(tx, sub.PlanId)
		if err != nil {
			return err
		}
		remaining := sub.AmountTotal - sub.AmountUsed
		quota, err := PlanRefundQuota(remaining, sub.AmountTotal, plan.PriceAmount)
		if err != nil {
			return err
		}
		amount, err := quotaAsMoney(quota)
		if err != nil {
			return err
		}
		req := &RefundRequest{
			UserId:             userId,
			Kind:               RefundKindPlan,
			Status:             RefundStatusPending,
			UserSubscriptionId: sub.Id,
			PrincipalQuota:     int(remaining),
			DeductedQuota:      0,
			Quota:              quota,
			Amount:             amount,
			Remark:             strings.TrimSpace(remark),
			AdminId:            adminId,
		}
		if err := tx.Create(req).Error; err != nil {
			return err
		}
		created = req
		downgradeGroup = strings.TrimSpace(sub.DowngradeGroup)
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	return created, downgradeGroup, nil
}

// CreateCashRefundRequest records a pending general refund and immediately
// removes the balance it covers, so the user cannot spend it while the payout
// is being processed. The retained fee is not returned, which is what makes the
// configured ratio meaningful. Rejecting the request restores the balance.
func CreateCashRefundRequest(userId int, ratio float64, remark string, adminId int) (*RefundRequest, error) {
	if userId <= 0 {
		return nil, ErrRefundNotAllowed
	}
	if ratio <= 0 || ratio > 1 {
		return nil, ErrRefundNotAllowed
	}
	var created *RefundRequest
	err := DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).Where("id = ?", userId).First(&user).Error; err != nil {
			return err
		}
		cashQuota := RefundableCashQuota(&user)
		if cashQuota <= 0 {
			return ErrNothingToRefund
		}
		quota, err := CashRefundQuota(cashQuota, ratio)
		if err != nil {
			return err
		}
		if quota <= 0 {
			return ErrNothingToRefund
		}
		amount, err := quotaAsMoney(quota)
		if err != nil {
			return err
		}
		if err := tx.Model(&User{}).Where("id = ?", userId).
			Update("quota", gorm.Expr("quota - ?", cashQuota)).Error; err != nil {
			return err
		}
		// The gifted share of the balance stays attributed to the user's gift
		// bucket, clamped to whatever balance is left.
		if err := restoreGiftQuotaAfterCashDebitTx(tx, userId); err != nil {
			return err
		}
		req := &RefundRequest{
			UserId:         userId,
			Kind:           RefundKindCash,
			Status:         RefundStatusPending,
			PrincipalQuota: cashQuota,
			DeductedQuota:  cashQuota,
			Quota:          quota,
			Amount:         amount,
			Remark:         strings.TrimSpace(remark),
			AdminId:        adminId,
		}
		if err := tx.Create(req).Error; err != nil {
			return err
		}
		created = req
		return nil
	})
	if err != nil {
		return nil, err
	}
	// The debit already committed; mirror it into the cache so the frozen
	// balance is visible immediately instead of after the cache expires.
	cacheDecrUserQuota(userId, int64(created.DeductedQuota))
	return created, nil
}

// restoreGiftQuotaAfterCashDebitTx keeps the gift attribution consistent after a
// cash refund debits the wallet: the gift bucket can never exceed the balance.
func restoreGiftQuotaAfterCashDebitTx(tx *gorm.DB, userId int) error {
	return tx.Model(&User{}).Where("id = ? AND gift_quota > quota", userId).
		Update("gift_quota", gorm.Expr("quota")).Error
}

// AdvanceRefundRequest moves a request through the refund workflow.
//
//	pending  -> approved : settle a plan refund (credit the wallet, end the plan)
//	approved -> credited : confirm the payout reached the user
//	pending|approved -> rejected : restore a debited balance
func AdvanceRefundRequest(id int, targetStatus string, adminId int) (*RefundRequest, string, error) {
	if id <= 0 {
		return nil, "", ErrRefundRequestNotFound
	}
	targetStatus = strings.TrimSpace(targetStatus)
	var result *RefundRequest
	downgradeGroup := ""
	userId := 0
	cacheCredit := 0
	err := DB.Transaction(func(tx *gorm.DB) error {
		var req RefundRequest
		if err := lockForUpdate(tx).Where("id = ?", id).First(&req).Error; err != nil {
			return ErrRefundRequestNotFound
		}
		userId = req.UserId
		now := common.GetTimestamp()
		switch targetStatus {
		case RefundStatusApproved:
			if req.Status != RefundStatusPending {
				return ErrRefundRequestStatus
			}
			if req.Kind == RefundKindPlan {
				target, err := settlePlanRefundTx(tx, &req, now)
				if err != nil {
					return err
				}
				downgradeGroup = target
				cacheCredit += req.Quota
			}
			req.Status = RefundStatusApproved
			req.ProcessedAt = now
		case RefundStatusCredited:
			if req.Status != RefundStatusApproved {
				return ErrRefundRequestStatus
			}
			req.Status = RefundStatusCredited
			req.CreditedAt = now
		case RefundStatusRejected:
			if req.Status == RefundStatusCredited || req.Status == RefundStatusRejected {
				return ErrRefundRequestTerminal
			}
			if req.Kind == RefundKindCash && req.DeductedQuota > 0 {
				if err := tx.Model(&User{}).Where("id = ?", req.UserId).
					Update("quota", gorm.Expr("quota + ?", req.DeductedQuota)).Error; err != nil {
					return err
				}
				cacheCredit += req.DeductedQuota
			}
			req.Status = RefundStatusRejected
			req.ProcessedAt = now
		default:
			return ErrRefundRequestStatus
		}
		if adminId > 0 {
			req.AdminId = adminId
		}
		if err := tx.Save(&req).Error; err != nil {
			return err
		}
		result = &req
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	if cacheCredit > 0 {
		syncCreditUserQuotaCache(userId, cacheCredit, "refund advance")
	}
	if downgradeGroup != "" {
		refreshSubscriptionUserGroupCache(userId, "plan refund")
	}
	return result, downgradeGroup, nil
}

// settlePlanRefundTx ends the subscription and credits the refunded principal to
// the wallet. The credited amount comes from the request snapshot, so approving
// a request can never refund more than what was computed when it was created.
func settlePlanRefundTx(tx *gorm.DB, req *RefundRequest, now int64) (string, error) {
	if req.Quota <= 0 {
		return "", ErrNothingToRefund
	}
	var sub UserSubscription
	if err := lockForUpdate(tx).Where("id = ?", req.UserSubscriptionId).First(&sub).Error; err != nil {
		return "", ErrRefundSubscriptionNotActive
	}
	if sub.UserId != req.UserId {
		return "", ErrRefundRequestNotOwned
	}
	if sub.Status != "active" {
		return "", ErrRefundSubscriptionNotActive
	}
	downgradeGroup := ""
	// Zero the remaining allowance before ending the plan so the refunded
	// portion cannot also be spent.
	if err := tx.Model(&sub).Updates(map[string]any{
		"status":      "cancelled",
		"end_time":    now,
		"amount_used": sub.AmountTotal,
		"updated_at":  now,
	}).Error; err != nil {
		return "", err
	}
	target, err := downgradeUserGroupForSubscriptionTx(tx, &sub, now)
	if err != nil {
		return "", err
	}
	downgradeGroup = target
	if err := creditRefundQuotaTx(tx, req.UserId, req.Quota); err != nil {
		return "", err
	}
	return downgradeGroup, nil
}

// creditRefundQuotaTx credits a plan refund as non-withdrawable wallet balance,
// unless the operator configured the refund to restore withdrawable cash.
func creditRefundQuotaTx(tx *gorm.DB, userId int, quota int) error {
	if quota <= 0 {
		return nil
	}
	if err := common.ValidateWalletQuota(quota); err != nil {
		return err
	}
	if !operation_setting.GetRefundSetting().PlanRefundToGiftQuota {
		result := tx.Model(&User{}).
			Where("id = ? AND quota <= ?", userId, common.MaxWalletQuota-quota).
			Update("quota", gorm.Expr("quota + ?", quota))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrWalletQuotaLimitExceeded
		}
		return nil
	}
	result := tx.Model(&User{}).
		Where("id = ? AND quota <= ?", userId, common.MaxWalletQuota-quota).
		Updates(map[string]any{
			"quota":      gorm.Expr("quota + ?", quota),
			"gift_quota": gorm.Expr("gift_quota + ?", quota),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrWalletQuotaLimitExceeded
	}
	return nil
}

// RecordRefundRequestLog writes the user-visible refund audit entry.
func RecordRefundRequestLog(req *RefundRequest, action string) {
	if req == nil {
		return
	}
	RecordLog(req.UserId, LogTypeRefund, fmt.Sprintf(
		"退款%s：类型 %s，金额 %.4f 元（额度 %d），状态 %s",
		action, req.Kind, req.Amount, req.Quota, req.Status,
	))
}
