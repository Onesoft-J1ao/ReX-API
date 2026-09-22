package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type refundPageResponse struct {
	Items []model.RefundRequest `json:"items"`
	Total int                   `json:"total"`
}

// setupRefundControllerTest opens an isolated in-memory database with every table
// the refund handler chain touches, including the audit tables admin routes write.
func setupRefundControllerTest(t *testing.T) *gorm.DB {
	t.Helper()

	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	previousRedis := common.RedisEnabled
	previousMaster := common.IsMasterNode
	previousQuotaPerUnit := common.QuotaPerUnit

	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.UserSubscription{},
		&model.SubscriptionPlan{},
		&model.RefundRequest{},
		&model.Log{},
		&model.AuditLog{},
		&model.CasbinRule{},
		&model.AuthzRole{},
	))

	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.IsMasterNode = true
	// Pin the quota unit so the refund arithmetic has exact expected values.
	common.QuotaPerUnit = 500 * 1000.0
	require.NoError(t, authz.Init(db))

	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.IsMasterNode = previousMaster
		common.SetDatabaseTypes(previousMain, previousLog)
		common.RedisEnabled = previousRedis
		common.QuotaPerUnit = previousQuotaPerUnit
	})

	return db
}

// seedRefundUser creates an enabled user with a legacy access token so the
// dashboard auth middleware can resolve the request to that role.
func seedRefundUser(t *testing.T, db *gorm.DB, username string, role int, quota int, giftQuota int) (*model.User, string) {
	t.Helper()

	token := username + "-token"
	user := &model.User{
		Username:    username,
		Password:    "placeholder",
		Role:        role,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		Quota:       quota,
		GiftQuota:   giftQuota,
		AccessToken: &token,
		AuthVersion: 1,
		AffCode:     username,
	}
	require.NoError(t, db.Create(user).Error)
	return user, token
}

// refundTestRouter mirrors the middleware stack the refund routes are registered
// with in router/api-router.go.
func refundTestRouter() *gin.Engine {
	router := gin.New()
	router.Use(middleware.RequestId())
	router.GET("/api/refund/self", middleware.UserAuth(), GetUserRefundRequests)
	admin := router.Group("/api/refund/admin", middleware.AdminAuth())
	admin.GET("/", AdminListRefundRequests)
	admin.POST("/plan", AdminCreatePlanRefundRequest)
	admin.POST("/cash", AdminCreateCashRefundRequest)
	admin.POST("/:id/advance", AdminAdvanceRefundRequest)
	return router
}

func callRefundRoute(t *testing.T, router http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func mustRefundJSON(t *testing.T, value any) string {
	t.Helper()

	encoded, err := common.Marshal(value)
	require.NoError(t, err)
	return string(encoded)
}

func TestGetUserRefundRequestsReturnsOnlyTheCallersRequests(t *testing.T) {
	db := setupRefundControllerTest(t)
	owner, ownerToken := seedRefundUser(t, db, "refund-owner", common.RoleCommonUser, 1000, 0)
	other, _ := seedRefundUser(t, db, "refund-other", common.RoleCommonUser, 1000, 0)

	for _, request := range []*model.RefundRequest{
		{UserId: owner.Id, Kind: model.RefundKindPlan, Status: model.RefundStatusPending, Quota: 10},
		{UserId: owner.Id, Kind: model.RefundKindCash, Status: model.RefundStatusCredited, Quota: 20},
		{UserId: other.Id, Kind: model.RefundKindCash, Status: model.RefundStatusPending, Quota: 30},
	} {
		require.NoError(t, db.Create(request).Error)
	}

	response := callRefundRoute(t, refundTestRouter(), http.MethodGet, "/api/refund/self", ownerToken, "")
	require.Equal(t, http.StatusOK, response.Code)

	decoded := decodeAPIResponse(t, response)
	require.True(t, decoded.Success, decoded.Message)

	var page refundPageResponse
	require.NoError(t, common.Unmarshal(decoded.Data, &page))
	require.Len(t, page.Items, 2, "another user's request must not be readable through /refund/self")
	assert.Equal(t, 2, page.Total)
	for _, item := range page.Items {
		assert.Equal(t, owner.Id, item.UserId)
	}
}

func TestRefundAdminRoutesRejectNonAdminUsers(t *testing.T) {
	db := setupRefundControllerTest(t)
	regular, regularToken := seedRefundUser(t, db, "refund-regular", common.RoleCommonUser, 1000, 0)
	_, adminToken := seedRefundUser(t, db, "refund-admin", common.RoleAdminUser, 0, 0)

	router := refundTestRouter()
	userBody := mustRefundJSON(t, gin.H{"user_id": regular.Id})

	for _, route := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/refund/admin/", ""},
		{http.MethodPost, "/api/refund/admin/plan", userBody},
		{http.MethodPost, "/api/refund/admin/cash", userBody},
		{http.MethodPost, "/api/refund/admin/1/advance", mustRefundJSON(t, gin.H{"status": "approved"})},
	} {
		response := callRefundRoute(t, router, route.method, route.path, regularToken, route.body)
		assert.Equal(t, http.StatusForbidden, response.Code, "%s %s must reject a regular user", route.method, route.path)
		assert.Contains(t, response.Body.String(), "AUTH_INSUFFICIENT_PRIVILEGE")
	}

	response := callRefundRoute(t, router, http.MethodGet, "/api/refund/admin/", adminToken, "")
	assert.Equal(t, http.StatusOK, response.Code, "an administrator must reach the same route")
	assert.True(t, decodeAPIResponse(t, response).Success)
}

