package billing_setting

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/samber/lo"
)

const (
	BillingModeRatio        = "ratio"
	BillingModeTieredExpr   = "tiered_expr"
	BillingModeField        = "billing_mode"
	BillingExprField        = "billing_expr"
	PluginBillingExprOption = "billing_setting.plugin_billing_expr"
	maxTaskExprSmokeTests   = 64
)

// The pricing sources a group may pin a model to. "price" is the per-call
// price, "ratio" the per-token ratio; neither is a BillingMode, because the
// per-call path is selected by whether a price is configured rather than by a
// mode value.
const (
	PricingSourceRatio      = "ratio"
	PricingSourcePrice      = "price"
	PricingSourceTieredExpr = "tiered_expr"
)

// BillingSetting is managed by config.GlobalConfig.Register.
// DB keys: billing_setting.billing_mode, billing_setting.billing_expr,
// billing_setting.plugin_billing_expr, billing_setting.group_pricing_source
type BillingSetting struct {
	BillingMode       map[string]string `json:"billing_mode"`
	BillingExpr       map[string]string `json:"billing_expr"`
	PluginBillingExpr map[string]string `json:"plugin_billing_expr"`
	// GroupPricingSource pins how one model is billed in one group, so the same
	// model name can be priced per token in one group and per call in another.
	// Shape: {"<group>": {"<model>": "ratio" | "price" | "tiered_expr"}}. A group
	// absent from this map leaves every model on its global billing mode.
	GroupPricingSource map[string]map[string]string `json:"group_pricing_source"`
	// GroupModelPrice overrides the per-call price of one model inside one
	// group, so the same model can cost a different amount per group without
	// changing what every other group pays. Shape: {"<group>": {"<model>": 0.5}}.
	// The group ratio still multiplies the result, exactly as it does for the
	// global per-call price, and an entry here also selects per-call billing for
	// that group. A model absent from a group's map uses its global price.
	GroupModelPrice map[string]map[string]float64 `json:"group_model_price"`
	// GroupModelRatio overrides the per-token ratio of one model inside one
	// group. This is the per-token counterpart to GroupModelPrice: a model can
	// be fed by a different upstream in each group, so the same name costs a
	// different amount per token depending on which group serves it. Shape:
	// {"<group>": {"<model>": 0.5}}. The group ratio still multiplies the
	// result, and a group absent from this map leaves the global ratio alone.
	//
	// Kept separate from GroupModelPrice because that one forces per-call
	// billing; writing a ratio there would charge one flat fee per request.
	GroupModelRatio map[string]map[string]float64 `json:"group_model_ratio"`
	// GroupModelCompletionRatio and GroupModelCacheRatio override the output and
	// cache-read multipliers of one model inside one group. They exist because a
	// group override on the input ratio alone is not enough: two upstreams can
	// quote the same model with different output/input and cache/input ratios,
	// and the global multipliers only fit one of them. Shape matches
	// GroupModelRatio. A missing entry leaves the model's global multiplier.
	GroupModelCompletionRatio map[string]map[string]float64 `json:"group_model_completion_ratio"`
	GroupModelCacheRatio      map[string]map[string]float64 `json:"group_model_cache_ratio"`
}

var billingSetting = BillingSetting{
	BillingMode:               make(map[string]string),
	BillingExpr:               make(map[string]string),
	PluginBillingExpr:         make(map[string]string),
	GroupPricingSource:        make(map[string]map[string]string),
	GroupModelPrice:           make(map[string]map[string]float64),
	GroupModelRatio:           make(map[string]map[string]float64),
	GroupModelCompletionRatio: make(map[string]map[string]float64),
	GroupModelCacheRatio:      make(map[string]map[string]float64),
}

func init() {
	config.GlobalConfig.Register("billing_setting", &billingSetting)
}

// ---------------------------------------------------------------------------
// Read accessors (hot path, must be fast)
// ---------------------------------------------------------------------------

func GetBillingMode(model string) string {
	if mode, ok := billingSetting.BillingMode[model]; ok {
		return mode
	}
	if _, ok := builtinBillingExpr[model]; ok {
		// Existing administrator-configured legacy prices take precedence over
		// a newly introduced built-in expression unless a mode was explicit.
		if ratio_setting.HasConfiguredModelRatio(model) {
			return BillingModeRatio
		}
		if _, configured := ratio_setting.GetModelPrice(model, false); configured {
			return BillingModeRatio
		}
		return BillingModeTieredExpr
	}
	return BillingModeRatio
}

