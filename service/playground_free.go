package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

// playgroundVisitorIdentity resolves who is calling the playground. The guard
// middleware records the fingerprint and the caller IP; the IP is the fallback
// for clients that send no fingerprint.
func playgroundVisitorIdentity(c *gin.Context) (fingerprint string, ip string) {
	fingerprint = common.GetContextKeyString(c, constant.ContextKeyPlaygroundFingerprint)
	ip = common.GetContextKeyString(c, constant.ContextKeyPlaygroundIP)
	// Billing is also exercised from tests and internal callers that build a
	// context without a request, where there is no client IP to read.
	if ip == "" && c.Request != nil {
		ip = c.ClientIP()
	}
	return fingerprint, ip
}

// ShouldServeFromPlaygroundFreeTier reports whether this request may be served
// from the free playground allowance. A lookup failure counts as "not covered",
// so a database problem can never hand out unlimited free usage.
func ShouldServeFromPlaygroundFreeTier(c *gin.Context, relayInfo *relaycommon.RelayInfo) bool {
	if relayInfo == nil || !relayInfo.IsPlayground {
		return false
	}
	if !operation_setting.GetPlaygroundSetting().Enabled {
		return false
	}
	fingerprint, ip := playgroundVisitorIdentity(c)
	available, err := model.PlaygroundFreeAllowanceAvailable(fingerprint, ip)
	if err != nil {
		common.SysLog("playground free allowance lookup failed: " + err.Error())
		return false
	}
	return available
}

// RecordPlaygroundFreeConsumption books one covered request: a generation plus
// the tokens the relay reported for it. Image generations report no tokens, so
// the generation budget is what bounds them.
func RecordPlaygroundFreeConsumption(c *gin.Context, relayInfo *relaycommon.RelayInfo, tokens int64) {
	if relayInfo == nil || !relayInfo.PlaygroundFreeTier {
		return
	}
	fingerprint, ip := playgroundVisitorIdentity(c)
	if err := model.RecordPlaygroundFreeUsage(fingerprint, ip, tokens); err != nil {
		common.SysLog("playground free allowance could not be recorded: " + err.Error())
	}
}