func TestPlanRefundRefusesIllegalTransitionsAndCreditsTheCreationSnapshot(t *testing.T) {
	db := setupRefundControllerTest(t)
	user, userToken := seedRefundUser(t, db, "refund-plan-user", common.RoleCommonUser, 0, 0)
	_, adminToken := seedRefundUser(t, db, "refund-plan-admin", common.RoleAdminUser, 0, 0)

	plan := &model.SubscriptionPlan{Title: "Go Plan", PriceAmount: 40, TotalAmount: 60, Enabled: true}
	require.NoError(t, db.Create(plan).Error)
	subscription := &model.UserSubscription{
		UserId:      user.Id,
		PlanId:      plan.Id,
		AmountTotal: 60,
		AmountUsed:  0,
		Status:      "active",
		EndTime:     common.GetTimestamp() + 3600,
	}
	require.NoError(t, db.Create(subscription).Error)

	router := refundTestRouter()
	created := callRefundRoute(t, router, http.MethodPost, "/api/refund/admin/plan", adminToken,
		mustRefundJSON(t, gin.H{"user_id": user.Id, "user_subscription_id": subscription.Id}))
	require.Equal(t, http.StatusOK, created.Code)

	createdResponse := decodeAPIResponse(t, created)
	require.True(t, createdResponse.Success, createdResponse.Message)
	var request model.RefundRequest
	require.NoError(t, common.Unmarshal(createdResponse.Data, &request))
	require.Equal(t, model.RefundStatusPending, request.Status)
	// The whole 60-unit allowance is unused, so the refund returns the full
	// principal: 60/60 * 40 元 * 500000 quota per 元.
	require.Equal(t, 40*500000, request.Quota)

	advancePath := "/api/refund/admin/" + mustRefundJSON(t, request.Id) + "/advance"

	// pending -> credited is not part of the workflow.
	illegal := callRefundRoute(t, router, http.MethodPost, advancePath, adminToken,
		mustRefundJSON(t, gin.H{"status": "credited"}))
	assert.False(t, decodeAPIResponse(t, illegal).Success, "pending -> credited must be refused")

	stored, err := model.GetRefundRequestById(request.Id)
	require.NoError(t, err)
	assert.Equal(t, model.RefundStatusPending, stored.Status, "a refused transition must not change the row")

	// An unknown target status is rejected before the model is consulted.
	invalid := callRefundRoute(t, router, http.MethodPost, advancePath, adminToken,
		mustRefundJSON(t, gin.H{"status": "refunded"}))
	assert.False(t, decodeAPIResponse(t, invalid).Success)

	// pending -> approved settles the plan and credits exactly the snapshotted quota.
	approved := callRefundRoute(t, router, http.MethodPost, advancePath, adminToken,
		mustRefundJSON(t, gin.H{"status": "approved"}))
	require.True(t, decodeAPIResponse(t, approved).Success)

	settled, err := model.GetRefundRequestById(request.Id)
	require.NoError(t, err)
	assert.Equal(t, model.RefundStatusApproved, settled.Status)

	var updatedSubscription model.UserSubscription
	require.NoError(t, db.Where("id = ?", subscription.Id).First(&updatedSubscription).Error)
	assert.Equal(t, "cancelled", updatedSubscription.Status)

	wallet, err := model.GetUserById(user.Id, true)
	require.NoError(t, err)
	assert.Equal(t, request.Quota, wallet.Quota, "the credited amount must equal the creation snapshot")
	assert.Equal(t, request.Quota, wallet.GiftQuota, "a plan refund lands as non-withdrawable balance")

	// approved -> credited, after which the request is settled and irreversible.
	credited := callRefundRoute(t, router, http.MethodPost, advancePath, adminToken,
		mustRefundJSON(t, gin.H{"status": "credited"}))
	require.True(t, decodeAPIResponse(t, credited).Success)

	rejected := callRefundRoute(t, router, http.MethodPost, advancePath, adminToken,
		mustRefundJSON(t, gin.H{"status": "rejected"}))
	assert.False(t, decodeAPIResponse(t, rejected).Success, "a settled request must not be reversible")

	// The state changes are visible in the user's own refund log.
	logs, total, err := model.GetUserLogs(user.Id, model.LogTypeRefund, 0, 0, "", "", 0, 10, "", "", "")
	require.NoError(t, err)
	assert.Equal(t, int64(3), total, "create, approve and credited are logged")
	assert.Len(t, logs, 3)

	// The owner can read their own request through the user route.
	self := decodeAPIResponse(t, callRefundRoute(t, router, http.MethodGet, "/api/refund/self", userToken, ""))
	require.True(t, self.Success)
	var page refundPageResponse
	require.NoError(t, common.Unmarshal(self.Data, &page))
	assert.Equal(t, 1, page.Total)
}