// GetGroupPricingSource returns the source a group pins a model to, or "" when
// the group leaves the model on its global billing mode.
func GetGroupPricingSource(group, model string) string {
	if group == "" || model == "" {
		return ""
	}
	byModel, ok := billingSetting.GroupPricingSource[group]
	if !ok {
		return ""
	}
	return strings.TrimSpace(byModel[model])
}

func GetGroupPricingSourceCopy() map[string]map[string]string {
	if len(billingSetting.GroupPricingSource) == 0 {
		return nil
	}
	cloned := make(map[string]map[string]string, len(billingSetting.GroupPricingSource))
	for group, byModel := range billingSetting.GroupPricingSource {
		copied := make(map[string]string, len(byModel))
		maps.Copy(copied, byModel)
		cloned[group] = copied
	}
	return cloned
}

// GetGroupModelPrice returns the per-call price this group charges for this
// model, if one was configured. The caller still multiplies the group ratio.
func GetGroupModelPrice(group, model string) (float64, bool) {
	if group == "" || model == "" {
		return 0, false
	}
	byModel, ok := billingSetting.GroupModelPrice[group]
	if !ok {
		return 0, false
	}
	price, ok := byModel[model]
	return price, ok
}

// GetGroupModelPriceCopy returns a copy for the settings API.
func GetGroupModelPriceCopy() map[string]map[string]float64 {
	return cloneGroupModelMap(billingSetting.GroupModelPrice)
}

// GetGroupModelRatio returns the per-token ratio this group charges for this
// model, if one was configured. The caller still multiplies the group ratio.
func GetGroupModelRatio(group, model string) (float64, bool) {
	if group == "" || model == "" {
		return 0, false
	}
	byModel, ok := billingSetting.GroupModelRatio[group]
	if !ok {
		return 0, false
	}
	ratio, ok := byModel[model]
	return ratio, ok
}

// GetGroupModelRatioCopy returns a copy for the settings API.
func GetGroupModelRatioCopy() map[string]map[string]float64 {
	return cloneGroupModelMap(billingSetting.GroupModelRatio)
}

// GetGroupModelCompletionRatio returns the output/input multiplier this group
// charges for this model, if one was configured.
func GetGroupModelCompletionRatio(group, model string) (float64, bool) {
	return lookupGroupModelValue(billingSetting.GroupModelCompletionRatio, group, model)
}

// GetGroupModelCompletionRatioCopy returns a copy for the settings API.
func GetGroupModelCompletionRatioCopy() map[string]map[string]float64 {
	return cloneGroupModelMap(billingSetting.GroupModelCompletionRatio)
}

// GetGroupModelCacheRatio returns the cache-read multiplier this group charges
// for this model, if one was configured.
func GetGroupModelCacheRatio(group, model string) (float64, bool) {
	return lookupGroupModelValue(billingSetting.GroupModelCacheRatio, group, model)
}

// GetGroupModelCacheRatioCopy returns a copy for the settings API.
func GetGroupModelCacheRatioCopy() map[string]map[string]float64 {
	return cloneGroupModelMap(billingSetting.GroupModelCacheRatio)
}

func lookupGroupModelValue(
	set map[string]map[string]float64,
	group, model string,
) (float64, bool) {
	if group == "" || model == "" {
		return 0, false
	}
	byModel, ok := set[group]
	if !ok {
		return 0, false
	}
	value, ok := byModel[model]
	return value, ok
}

func cloneGroupModelMap(set map[string]map[string]float64) map[string]map[string]float64 {
	if len(set) == 0 {
		return nil
	}
	cloned := make(map[string]map[string]float64, len(set))
	for group, byModel := range set {
		copied := make(map[string]float64, len(byModel))
		maps.Copy(copied, byModel)
		cloned[group] = copied
	}
	return cloned
}

