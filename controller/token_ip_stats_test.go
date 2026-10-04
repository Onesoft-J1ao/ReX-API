package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// newTokenIpStatsTestDB wires an in-memory database as both the main and the log
// database so the aggregation can read tokens and logs from one handle.
func newTokenIpStatsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Token{}, &model.Log{}))
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
	})
	return db
}

func seedTokenIpStatsLog(t *testing.T, db *gorm.DB, tokenId int, ip string, promptTokens, completionTokens, quota int, createdAt int64) {
	t.Helper()
	require.NoError(t, db.Create(&model.Log{
		UserId:           1,
		CreatedAt:        createdAt,
		Type:             model.LogTypeConsume,
		TokenId:          tokenId,
		TokenName:        "token",
		ModelName:        "test-model",
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		Quota:            quota,
		Ip:               ip,
	}).Error)
}

// A caller must only ever receive the IP breakdown of tokens they own, even
// when they ask for someone else's tokens by id.
func TestGetTokenIpStatsRejectsForeignTokensForRegularUser(t *testing.T) {
	db := newTokenIpStatsTestDB(t)

	require.NoError(t, db.Create(&model.Token{UserId: 1, Key: "owner-key", Name: "owner"}).Error)
	foreign := model.Token{UserId: 2, Key: "foreign-key", Name: "foreign"}
	require.NoError(t, db.Create(&foreign).Error)
	seedTokenIpStatsLog(t, db, foreign.Id, "203.0.113.7", 100, 50, 10, 1700000000)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/token/ip-stats", nil)
	c.Set("id", 1)
	c.Set("role", common.RoleCommonUser)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/token/ip-stats?ids="+itoa(foreign.Id), nil)

	GetTokenIpStats(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Stats []*model.TokenIpStat `json:"stats"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Empty(t, response.Data.Stats, "a foreign token id must not return another account's IP usage")
}

// A non-admin asking for another account via user_id is refused outright,
// rather than silently narrowed to their own scope.
func TestGetTokenIpStatsRejectsForeignUserScopeForRegularUser(t *testing.T) {
	db := newTokenIpStatsTestDB(t)

	token := model.Token{UserId: 1, Key: "owner-key", Name: "owner"}
	require.NoError(t, db.Create(&token).Error)
	seedTokenIpStatsLog(t, db, token.Id, "203.0.113.9", 10, 5, 1, 1700000000)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("id", 1)
	c.Set("role", common.RoleCommonUser)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/token/ip-stats?ids="+itoa(token.Id)+"&user_id=2", nil)

	GetTokenIpStats(c)

	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success, "widening the user scope must fail for a regular user")
}

// An admin inspecting another account's key does get the breakdown; this is the
// path the abuse investigation depends on.
func TestGetTokenIpStatsAllowsAdminToInspectAnotherUser(t *testing.T) {
	db := newTokenIpStatsTestDB(t)

	token := model.Token{UserId: 2, Key: "victim-key", Name: "victim"}
	require.NoError(t, db.Create(&token).Error)
	seedTokenIpStatsLog(t, db, token.Id, "198.51.100.4", 200, 100, 25, 1700000000)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("id", 99)
	c.Set("role", common.RoleAdminUser)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/token/ip-stats?ids="+itoa(token.Id)+"&user_id=2", nil)

	GetTokenIpStats(c)

	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Stats []*model.TokenIpStat `json:"stats"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Len(t, response.Data.Stats, 1)
	assert.Equal(t, 300, response.Data.Stats[0].TotalTokens)
	require.Len(t, response.Data.Stats[0].Ips, 1)
	assert.Equal(t, "198.51.100.4", response.Data.Stats[0].Ips[0].Ip)
	assert.Equal(t, 300, response.Data.Stats[0].Ips[0].PromptTokens+response.Data.Stats[0].Ips[0].CompletionTokens)
}

// The per-token list is capped by tokens consumed, and the distinct-IP count
// still reports every address so the UI can say "top 20 of 340".
func TestGetTokenIpStatsCapsIpsButReportsFullDistinctCount(t *testing.T) {
	db := newTokenIpStatsTestDB(t)

	token := model.Token{UserId: 1, Key: "capped-key", Name: "capped"}
	require.NoError(t, db.Create(&token).Error)
	// 25 distinct addresses; the busiest one is created last so ranking must
	// come from the aggregate, not insertion order.
	for i := range 25 {
		seedTokenIpStatsLog(t, db, token.Id, ipForIndex(i), 10, 0, 1, int64(1700000000+i))
	}
	seedTokenIpStatsLog(t, db, token.Id, "203.0.113.250", 100000, 0, 999, 1700009999)

	stats, err := model.GetUserTokenIpStats(1, []int{token.Id}, 3)
	require.NoError(t, err)
	stat := stats[token.Id]
	require.NotNil(t, stat)

	assert.Equal(t, 26, stat.TotalIps, "distinct count must include addresses beyond the cap")
	assert.Len(t, stat.Ips, 3, "only the cap is returned")
	assert.Equal(t, "203.0.113.250", stat.Ips[0].Ip, "busiest address ranks first")
	assert.Equal(t, 100000, stat.Ips[0].PromptTokens)
}

// Rows written before IP recording was enabled carry an empty address. They
// must land in one labelled bucket and still count toward the token totals,
// never disappear and make the numbers look clean.
func TestGetTokenIpStatsBucketsUnrecordedIpsAsUnknown(t *testing.T) {
	db := newTokenIpStatsTestDB(t)

	token := model.Token{UserId: 1, Key: "legacy-key", Name: "legacy"}
	require.NoError(t, db.Create(&token).Error)
	seedTokenIpStatsLog(t, db, token.Id, "", 500, 250, 30, 1700000000)
	seedTokenIpStatsLog(t, db, token.Id, "", 100, 50, 5, 1700000100)
	seedTokenIpStatsLog(t, db, token.Id, "203.0.113.5", 10, 5, 1, 1700000200)

	stats, err := model.GetUserTokenIpStats(1, []int{token.Id}, 20)
	require.NoError(t, err)
	stat := stats[token.Id]
	require.NotNil(t, stat)

	assert.Equal(t, 1, stat.TotalIps, "an empty address is not a distinct IP")
	assert.Equal(t, 915, stat.TotalTokens, "unknown rows still count toward the total")
	assert.Equal(t, int64(2), stat.UnknownRequest)
	assert.Equal(t, 900, stat.UnknownTokens)
	require.Len(t, stat.Ips, 2, "one known address plus the unknown bucket")
	assert.Equal(t, "203.0.113.5", stat.Ips[0].Ip)
	assert.Equal(t, "", stat.Ips[1].Ip, "the unknown bucket is labelled by an empty address")
	assert.Equal(t, int64(2), stat.Ips[1].Requests, "unknown rows are aggregated into one bucket")
}

// First and last seen come from the log timestamps, so an operator can tell a
// one-off probe from an address that farmed the key over weeks.
func TestGetTokenIpStatsReportsFirstAndLastSeen(t *testing.T) {
	db := newTokenIpStatsTestDB(t)

	token := model.Token{UserId: 1, Key: "window-key", Name: "window"}
	require.NoError(t, db.Create(&token).Error)
	seedTokenIpStatsLog(t, db, token.Id, "203.0.113.8", 10, 0, 1, 1700000500)
	seedTokenIpStatsLog(t, db, token.Id, "203.0.113.8", 20, 0, 1, 1700000900)
	seedTokenIpStatsLog(t, db, token.Id, "203.0.113.8", 30, 0, 1, 1700000700)

	stats, err := model.GetUserTokenIpStats(1, []int{token.Id}, 20)
	require.NoError(t, err)
	stat := stats[token.Id]
	require.NotNil(t, stat)
	require.Len(t, stat.Ips, 1)
	assert.Equal(t, int64(1700000500), stat.Ips[0].FirstSeen)
	assert.Equal(t, int64(1700000900), stat.Ips[0].LastSeen)
	assert.Equal(t, int64(3), stat.Ips[0].Requests)
	assert.Equal(t, 60, stat.TotalTokens)
}

// ForceRecordIpLog must record the caller address even when the user left their
// own record_ip_log toggle off; without the override the feature would show
// nothing for the very accounts under investigation. Exercised through the real
// consume-log write rather than the gate directly, because the write is what the
// investigation depends on.
func TestRecordConsumeLogHonorsForceRecordIpLogOverride(t *testing.T) {
	db := newTokenIpStatsTestDB(t)
	previousForce := common.ForceRecordIpLog
	previousConsumeEnabled := common.LogConsumeEnabled
	previousRedis := common.RedisEnabled
	t.Cleanup(func() {
		common.ForceRecordIpLog = previousForce
		common.LogConsumeEnabled = previousConsumeEnabled
		common.RedisEnabled = previousRedis
	})
	common.LogConsumeEnabled = true
	// A missing user row makes GetUserSetting fail, which is the "user never
	// opted in" case the override has to rescue.
	common.RedisEnabled = false

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Request.RemoteAddr = "203.0.113.77:41234"
	c.Set("username", "victim")

	common.ForceRecordIpLog = false
	model.RecordConsumeLog(c, 4242, model.RecordConsumeLogParams{
		ModelName: "test-model", TokenName: "leaked", TokenId: 1,
		PromptTokens: 10, CompletionTokens: 5, Quota: 1,
	})
	var withoutForce model.Log
	require.NoError(t, db.Where("type = ?", model.LogTypeConsume).First(&withoutForce).Error)
	assert.Empty(t, withoutForce.Ip, "without either switch the address is not recorded")

	require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Delete(&model.Log{}).Error)
	common.ForceRecordIpLog = true
	model.RecordConsumeLog(c, 4242, model.RecordConsumeLogParams{
		ModelName: "test-model", TokenName: "leaked", TokenId: 1,
		PromptTokens: 10, CompletionTokens: 5, Quota: 1,
	})
	var withForce model.Log
	require.NoError(t, db.Where("type = ?", model.LogTypeConsume).First(&withForce).Error)
	assert.Equal(t, "203.0.113.77", withForce.Ip, "the force override records the caller address")
}

func itoa(v int) string {
	return strconv.Itoa(v)
}

func ipForIndex(i int) string {
	return "10.0.0." + strconv.Itoa(i%256)
}