func TestCashRefundFreezesTheWithdrawableBalanceAndRejectionRestoresIt(t *testing.T) {
	db := setupRefundControllerTest(t)
	user, userToken := seedRefundUser(t, db, "refund-cash-user", common.RoleCommonUser, 1000, 400)
	_, adminToken := seedRefundUser(t, db, "refund-cash-admin", common.RoleAdminUser, 0, 0)

	router := refundTestRouter()
	created := callRefundRoute(t, router, http.MethodPost, "/api/refund/admin/cash", adminToken,
		mustRefundJSON(t, gin.H{"user_id": user.Id, "ratio": 0.5}))
	require.Equal(t, http.StatusOK, created.Code)

	createdResponse := decodeAPIResponse(t, created)
	require.True(t, createdResponse.Success, createdResponse.Message)
	var request model.RefundRequest
	require.NoError(t, common.Unmarshal(createdResponse.Data, &request))
	// Only quota-minus-gift is withdrawable: 1000 - 400 = 600, halved to 300.
	require.Equal(t, 600, request.PrincipalQuota)
	require.Equal(t, 300, request.Quota)

	frozen, err := model.GetUserById(user.Id, true)
	require.NoError(t, err)
	assert.Equal(t, 400, frozen.Quota, "the withdrawable balance is frozen while the payout is in flight")

	// Rejecting restores exactly the frozen amount.
	rejected := callRefundRoute(t, router, http.MethodPost,
		"/api/refund/admin/"+mustRefundJSON(t, request.Id)+"/advance", adminToken,
		mustRefundJSON(t, gin.H{"status": "rejected"}))
	require.True(t, decodeAPIResponse(t, rejected).Success)

	restored, err := model.GetUserById(user.Id, true)
	require.NoError(t, err)
	assert.Equal(t, 1000, restored.Quota)
	assert.Equal(t, 400, restored.GiftQuota)
	assert.LessOrEqual(t, restored.GiftQuota, restored.Quota, "gift attribution must never exceed the balance")

	// A purely gifted balance has nothing withdrawable to refund.
	gifted, _ := seedRefundUser(t, db, "refund-cash-gifted", common.RoleCommonUser, 500, 500)
	nothing := callRefundRoute(t, router, http.MethodPost, "/api/refund/admin/cash", adminToken,
		mustRefundJSON(t, gin.H{"user_id": gifted.Id}))
	assert.False(t, decodeAPIResponse(t, nothing).Success)

	self := callRefundRoute(t, router, http.MethodGet, "/api/refund/self", userToken, "")
	assert.True(t, decodeAPIResponse(t, self).Success)
}