// ValidateGroupPricingSources parses the stored option and rejects a pin the
// model cannot honour.
//
// A pin to per-call billing on a model with no per-call price would fail every
// request in that group at billing time, and a typo in a group name would
// silently do nothing — both are cheaper to catch when the option is saved than
// when a user's request is rejected.
func ValidateGroupPricingSources(raw string) (map[string]map[string]string, error) {
	sources := make(map[string]map[string]string)
	if strings.TrimSpace(raw) == "" {
		return sources, nil
	}
	if err := common.UnmarshalJsonStr(raw, &sources); err != nil {
		return nil, fmt.Errorf("分组计费配置必须是分组到模型的 JSON 对象: %w", err)
	}

	groupRatios := ratio_setting.GetGroupRatioCopy()
	groups := make([]string, 0, len(sources))
	for group := range sources {
		groups = append(groups, group)
	}
	sort.Strings(groups)

	for _, group := range groups {
		if strings.TrimSpace(group) == "" {
			return nil, errors.New("分组名不能为空")
		}
		if _, configured := groupRatios[group]; !configured && group != "auto" {
			return nil, fmt.Errorf("分组 %q 没有配置分组倍率，按分组计费不会生效", group)
		}
		models := make([]string, 0, len(sources[group]))
		for model := range sources[group] {
			models = append(models, model)
		}
		sort.Strings(models)
		for _, model := range models {
			if err := validateGroupPricingSource(group, model, sources[group][model]); err != nil {
				return nil, err
			}
		}
	}
	return sources, nil
}

func validateGroupPricingSource(group, model, source string) error {
	if strings.TrimSpace(model) == "" {
		return fmt.Errorf("分组 %q 下存在空的模型名", group)
	}
	switch strings.TrimSpace(source) {
	case PricingSourcePrice:
		if _, configured := ratio_setting.GetModelPrice(model, false); !configured {
			return fmt.Errorf("模型 %s 没有配置按次价格，分组 %q 无法按次计费", model, group)
		}
	case PricingSourceRatio:
		if _, exists := ratio_setting.GetModelRatioPrice(model); !exists {
			return fmt.Errorf("模型 %s 没有配置按量倍率，分组 %q 无法按量计费", model, group)
		}
	case PricingSourceTieredExpr:
		if _, ok := GetBillingExpr(model); !ok {
			return fmt.Errorf("模型 %s 没有配置计费表达式，分组 %q 无法按表达式计费", model, group)
		}
	default:
		return fmt.Errorf("分组 %q 的模型 %s 计费方式 %q 无效，只支持 %s / %s / %s",
			group, model, source, PricingSourceRatio, PricingSourcePrice, PricingSourceTieredExpr)
	}
	return nil
}

// maxGroupModelPrice caps a per-call price a group may be charged. It is far
// above any real price and exists only so a typo cannot become an unbounded
// charge; the value is in USD per request.
const maxGroupModelPrice = 1_000_000.0

// ValidateGroupModelPrices parses the stored per-group price override and
// rejects anything that would misprice a request.
//
// Every value here becomes a multiplier on the customer's bill, so a NaN, an
// infinity, a negative, or an absurd number is refused at save time rather than
// reaching quota math. A group with no ratio configured is rejected for the same
// reason the source pin rejects it: the override would silently never apply.
func ValidateGroupModelPrices(raw string) (map[string]map[string]float64, error) {
	prices := make(map[string]map[string]float64)
	if strings.TrimSpace(raw) == "" {
		return prices, nil
	}
	if err := common.UnmarshalJsonStr(raw, &prices); err != nil {
		return nil, fmt.Errorf("分组定价配置必须是分组到模型的 JSON 对象: %w", err)
	}

	groupRatios := ratio_setting.GetGroupRatioCopy()
	groups := make([]string, 0, len(prices))
	for group := range prices {
		groups = append(groups, group)
	}
	sort.Strings(groups)

	for _, group := range groups {
		if strings.TrimSpace(group) == "" {
			return nil, errors.New("分组名不能为空")
		}
		if _, configured := groupRatios[group]; !configured && group != "auto" {
			return nil, fmt.Errorf("分组 %q 没有配置分组倍率，分组定价不会生效", group)
		}
		models := make([]string, 0, len(prices[group]))
		for model := range prices[group] {
			models = append(models, model)
		}
		sort.Strings(models)
		for _, model := range models {
			if strings.TrimSpace(model) == "" {
				return nil, fmt.Errorf("分组 %q 下存在空的模型名", group)
			}
			price := prices[group][model]
			if math.IsNaN(price) || math.IsInf(price, 0) {
				return nil, fmt.Errorf("模型 %s 在分组 %q 的价格不是有限数", model, group)
			}
			if price < 0 {
				return nil, fmt.Errorf("模型 %s 在分组 %q 的价格不能为负", model, group)
			}
			if price > maxGroupModelPrice {
				return nil, fmt.Errorf("模型 %s 在分组 %q 的价格超过上限 %v", model, group, maxGroupModelPrice)
			}
		}
	}
	return prices, nil
}

