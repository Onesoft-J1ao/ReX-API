/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package helper

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loadGroupPricingFixture installs ratios, per-call prices and group pins that
// survive until the test ends, so a later case cannot see them.
func loadGroupPricingFixture(t *testing.T, groupSources string) {
	t.Helper()

	savedConfig := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		savedConfig[key] = value
		return nil
	}))
	savedRatios := ratio_setting.ModelRatio2JSONString()
	savedPrices, err := common.Marshal(ratio_setting.GetModelPriceMap())
	require.NoError(t, err)
	savedGroupRatios, err := common.Marshal(ratio_setting.GetGroupRatioCopy())
	require.NoError(t, err)
	oldSelfUse := operation_setting.SelfUseModeEnabled
	operation_setting.SelfUseModeEnabled = false

	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(savedConfig))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(savedRatios))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(string(savedPrices)))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(string(savedGroupRatios)))
		operation_setting.SelfUseModeEnabled = oldSelfUse
	})

	// A model priced both ways, so the pin is the only thing that decides.
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"group-priced-model": 2}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"group-priced-model": 0.05}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"token-group": 1, "call-group": 1, "expr-group": 1, "default-group": 1}`))

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode":         `{"group-priced-model":"ratio"}`,
		"billing_setting.billing_expr":         `{"group-priced-model":"tier(\"base\", p * 3 + c * 15)"}`,
		"billing_setting.group_pricing_source": groupSources,
	}))
}

// TestModelPriceHelperGroupPricingSource pins the same model to a different
// pricing source per group.
//
// The model is priced both ways, so the per-call price would otherwise win in
// every group — that is the existing precedence, and the pin is what overrides
// it. A group with no pin keeps that existing answer.
func TestModelPriceHelperGroupPricingSource(t *testing.T) {
	gin.SetMode(gin.TestMode)
	loadGroupPricingFixture(t, `{"call-group":{"group-priced-model":"price"},"expr-group":{"group-priced-model":"tiered_expr"},"token-group":{"group-priced-model":"ratio"}}`)

	tests := []struct {
		name     string
		group    string
		wantMode string
		wantUse  bool
	}{
		{name: "a group pinned to the per-call price", group: "call-group", wantMode: "price", wantUse: true},
		{name: "a group pinned to the expression", group: "expr-group", wantMode: "tiered_expr", wantUse: false},
		{name: "a group pinned back to the token ratio", group: "token-group", wantMode: "ratio", wantUse: false},
		{name: "an unpinned group keeps the existing precedence", group: "default-group", wantMode: "price", wantUse: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Set("group", tc.group)
			info := &relaycommon.RelayInfo{
				OriginModelName: "group-priced-model",
				UserGroup:       tc.group,
				UsingGroup:      tc.group,
			}

			priceData, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{})
			require.NoError(t, err)

			switch tc.wantMode {
			case "price":
				assert.True(t, priceData.UsePrice, "the group pins this model to per-call billing")
				assert.Equal(t, 0.05, priceData.ModelPrice)
			case "tiered_expr":
				assert.NotNil(t, info.TieredBillingSnapshot, "the group pins this model to its expression")
				assert.Equal(t, "tiered_expr", info.TieredBillingSnapshot.BillingMode)
			default:
				assert.False(t, priceData.UsePrice, "the group pins this model back to per-token billing")
				assert.Equal(t, 2.0, priceData.ModelRatio)
			}
		})
	}
}

// A per-call pin on a model whose price was later removed must fail loudly.
// Charging the per-call path at zero would hand out free requests.
func TestModelPriceHelperGroupPricingSourcePriceRemoved(t *testing.T) {
	gin.SetMode(gin.TestMode)
	loadGroupPricingFixture(t, `{"call-group":{"group-priced-model":"price"}}`)
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{}`))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("group", "call-group")
	info := &relaycommon.RelayInfo{
		OriginModelName: "group-priced-model",
		UserGroup:       "call-group",
		UsingGroup:      "call-group",
	}

	_, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "call-group")
}

func TestValidateGroupPricingSources(t *testing.T) {
	gin.SetMode(gin.TestMode)
	loadGroupPricingFixture(t, `{}`)

	tests := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{
			name: "a valid pin for every source",
			raw:  `{"call-group":{"group-priced-model":"price"},"expr-group":{"group-priced-model":"tiered_expr"},"token-group":{"group-priced-model":"ratio"}}`,
		},
		{
			name: "an empty value clears the pins",
			raw:  ``,
		},
		{
			name:    "a group with no group ratio would never apply",
			raw:     `{"no-such-group":{"group-priced-model":"price"}}`,
			wantErr: "no-such-group",
		},
		{
			name:    "a per-call pin on a model with no price",
			raw:     `{"call-group":{"no-price-model":"price"}}`,
			wantErr: "no-price-model",
		},
		{
			name:    "a ratio pin on a model with no ratio",
			raw:     `{"call-group":{"no-ratio-model":"ratio"}}`,
			wantErr: "no-ratio-model",
		},
		{
			name:    "an unknown source",
			raw:     `{"call-group":{"group-priced-model":"per-kg"}}`,
			wantErr: "per-kg",
		},
		{
			name:    "malformed json",
			raw:     `{"call-group":`,
			wantErr: "JSON",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := billing_setting.ValidateGroupPricingSources(tc.raw)
			if tc.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

// TestModelPriceHelperGroupModelPrice charges one group a different per-call
// price for the same model, leaving every other group on the global price.
func TestModelPriceHelperGroupModelPrice(t *testing.T) {
	gin.SetMode(gin.TestMode)
	loadGroupPricingFixture(t, `{}`)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.group_model_price": `{"call-group":{"group-priced-model":0.5}}`,
	}))

	tests := []struct {
		name      string
		group     string
		wantPrice float64
	}{
		{name: "the group with its own price", group: "call-group", wantPrice: 0.5},
		{name: "a group without one keeps the global price", group: "default-group", wantPrice: 0.05},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Set("group", tc.group)
			info := &relaycommon.RelayInfo{
				OriginModelName: "group-priced-model",
				UserGroup:       tc.group,
				UsingGroup:      tc.group,
			}

			priceData, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{})
			require.NoError(t, err)
			assert.True(t, priceData.UsePrice, "a group price selects per-call billing")
			assert.Equal(t, tc.wantPrice, priceData.ModelPrice)
		})
	}
}

// A group price that is not a finite non-negative number would become a
// multiplier on the customer's bill, so it is refused when the option is saved.
func TestValidateGroupModelPrices(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// The fixture configures group ratios for call-group and default-group; a
	// price under a group with no ratio is rejected, so the cases use those.
	loadGroupPricingFixture(t, `{}`)

	tests := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{name: "empty is allowed", raw: ""},
		{name: "a plain price", raw: `{"call-group":{"m":0.5}}`},
		{name: "zero is allowed", raw: `{"call-group":{"m":0}}`},
		{name: "negative", raw: `{"call-group":{"m":-1}}`, wantErr: "不能为负"},
		{name: "not a number", raw: `{"call-group":{"m":"x"}}`, wantErr: "JSON"},
		{name: "empty model name", raw: `{"call-group":{"":1}}`, wantErr: "空的模型名"},
		{name: "empty group name", raw: `{"":{"m":1}}`, wantErr: "分组名不能为空"},
		{name: "a group with no ratio", raw: `{"nope":{"m":1}}`, wantErr: "没有配置分组倍率"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := billing_setting.ValidateGroupModelPrices(tc.raw)
			if tc.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}
