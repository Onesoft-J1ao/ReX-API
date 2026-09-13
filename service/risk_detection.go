package service

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

const (
	riskSignalRequestBurst  = "request_burst"
	riskSignalQuotaBurn     = "quota_burn"
	riskSignalFixedInterval = "fixed_interval"
)

// riskObservation is one billed request inside the sliding window.
type riskObservation struct {
	at    int64
	quota int
}

// riskTokenState is the rolling window of a single token. lastAlertAt throttles
// the alert so one abusive token cannot page the administrator per request.
type riskTokenState struct {
	observations []riskObservation
	lastAlertAt  int64
}

// riskSummary is the traffic that justified an alert, quoted in the report.
type riskSummary struct {
	requests            int
	quota               int64
	meanIntervalSeconds float64
	intervalCV          float64
}

var (
	riskMu           sync.Mutex
	riskStates       = map[int]*riskTokenState{}
	riskSweepCounter int
)

// riskSweepInterval bounds how often the map is swept for tokens that have been
// idle for a full window, so the detector stays proportional to the tokens
// currently sending traffic instead of every token that ever sent one.
const riskSweepInterval = 1024

// ObserveTokenRisk records one billed request and alerts when a token's recent
// traffic looks automated or abusive. It runs on the billing path, so it holds
// the lock only for the window update and leaves the alert to the caller.
func ObserveTokenRisk(relayInfo *relaycommon.RelayInfo, quota int) {
	if relayInfo == nil || relayInfo.TokenId <= 0 {
		return
	}
	setting := operation_setting.GetRiskControlSetting()
	if !setting.Enabled {
		return
	}
	windowSeconds := int64(setting.WindowSeconds)
	if windowSeconds <= 0 {
		windowSeconds = 60
	}

	now := time.Now().Unix()
	riskMu.Lock()
	state := riskStates[relayInfo.TokenId]
	if state == nil {
		state = &riskTokenState{}
		riskStates[relayInfo.TokenId] = state
	}
	state.observations = append(state.observations, riskObservation{at: now, quota: quota})

	cutoff := now - windowSeconds
	pruned := state.observations[:0]
	for _, observation := range state.observations {
		if observation.at >= cutoff {
			pruned = append(pruned, observation)
		}
	}
	state.observations = pruned

	riskSweepCounter++
	if riskSweepCounter >= riskSweepInterval {
		riskSweepCounter = 0
		for id, candidate := range riskStates {
			if len(candidate.observations) == 0 || candidate.observations[len(candidate.observations)-1].at < cutoff {
				delete(riskStates, id)
			}
		}
	}

	signals, summary := assessTokenRisk(pruned, setting)
	cooldown := int64(setting.AlertCooldownSeconds)
	if cooldown <= 0 {
		cooldown = 600
	}
	alert := len(signals) > 0 && now-state.lastAlertAt >= cooldown
	if alert {
		state.lastAlertAt = now
	}
	riskMu.Unlock()

	if alert {
		reportTokenRisk(relayInfo, signals, summary, setting.AutoDisableToken)
	}
}

// assessTokenRisk turns one token's window into the signals it tripped.
func assessTokenRisk(window []riskObservation, setting *operation_setting.RiskControlSetting) ([]string, riskSummary) {
	summary := riskSummary{requests: len(window)}
	for _, observation := range window {
		summary.quota += int64(observation.quota)
	}

	signals := make([]string, 0, 3)
	if setting.MaxRequestsPerWindow > 0 && summary.requests > setting.MaxRequestsPerWindow {
		signals = append(signals, riskSignalRequestBurst)
	}
	if setting.MaxQuotaPerWindow > 0 && summary.quota > int64(setting.MaxQuotaPerWindow) {
		signals = append(signals, riskSignalQuotaBurn)
	}

	// A script on a timer produces near-identical gaps between requests; a
	// person driving the API does not. A low coefficient of variation over
	// enough samples is the machine signature.
	if setting.MinRequestsForPattern > 0 && setting.IntervalCVThreshold > 0 && len(window) > setting.MinRequestsForPattern {
		samples := len(window) - 1
		var total float64
		for i := 1; i < len(window); i++ {
			total += float64(window[i].at - window[i-1].at)
		}
		mean := total / float64(samples)
		if mean > 0 {
			var squaredDeviations float64
			for i := 1; i < len(window); i++ {
				delta := float64(window[i].at-window[i-1].at) - mean
				squaredDeviations += delta * delta
			}
			summary.meanIntervalSeconds = mean
			summary.intervalCV = math.Sqrt(squaredDeviations/float64(samples)) / mean
			if summary.intervalCV < setting.IntervalCVThreshold {
				signals = append(signals, riskSignalFixedInterval)
			}
		}
	}

	return signals, summary
}

// reportTokenRisk tells the administrator about an incident and, when
// configured, disables the offending token. It runs outside the detector lock
// because it performs database and network I/O.
func reportTokenRisk(relayInfo *relaycommon.RelayInfo, signals []string, summary riskSummary, autoDisable bool) {
	token, err := model.GetTokenById(relayInfo.TokenId)
	if err != nil {
		common.SysLog(fmt.Sprintf("risk control could not load token %d: %s", relayInfo.TokenId, err.Error()))
		return
	}

	joined := strings.Join(signals, ", ")
	subject := fmt.Sprintf("Risk control flagged token %d (%s)", token.Id, joined)
	content := fmt.Sprintf(
		"Token: %s (id %d)\nUser id: %d\nSignals: %s\nRequests in window: %d\nQuota in window: %d\nMean interval: %.2fs (CV %.3f)\n",
		token.GetMaskedKey(), token.Id, token.UserId, joined, summary.requests, summary.quota, summary.meanIntervalSeconds, summary.intervalCV,
	)
	notifyRiskIncident(subject, content)

	if !autoDisable {
		return
	}
	if token.Status != common.TokenStatusEnabled {
		return
	}
	token.Status = common.TokenStatusDisabled
	if err := token.Update(); err != nil {
		common.SysLog(fmt.Sprintf("risk control could not disable token %d: %s", token.Id, err.Error()))
		return
	}
	common.SysLog(fmt.Sprintf("risk control disabled token %d of user %d after signals: %s", token.Id, token.UserId, joined))
}

// notifyRiskIncident reaches the administrator on the channel configured for
// root notifications and, unless that channel is already the webhook, sends a
// structured webhook copy as well.
func notifyRiskIncident(subject string, content string) {
	NotifyRootUser(dto.NotifyTypeRiskAlert, subject, content)

	rootSetting := model.GetRootUser().GetSetting()
	if rootSetting.NotifyType == dto.NotifyTypeWebhook || rootSetting.WebhookUrl == "" {
		return
	}
	if err := SendWebhookNotify(rootSetting.WebhookUrl, rootSetting.WebhookSecret, dto.NewNotify(dto.NotifyTypeRiskAlert, subject, content, nil)); err != nil {
		common.SysLog(fmt.Sprintf("risk control webhook notify failed: %s", err.Error()))
	}
}