// maxGroupModelRatio caps a per-token ratio a group may be charged. Like
// maxGroupModelPrice it exists only so a typo cannot become an unbounded
// charge, and sits far above any real ratio.
const maxGroupModelRatio = 1_000_000.0

// ValidateGroupModelRatios parses the stored per-group ratio override and
// rejects anything that would misprice a request.
//
// Mirrors ValidateGroupModelPrices: the group must have a configured ratio (or
// be "auto") or the override would silently never apply, and each value must be
// a finite, non-negative, sane number because it multiplies the customer's bill.
func ValidateGroupModelRatios(raw string) (map[string]map[string]float64, error) {
	return validateGroupModelMap(raw, "分组倍率")
}

// ValidateGroupModelCompletionRatios and ValidateGroupModelCacheRatios mirror
// ValidateGroupModelRatios for the two multiplier maps.
func ValidateGroupModelCompletionRatios(raw string) (map[string]map[string]float64, error) {
	return validateGroupModelMap(raw, "分组输出倍率")
}

func ValidateGroupModelCacheRatios(raw string) (map[string]map[string]float64, error) {
	return validateGroupModelMap(raw, "分组缓存倍率")
}

func validateGroupModelMap(raw, label string) (map[string]map[string]float64, error) {
	ratios := make(map[string]map[string]float64)
	if strings.TrimSpace(raw) == "" {
		return ratios, nil
	}
	if err := common.UnmarshalJsonStr(raw, &ratios); err != nil {
		return nil, fmt.Errorf("%s配置必须是分组到模型的 JSON 对象: %w", label, err)
	}

	groupRatios := ratio_setting.GetGroupRatioCopy()
	groups := make([]string, 0, len(ratios))
	for group := range ratios {
		groups = append(groups, group)
	}
	sort.Strings(groups)

	for _, group := range groups {
		if strings.TrimSpace(group) == "" {
			return nil, errors.New("分组名不能为空")
		}
		if _, configured := groupRatios[group]; !configured && group != "auto" {
			return nil, fmt.Errorf("%s：分组 %q 没有配置分组倍率，覆盖不会生效", label, group)
		}
		models := make([]string, 0, len(ratios[group]))
		for model := range ratios[group] {
			models = append(models, model)
		}
		sort.Strings(models)
		for _, model := range models {
			if strings.TrimSpace(model) == "" {
				return nil, fmt.Errorf("分组 %q 下存在空的模型名", group)
			}
			ratio := ratios[group][model]
			if math.IsNaN(ratio) || math.IsInf(ratio, 0) {
				return nil, fmt.Errorf("%s：模型 %s 在分组 %q 的倍率不是有限数", label, model, group)
			}
			if ratio < 0 {
				return nil, fmt.Errorf("%s：模型 %s 在分组 %q 的倍率不能为负", label, model, group)
			}
			if ratio > maxGroupModelRatio {
				return nil, fmt.Errorf("%s：模型 %s 在分组 %q 的倍率超过上限 %v", label, model, group, maxGroupModelRatio)
			}
		}
	}
	return ratios, nil
}

func GetBillingExpr(model string) (string, bool) {
	if expr, ok := billingSetting.BillingExpr[model]; ok {
		return expr, true
	}
	if GetBillingMode(model) == BillingModeTieredExpr {
		expr, ok := builtinBillingExpr[model]
		return expr, ok
	}
	return "", false
}

func GetBuiltinBillingExpr(model string) (string, bool) {
	expression, ok := builtinBillingExpr[model]
	return expression, ok
}

func PluginBillingExprKey(pluginKey, model string) string {
	return pluginKey + "::" + model
}

func SplitPluginBillingExprKey(key string) (plugin, model string, ok bool) {
	plugin, model, ok = strings.Cut(key, "::")
	if !ok || !jsplugin.ValidPluginKey(plugin) || strings.TrimSpace(model) == "" {
		return "", "", false
	}
	return plugin, model, true
}

func GetPluginBillingExprCopy() map[string]string {
	return maps.Clone(billingSetting.PluginBillingExpr)
}

func GetPluginBillingExpr(pluginKey, model string) (string, bool) {
	expression, ok := billingSetting.PluginBillingExpr[PluginBillingExprKey(pluginKey, model)]
	return expression, ok
}

