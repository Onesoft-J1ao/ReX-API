package service

import (
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// regularTraffic ticks every five seconds, the signature of a client on a timer.
func regularTraffic(samples int) []riskObservation {
	observations := make([]riskObservation, 0, samples)
	for i := range samples {
		observations = append(observations, riskObservation{at: int64(i * 5), quota: 1})
	}
	return observations
}

// irregularTraffic uses uneven gaps of the size a person driving the API leaves.
func irregularTraffic() []riskObservation {
	gaps := []int{1, 3, 1, 40, 2, 9, 1, 30, 4, 1, 17}
	observations := make([]riskObservation, 0, len(gaps)+1)
	at := int64(0)
	observations = append(observations, riskObservation{at: at, quota: 1})
	for _, gap := range gaps {
		at += int64(gap)
		observations = append(observations, riskObservation{at: at, quota: 1})
	}
	return observations
}

func TestAssessTokenRiskSignals(t *testing.T) {
	base := operation_setting.RiskControlSetting{
		Enabled:               true,
		WindowSeconds:         60,
		MaxRequestsPerWindow:  5,
		MaxQuotaPerWindow:     1000,
		MinRequestsForPattern: 8,
		IntervalCVThreshold:   0.05,
	}

	heavyQuota := []riskObservation{
		{at: 0, quota: 500},
		{at: 7, quota: 500},
		{at: 23, quota: 500},
		{at: 51, quota: 500},
	}

	withoutBounds := base
	withoutBounds.MaxRequestsPerWindow = 0
	withoutBounds.MaxQuotaPerWindow = 0
	withoutBounds.IntervalCVThreshold = 0

	belowPatternFloor := base
	belowPatternFloor.MaxRequestsPerWindow = 0

	tests := []struct {
		name     string
		window   []riskObservation
		setting  operation_setting.RiskControlSetting
		expected []string
	}{
		{
			name:     "a quiet token trips nothing",
			window:   regularTraffic(3),
			setting:  base,
			expected: []string{},
		},
		{
			name:     "uneven traffic trips the request burst alone",
			window:   irregularTraffic(),
			setting:  base,
			expected: []string{riskSignalRequestBurst},
		},
		{
			name:     "evenly spaced traffic also looks machine-regular",
			window:   regularTraffic(12),
			setting:  base,
			expected: []string{riskSignalRequestBurst, riskSignalFixedInterval},
		},
		{
			name:     "a few expensive calls trip the quota burn alone",
			window:   heavyQuota,
			setting:  base,
			expected: []string{riskSignalQuotaBurn},
		},
		{
			name:     "zeroed budgets disable every signal",
			window:   regularTraffic(12),
			setting:  withoutBounds,
			expected: []string{},
		},
		{
			name:     "too few samples to judge regularity",
			window:   regularTraffic(6),
			setting:  belowPatternFloor,
			expected: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			signals, summary := assessTokenRisk(tc.window, &tc.setting)
			assert.Equal(t, tc.expected, signals)
			assert.Equal(t, len(tc.window), summary.requests)
		})
	}
}

func TestAssessTokenRiskMeasuresEvenSpacing(t *testing.T) {
	setting := operation_setting.RiskControlSetting{
		MaxRequestsPerWindow:  0,
		MaxQuotaPerWindow:     0,
		MinRequestsForPattern: 8,
		IntervalCVThreshold:   0.05,
	}

	signals, summary := assessTokenRisk(regularTraffic(12), &setting)
	assert.Equal(t, []string{riskSignalFixedInterval}, signals)
	assert.Equal(t, 5.0, summary.meanIntervalSeconds)
	assert.Less(t, summary.intervalCV, 0.05)

	signals, summary = assessTokenRisk(irregularTraffic(), &setting)
	assert.Empty(t, signals)
	assert.Greater(t, summary.intervalCV, 0.05)
}

func TestObserveTokenRiskRecordsAndPrunesWindow(t *testing.T) {
	setting := operation_setting.GetRiskControlSetting()
	restoreSetting := *setting
	riskMu.Lock()
	restoreStates := riskStates
	riskStates = map[int]*riskTokenState{}
	riskMu.Unlock()
	t.Cleanup(func() {
		*setting = restoreSetting
		riskMu.Lock()
		riskStates = restoreStates
		riskMu.Unlock()
	})

	setting.Enabled = false
	ObserveTokenRisk(&relaycommon.RelayInfo{TokenId: 7}, 10)
	riskMu.Lock()
	require.Empty(t, riskStates, "a disabled detector must not record traffic")
	riskMu.Unlock()

	setting.Enabled = true
	setting.WindowSeconds = 60
	setting.MaxRequestsPerWindow = 0
	setting.MaxQuotaPerWindow = 0
	setting.MinRequestsForPattern = 0
	setting.IntervalCVThreshold = 0

	info := &relaycommon.RelayInfo{TokenId: 7}
	ObserveTokenRisk(info, 10)
	ObserveTokenRisk(info, 20)

	riskMu.Lock()
	require.Len(t, riskStates[7].observations, 2)
	riskStates[7].observations[0].at = time.Now().Unix() - 3600
	riskMu.Unlock()

	ObserveTokenRisk(info, 30)

	riskMu.Lock()
	defer riskMu.Unlock()
	require.Len(t, riskStates[7].observations, 2, "the sample outside the window must be dropped")
	assert.Equal(t, 20, riskStates[7].observations[0].quota)
	assert.Equal(t, 30, riskStates[7].observations[1].quota)
}

func TestObserveTokenRiskIgnoresTrafficWithoutToken(t *testing.T) {
	setting := operation_setting.GetRiskControlSetting()
	restoreSetting := *setting
	riskMu.Lock()
	restoreStates := riskStates
	riskStates = map[int]*riskTokenState{}
	riskMu.Unlock()
	t.Cleanup(func() {
		*setting = restoreSetting
		riskMu.Lock()
		riskStates = restoreStates
		riskMu.Unlock()
	})

	setting.Enabled = true
	ObserveTokenRisk(nil, 10)
	ObserveTokenRisk(&relaycommon.RelayInfo{TokenId: 0}, 10)

	riskMu.Lock()
	defer riskMu.Unlock()
	assert.Empty(t, riskStates)
}

func TestObserveTokenRiskSweepsIdleTokens(t *testing.T) {
	setting := operation_setting.GetRiskControlSetting()
	restoreSetting := *setting
	riskMu.Lock()
	restoreStates := riskStates
	restoreCounter := riskSweepCounter
	riskStates = map[int]*riskTokenState{
		99: {observations: []riskObservation{{at: time.Now().Unix() - 3600, quota: 1}}},
	}
	riskSweepCounter = riskSweepInterval - 1
	riskMu.Unlock()
	t.Cleanup(func() {
		*setting = restoreSetting
		riskMu.Lock()
		riskStates = restoreStates
		riskSweepCounter = restoreCounter
		riskMu.Unlock()
	})

	setting.Enabled = true
	setting.WindowSeconds = 60
	setting.MaxRequestsPerWindow = 0
	setting.MaxQuotaPerWindow = 0
	setting.MinRequestsForPattern = 0
	setting.IntervalCVThreshold = 0

	ObserveTokenRisk(&relaycommon.RelayInfo{TokenId: 7}, 10)

	riskMu.Lock()
	defer riskMu.Unlock()
	assert.NotContains(t, riskStates, 99, "a token idle for a full window must be swept")
	assert.Contains(t, riskStates, 7, "the token that just sent traffic must be kept")
}
