package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// useRefundQuotaUnit pins the quota unit so the refund arithmetic has exact
// expected values instead of depending on deployment configuration.
func useRefundQuotaUnit(t *testing.T) {
	t.Helper()
	previous := common.QuotaPerUnit
	common.QuotaPerUnit = 500 * 1000.0
	t.Cleanup(func() { common.QuotaPerUnit = previous })
}

func TestPlanRefundQuotaReturnsPrincipalProportion(t *testing.T) {
	useRefundQuotaUnit(t)

	tests := []struct {
		name      string
		remaining int64
		total     int64
		price     float64
		want      int
		wantErr   error
	}{
		{
			name: "the Go Plan untouched returns the full principal",
			// The owner's rule: remaining x converts back at x/60*40.
			remaining: 60, total: 60, price: 40,
			want: 40 * 500000,
		},
		{
			name:      "half used returns half the principal",
			remaining: 30, total: 60, price: 40,
			want: 20 * 500000,
		},
		{
			name:      "a non-round remainder rounds to the nearest quota",
			remaining: 7, total: 60, price: 40,
			want: 2333333,
		},
		{
			name:      "claiming more remaining than the plan holds is capped at the principal",
			remaining: 120, total: 60, price: 40,
			want: 40 * 500000,
		},
		{
			name:      "nothing left is not refundable",
			remaining: 0, total: 60, price: 40,
			wantErr: ErrNothingToRefund,
		},
		{
			name:      "a negative remainder is not refundable",
			remaining: -5, total: 60, price: 40,
			wantErr: ErrNothingToRefund,
		},
		{
			name:      "a plan with no total allowance is not refundable",
			remaining: 10, total: 0, price: 40,
			wantErr: ErrNothingToRefund,
		},
		{
			name:      "a free plan has no principal to return",
			remaining: 10, total: 60, price: 0,
			wantErr: ErrNothingToRefund,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := PlanRefundQuota(tc.remaining, tc.total, tc.price)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				assert.Zero(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestCashRefundQuotaAppliesTheConfiguredRatio(t *testing.T) {
	tests := []struct {
		name    string
		cash    int
		ratio   float64
		want    int
		wantErr error
	}{
		{name: "the general refund keeps a quarter", cash: 1000, ratio: 0.75, want: 750},
		{name: "a ratio of one returns everything", cash: 1000, ratio: 1, want: 1000},
		{name: "an empty balance is not refundable", cash: 0, ratio: 0.75, wantErr: ErrNothingToRefund},
		{name: "a zero ratio is rejected rather than paying nothing", cash: 1000, ratio: 0, wantErr: ErrRefundNotAllowed},
		{name: "a ratio above one cannot pay out more than the balance", cash: 1000, ratio: 1.5, wantErr: ErrRefundNotAllowed},
		{name: "a negative ratio is rejected", cash: 1000, ratio: -0.5, wantErr: ErrRefundNotAllowed},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CashRefundQuota(tc.cash, tc.ratio)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				assert.Zero(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestRefundableCashQuotaExcludesGiftedQuota(t *testing.T) {
	tests := []struct {
		name string
		user *User
		want int
	}{
		{name: "no user has nothing to refund", user: nil, want: 0},
		{name: "a purely gifted balance is not refundable", user: &User{Quota: 1000, GiftQuota: 1000}, want: 0},
		{name: "gifted quota above the balance is not refundable", user: &User{Quota: 1000, GiftQuota: 1500}, want: 0},
		{name: "only the paid part is refundable", user: &User{Quota: 1000, GiftQuota: 400}, want: 600},
		{name: "a balance with no gifted part is fully refundable", user: &User{Quota: 1000}, want: 1000},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, RefundableCashQuota(tc.user))
		})
	}
}