// ResolveTaskBillingExpr selects the executing plugin's override before the
// model expression, retaining the model alias fallback and explicit modes.
func ResolveTaskBillingExpr(pluginKey, model, mappedModel string) (string, bool) {
	if pluginKey != "" {
		if expr, ok := GetPluginBillingExpr(pluginKey, model); ok {
			return expr, true
		}
		if mappedModel != "" && mappedModel != model {
			if expr, ok := GetPluginBillingExpr(pluginKey, mappedModel); ok {
				return expr, true
			}
		}
	}
	if GetBillingMode(model) == BillingModeTieredExpr {
		return GetBillingExpr(model)
	}
	if mappedModel != "" && mappedModel != model && GetBillingMode(mappedModel) == BillingModeTieredExpr {
		expression, ok := GetBillingExpr(mappedModel)
		return expression, ok && strings.TrimSpace(expression) != ""
	}
	return "", false
}

// TaskExprCompatible checks the schema contract even for usage references in
// branches that the current request would not evaluate.
func TaskExprCompatible(expression string, schema map[string]jsplugin.UsageFieldSchema) bool {
	if strings.TrimSpace(expression) == "" {
		return false
	}
	if _, err := billingexpr.CompileFromCache(expression); err != nil {
		return false
	}
	for key := range billingexpr.UsedUsageKeys(expression) {
		if _, exists := schema[key]; !exists {
			return false
		}
	}
	return !billingexpr.UsesFixedPricing(expression)
}

func GetBuiltinBillingExprCopy() map[string]string {
	return lo.Assign(builtinBillingExpr)
}

func GetBillingModeCopy() map[string]string {
	modes := lo.Assign(billingSetting.BillingMode)
	for model := range builtinBillingExpr {
		if _, configured := modes[model]; !configured && GetBillingMode(model) == BillingModeTieredExpr {
			modes[model] = BillingModeTieredExpr
		}
	}
	return modes
}

func GetBillingExprCopy() map[string]string {
	expressions := lo.Assign(billingSetting.BillingExpr)
	for model := range builtinBillingExpr {
		if _, configured := expressions[model]; configured {
			continue
		}
		if expression, ok := GetBillingExpr(model); ok {
			expressions[model] = expression
		}
	}
	return expressions
}

func GetPricingSyncData(base map[string]any) map[string]any {
	extra := make(map[string]any, 2)
	if modes := GetBillingModeCopy(); len(modes) > 0 {
		extra[BillingModeField] = modes
	}
	if exprs := GetBillingExprCopy(); len(exprs) > 0 {
		extra[BillingExprField] = exprs
	}
	return lo.Assign(base, extra)
}

// ---------------------------------------------------------------------------
// Smoke test (called externally for validation before save)
// ---------------------------------------------------------------------------

func SmokeTestExpr(exprStr string) error {
	return smokeTestExpr(exprStr)
}

func smokeTestExpr(exprStr string) error {
	if _, err := billingexpr.CompileFromCache(exprStr); err != nil {
		return err
	}
	usageKeys := billingexpr.UsedUsageKeys(exprStr)
	if len(usageKeys) > 0 {
		sortedKeys := make([]string, 0, len(usageKeys))
		for key := range usageKeys {
			sortedKeys = append(sortedKeys, key)
		}
		sort.Strings(sortedKeys)
		return fmt.Errorf("expression references usage keys %v but the model has no task plugin usage schema", sortedKeys)
	}

	vectors := []billingexpr.TokenParams{
		{P: 0, C: 0, Len: 0},
		{P: 1000, C: 1000, Len: 1000},
		{P: 100000, C: 100000, Len: 100000},
		{P: 1000000, C: 1000000, Len: 1000000},
		{P: 300, C: 100, Len: 1000, CR: 100, Img: 400, ImgCR: 200},
		{P: 800, C: 50, Len: 1000, AI: 200, AO: 50},
		{Len: math.MaxInt32, ImgCR: math.MaxInt32},
	}

	for _, v := range vectors {
		for _, request := range billingExprSmokeRequests() {
			result, _, err := billingexpr.RunExprWithRequest(exprStr, v, request)
			if err != nil {
				return fmt.Errorf("vector {p=%g, c=%g}: run failed: %w", v.P, v.C, err)
			}
			if math.IsNaN(result) || math.IsInf(result, 0) || result < 0 {
				return fmt.Errorf("vector {p=%g, c=%g}: result must be finite and non-negative, got %f", v.P, v.C, result)
			}
		}
	}
	return nil
}

