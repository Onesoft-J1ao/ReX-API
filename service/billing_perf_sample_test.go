package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// The model square reads its health badges from the perf-metrics summary, which
// is fed exclusively by RecordRelaySample. Settlement through BillingSession
// bypasses the legacy PostConsumeQuota path that used to be the only recording
// call site, so a request that settles through a session must still be sampled.
func TestSettleBillingRecordsPerfSampleForSessionSettlement(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &model.User{
		Username: "perf-sample-user",
		Password: "hashed",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
		Quota:    10_000_000,
	}
	require.NoError(t, model.DB.Create(user).Error)

	const modelName = "perf-sample-model"
	relayInfo := &relaycommon.RelayInfo{
		UserId:          user.Id,
		UsingGroup:      "default",
		OriginModelName: modelName,
		StartTime:       time.Now().Add(-1500 * time.Millisecond),
	}
	relayInfo.UserSetting.BillingPreference = "wallet_only"

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	session, apiErr := NewBillingSession(ctx, relayInfo, 0)
	require.Nil(t, apiErr)
	require.NotNil(t, session)
	relayInfo.Billing = session

	require.NoError(t, SettleBilling(ctx, relayInfo, 0, 42))

	// RecordRelaySample runs inside gopool, so wait for the sample to land
	// instead of assuming it is visible immediately.
	require.Eventually(t, func() bool {
		return perfmetrics.RecordedSampleCount(modelName, "default") == 1
	}, 2*time.Second, 10*time.Millisecond, "settling through a BillingSession must record one perf sample")
}
