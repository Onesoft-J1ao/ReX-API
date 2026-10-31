package ratio_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An unpriced model must not look priced. GetModelRatio used to answer with the
// self-use-mode flag, so every unpriced model on a self-use gateway claimed a
// configured ratio of 37.5 and the model square rendered it at 75.
// restoreModelRatios puts the global ratio map back after a test that replaced
// it, so the replacement cannot leak into whichever test runs next.
func restoreModelRatios(t *testing.T) {
	t.Helper()
	snapshot := modelRatioMap.MarshalJSONString()
	t.Cleanup(func() {
		require.NoError(t, UpdateModelRatioByJSONString(snapshot))
	})
}

func TestGetModelRatioReportsUnpricedModelsAsUnconfigured(t *testing.T) {
	restoreModelRatios(t)
	previous := operation_setting.SelfUseModeEnabled
	operation_setting.SelfUseModeEnabled = true
	t.Cleanup(func() { operation_setting.SelfUseModeEnabled = previous })

	require.NoError(t, UpdateModelRatioByJSONString(`{"lookup-test-priced-model":1.5}`))

	ratio, configured, name := GetModelRatio("lookup-test-priced-model")
	assert.Equal(t, 1.5, ratio)
	assert.True(t, configured)
	assert.Equal(t, "lookup-test-priced-model", name)

	// The sentinel keeps quota math from dividing by zero, but the flag must
	// say the model has no price.
	ratio, configured, _ = GetModelRatio("lookup-test-unpriced-model")
	assert.Equal(t, 37.5, ratio)
	assert.False(t, configured, "self-use mode is not a configured ratio")
}

func TestGetModelRatioPriceNeverReturnsTheUnpricedSentinel(t *testing.T) {
	restoreModelRatios(t)
	require.NoError(t, UpdateModelRatioByJSONString(`{"lookup-test-priced-model":2}`))

	ratio, ok := GetModelRatioPrice("lookup-test-priced-model")
	assert.True(t, ok)
	assert.Equal(t, 2.0, ratio)

	ratio, ok = GetModelRatioPrice("lookup-test-unpriced-model")
	assert.False(t, ok)
	assert.Zero(t, ratio, "an unpriced model must not surface the sentinel as its price")
}

func TestGetModelRatioOrPriceDistinguishesUnpricedModels(t *testing.T) {
	restoreModelRatios(t)
	require.NoError(t, UpdateModelRatioByJSONString(`{"lookup-test-priced-model":2}`))

	ratio, usePrice, exists := GetModelRatioOrPrice("lookup-test-priced-model")
	assert.True(t, exists)
	assert.False(t, usePrice)
	assert.Equal(t, 2.0, ratio)

	_, _, exists = GetModelRatioOrPrice("lookup-test-unpriced-model")
	assert.False(t, exists)
}