// SmokeTestTaskExpr validates a task usage expression against the usage facts
// declared by its plugin. Literal u() keys must be declared; dynamic calls are
// still exercised by the generated runtime vectors when possible.
func SmokeTestTaskExpr(exprStr string, schema map[string]jsplugin.UsageFieldSchema) error {
	if _, err := billingexpr.CompileFromCache(exprStr); err != nil {
		return err
	}
	if billingexpr.UsesFixedPricing(exprStr) {
		return fmt.Errorf("fixed pricing is not supported for task usage expressions")
	}
	for key := range billingexpr.UsedUsageKeys(exprStr) {
		if _, declared := schema[key]; !declared {
			return fmt.Errorf("usage key %q is not declared by the task plugin", key)
		}
	}

	for _, usage := range taskUsageSmokeVectors(schema) {
		for _, request := range billingExprSmokeRequests() {
			request.Usage = usage
			result, _, err := billingexpr.RunExprWithRequest(exprStr, billingexpr.TokenParams{}, request)
			if err != nil {
				return fmt.Errorf("usage vector %v: run failed: %w", usage, err)
			}
			if math.IsNaN(result) || math.IsInf(result, 0) || result < 0 {
				return fmt.Errorf("usage vector %v: result must be finite and non-negative, got %f", usage, result)
			}
		}
	}
	return nil
}

type usageSmokeDimension struct {
	name   string
	values []any
}

func taskUsageSmokeVectors(schema map[string]jsplugin.UsageFieldSchema) []map[string]any {
	names := make([]string, 0, len(schema))
	for name := range schema {
		names = append(names, name)
	}
	sort.Strings(names)

	dimensions := make([]usageSmokeDimension, 0, len(names))
	for _, name := range names {
		field := schema[name]
		if len(field.Enum) > 0 {
			values := make([]any, len(field.Enum))
			for index, value := range field.Enum {
				values[index] = value
			}
			dimensions = append(dimensions, usageSmokeDimension{name: name, values: values})
			continue
		}
		if field.Type == "boolean" {
			dimensions = append(dimensions, usageSmokeDimension{name: name, values: []any{false, true}})
			continue
		}
		limit := relaycommon.MaxTaskDurationSeconds
		if field.Unit == "count" {
			limit = dto.MaxImageN
		}
		if field.Unit == "token" || field.Unit == "credit" {
			limit = common.MaxQuota
		}
		dimensions = append(dimensions, usageSmokeDimension{
			name:   name,
			values: []any{float64(0), float64(1), float64(limit)},
		})
	}

	if usageSmokeCombinationCount(dimensions, maxTaskExprSmokeTests) > maxTaskExprSmokeTests {
		for index := range dimensions {
			field := schema[dimensions[index].name]
			if len(field.Enum) <= 2 {
				continue
			}
			dimensions[index].values = []any{field.Enum[0], field.Enum[len(field.Enum)-1]}
		}
	}

	vectors := make([]map[string]any, 0, maxTaskExprSmokeTests)
	var appendVectors func(int, map[string]any)
	appendVectors = func(index int, current map[string]any) {
		if len(vectors) >= maxTaskExprSmokeTests {
			return
		}
		if index == len(dimensions) {
			vector := make(map[string]any, len(current))
			maps.Copy(vector, current)
			vectors = append(vectors, vector)
			return
		}
		for _, value := range dimensions[index].values {
			current[dimensions[index].name] = value
			appendVectors(index+1, current)
		}
		delete(current, dimensions[index].name)
	}
	appendVectors(0, make(map[string]any, len(dimensions)))

	combinationCount := usageSmokeCombinationCount(dimensions, maxTaskExprSmokeTests)
	if combinationCount > maxTaskExprSmokeTests && len(vectors) > 0 {
		last := make(map[string]any, len(dimensions))
		for _, dimension := range dimensions {
			last[dimension.name] = dimension.values[len(dimension.values)-1]
		}
		vectors[len(vectors)-1] = last
	}
	return vectors
}

func usageSmokeCombinationCount(dimensions []usageSmokeDimension, stopAfter int) int {
	count := 1
	for _, dimension := range dimensions {
		if len(dimension.values) == 0 {
			return 0
		}
		if count > stopAfter/len(dimension.values) {
			return stopAfter + 1
		}
		count *= len(dimension.values)
	}
	return count
}

func billingExprSmokeRequests() []billingexpr.RequestInput {
	return []billingexpr.RequestInput{
		{},
		{
			Headers: map[string]string{
				"anthropic-beta": "fast-mode-2026-02-01",
			},
			Body: []byte(`{"service_tier":"fast","size":"4096x4096","stream_options":{"include_usage":true},"messages":[1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21]}`),
		},
	}
}
