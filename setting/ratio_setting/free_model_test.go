package ratio_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Probing a paid model bills a real upstream request every cycle, so the
// predicate that decides which groups are safe to probe has to be exact. In
// particular an unconfigured model falls back to a default ratio rather than a
// zero price, and must never be mistaken for free.
func TestIsModelFreeInGroup(t *testing.T) {
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})

	// The group map is a registered config, so it loads through the config
	// manager. The price and ratio maps are plain option keys with their own
	// loaders and are not reached by LoadFromDB.
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"group_ratio_setting.group_ratio": `{"free-group":0,"paid-group":1}`,
	}))
	require.NoError(t, UpdateModelPriceByJSONString(`{"priced-model":0.01,"zero-priced-model":0}`))
	require.NoError(t, UpdateModelRatioByJSONString(`{"free-ratio-model":0,"paid-ratio-model":2}`))

	for _, tc := range []struct {
		name      string
		model     string
		group     string
		wantFree  bool
		rationale string
	}{
		{"zero group ratio is free whatever the model", "paid-ratio-model", "free-group", true,
			"a group priced at zero costs nothing regardless of the model entry"},
		{"per-request model with a price is paid", "priced-model", "paid-group", false,
			"this is the case that bills us on every probe"},
		{"per-request model priced at zero is free", "zero-priced-model", "paid-group", true,
			"an explicit zero price means the probe is free"},
		{"ratio model priced at zero is free", "free-ratio-model", "paid-group", true,
			"token-priced models reach zero through the ratio map"},
		{"ratio model with a ratio is paid", "paid-ratio-model", "paid-group", false,
			"a non-zero ratio bills per token"},
		{"unconfigured model is paid", "never-configured-model", "paid-group", false,
			"an unconfigured model falls back to a default ratio, which is not zero"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantFree, IsModelFreeInGroup(tc.model, tc.group), tc.rationale)
		})
	}
}
