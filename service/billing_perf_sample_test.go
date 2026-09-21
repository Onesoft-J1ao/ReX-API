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
// is fed exclusively by the perf-metrics sampler. A request that settles through
// a BillingSession must still be sampled: that path bypasses the legacy
// PostConsumeQuota call site, so the sample is taken at the request boundary
// (controller/relay.go calls RecordRelayResult from a deferred hook) rather than
// during settlement.
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

	// Settlement alone must not sample: the boundary owns that, and sampling
	// here as well would double-count every request.
	require.Zero(t, perfmetrics.RecordedSampleCount(modelName, "default"),
		"settlement must not record a perf sample; the request boundary does")

	// The boundary hook is what records it.
	perfmetrics.RecordRelayResult(ctx, relayInfo, nil)

	require.Eventually(t, func() bool {
		return perfmetrics.RecordedSampleCount(modelName, "default") == 1
	}, 2*time.Second, 10*time.Millisecond, "the request boundary must record one perf sample")
}
