package helper

import (
	"fmt"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/reasoning"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	hostreasoning "github.com/QuantumNous/new-api/setting/reasoning"
	hosttypes "github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func modelPriceNotConfiguredError(modelName string, userId int) error {
	if model.IsAdmin(userId) {
		return fmt.Errorf(
			"模型 %s 的价格未配置。请前往「系统设置 → 运营设置」开启自用模式，或在「系统设置 → 分组与模型定价设置」中为该模型配置价格；"+
				"Model %s price not configured. Go to System Settings → Operation Settings to enable self-use mode, or configure the model price in System Settings → Group & Model Pricing.",
			modelName, modelName,
		)
	}
	return fmt.Errorf(
		"模型 %s 的价格尚未由管理员配置，暂时无法使用，请联系站点管理员开启该模型；"+
			"Model %s has not been priced by the administrator yet. Please contact the site administrator to enable this model.",
		modelName, modelName,
	)
}

// groupPricingSourceUnavailableError reports a group pin the model can no longer
// honour: the operator pinned this model to per-call billing in this group and
// the per-call price has since been removed. Failing the request is the safe
// reading; the alternative is billing the per-call path at a zero price.
func groupPricingSourceUnavailableError(modelName, group, source string) error {
	return fmt.Errorf(
		"分组 %s 把模型 %s 指定为「%s」计费，但该模型没有配置对应的价格，请管理员补上或移除该分组覆盖；"+
			"Group %s pins model %s to %q billing, but that price is not configured. Configure it or remove the group override.",
		group, modelName, source, group, modelName, source,
	)
}

// https://docs.claude.com/en/docs/build-with-claude/prompt-caching#1-hour-cache-duration
const claudeCacheCreation1hMultiplier = 6 / 3.75

// defaultTieredPreConsumeMaxTokens is the fallback completion-token estimate
// used for tiered expression pre-consume when the client omits max_tokens, so
// the pre-consumed quota still reflects a plausible output cost in paid groups.
const defaultTieredPreConsumeMaxTokens = 8192

func isFreeUnitPrice(price float64) bool {
	return price >= 0 && !math.IsNaN(price) && !math.IsInf(price, 0) && price < common.FreeModelPriceThresholdUSD
}

func isFreeTokenPricing(modelRatio, completionRatio, groupRatio float64) bool {
	inputPrice := modelRatio * 2 * groupRatio
	outputPrice := inputPrice * completionRatio
	// Both sides have to be free. A completion ratio of zero makes the output
	// price zero, and treating that alone as free would drop the charge for a
	// model whose input ratio is still being paid for.
	return isFreeUnitPrice(inputPrice) && isFreeUnitPrice(outputPrice)
}

func resolveAutoRoutePricingModel(c *gin.Context, modelName string) string {
	if !strings.HasPrefix(modelName, "auto/") {
		return modelName
	}
	value, ok := common.GetContextKey(c, constant.ContextKeyTokenAutoRoutes)
	if !ok {
		return modelName
	}
	routes, ok := value.(map[string][]string)
	if !ok || len(routes[modelName]) == 0 {
		return modelName
	}
	if _, configured := ratio_setting.GetModelPrice(modelName, false); configured {
		return modelName
	}
	if _, configured := ratio_setting.GetModelRatioPrice(modelName); configured {
		return modelName
	}
	return routes[modelName][0]
}

// HandleGroupRatio checks for "auto_group" in the context and updates the group ratio and relayInfo.UsingGroup if present
func HandleGroupRatio(ctx *gin.Context, relayInfo *relaycommon.RelayInfo) hosttypes.GroupRatioInfo {
	groupRatioInfo := hosttypes.GroupRatioInfo{
		GroupRatio:        1.0, // default ratio
		GroupSpecialRatio: -1,
	}

	// check auto group
	autoGroup, exists := ctx.Get("auto_group")
	if exists {
		logger.LogDebug(ctx, "final group: %s", autoGroup)
		relayInfo.UsingGroup = autoGroup.(string)
	}

	// check user group special ratio
	userGroupRatio, ok := ratio_setting.GetGroupGroupRatio(relayInfo.UserGroup, relayInfo.UsingGroup)
	if ok {
		// user group special ratio
		groupRatioInfo.GroupSpecialRatio = userGroupRatio
		groupRatioInfo.GroupRatio = userGroupRatio
		groupRatioInfo.HasSpecialRatio = true
	} else {
		// normal group ratio
		groupRatioInfo.GroupRatio = ratio_setting.GetGroupRatio(relayInfo.UsingGroup)
	}

	return groupRatioInfo
}

func ModelPriceHelper(c *gin.Context, info *relaycommon.RelayInfo, promptTokens int, meta *types.TokenCountMeta) (hosttypes.PriceData, error) {
	originModelName := info.OriginModelName
	if pricingModelName := resolveAutoRoutePricingModel(c, originModelName); pricingModelName != originModelName {
		info.OriginModelName = pricingModelName
		defer func() { info.OriginModelName = originModelName }()
	}
	if matched := resolveBillingModelName(info.GetOriginModelName()); matched != "" && matched != info.OriginModelName {
		info.BillingModelName = matched
	}
	billingModelName := info.GetBillingModelName()
	modelPrice, usePrice := ratio_setting.GetModelPrice(billingModelName, false)
	configuredModelPrice := modelPrice

	groupRatioInfo := HandleGroupRatio(c, info)

	// A group may charge its own per-call price for a model, so the same model
	// costs a different amount in different groups without touching what every
	// other group pays. Resolved after HandleGroupRatio because that is what
	// settles which group the request is actually routed through. The group
	// ratio still multiplies the result further down, exactly as it does for the
	// global per-call price, and an override also selects per-call billing.
	if groupPrice, ok := billing_setting.GetGroupModelPrice(info.UsingGroup, billingModelName); ok {
		modelPrice = groupPrice
		configuredModelPrice = groupPrice
		usePrice = true
	}

	// A group may pin this model to a different pricing source than the global
	// one, so the same model name is billed per token in one group and per call
	// in another.
	groupSource := billing_setting.GetGroupPricingSource(info.UsingGroup, billingModelName)
	switch groupSource {
	case billing_setting.PricingSourceTieredExpr:
		return modelPriceHelperTiered(c, info, billingModelName, promptTokens, groupRatioInfo)
	case billing_setting.PricingSourcePrice:
		if !usePrice {
			// The save-time validation rejects this, but a price can be removed
			// after the pin was stored; charging zero would be a silent giveaway.
			return hosttypes.PriceData{}, groupPricingSourceUnavailableError(billingModelName, info.UsingGroup, groupSource)
		}
		usePrice = true
	case billing_setting.PricingSourceRatio:
		usePrice = false
	}

	// Check if this model uses tiered_expr billing
	if groupSource == "" && billing_setting.GetBillingMode(billingModelName) == billing_setting.BillingModeTieredExpr {
		return modelPriceHelperTiered(c, info, billingModelName, promptTokens, groupRatioInfo)
	}

	var preConsumedQuota int
	var modelRatio float64
	var completionRatio float64
	var cacheRatio float64
	var imageRatio float64
	var cacheCreationRatio float64
	var cacheCreationRatio5m float64
	var cacheCreationRatio1h float64
	var audioRatio float64
	var audioCompletionRatio float64
	var freeModel bool
	if !usePrice {
		preConsumeMultiplier, err := operation_setting.InputPreConsumeMultiplier()
		if err != nil {
			return hosttypes.PriceData{}, err
		}
		preConsumedTokens := float64(promptTokens) * preConsumeMultiplier
		var success bool
		var matchName string
		modelRatio, success, matchName = ratio_setting.GetModelRatio(billingModelName)
		if groupRatio, ok := billing_setting.GetGroupModelRatio(info.UsingGroup, billingModelName); ok {
			modelRatio = groupRatio
			success = true
		}
		if !success {
			// Self-use mode means "serve unpriced models anyway"; it is not a
			// price. Without it the request is rejected rather than billed at
			// the unpriced sentinel.
			acceptUnsetRatio := operation_setting.SelfUseModeEnabled ||
				info.UserSetting.AcceptUnsetRatioModel
			if !acceptUnsetRatio {
				return hosttypes.PriceData{}, modelPriceNotConfiguredError(matchName, info.UserId)
			}
		}
		// A model can be fed by a different upstream in each group, so its
		// per-token cost differs by group. Resolved after the global ratio so a
		// group without an override keeps the global figure untouched.
		completionRatio = ratio_setting.GetCompletionRatio(billingModelName)
		if groupCompletion, ok := billing_setting.GetGroupModelCompletionRatio(info.UsingGroup, billingModelName); ok {
			completionRatio = groupCompletion
		}
		cacheRatio, _ = ratio_setting.GetCacheRatio(billingModelName)
		if groupCache, ok := billing_setting.GetGroupModelCacheRatio(info.UsingGroup, billingModelName); ok {
			cacheRatio = groupCache
		}
		cacheCreationRatio, _ = ratio_setting.GetCreateCacheRatio(billingModelName)
		cacheCreationRatio5m = cacheCreationRatio
		// 固定1h和5min缓存写入价格的比例
		cacheCreationRatio1h = cacheCreationRatio * claudeCacheCreation1hMultiplier
		imageRatio, _ = ratio_setting.GetImageRatio(billingModelName)
		audioRatio = ratio_setting.GetAudioRatio(billingModelName)
		audioCompletionRatio = ratio_setting.GetAudioCompletionRatio(billingModelName)
		ratio := modelRatio * groupRatioInfo.GroupRatio
		quota, err := common.QuotaFromFloatStrict(preConsumedTokens * ratio)
		if err != nil {
			return hosttypes.PriceData{}, err
		}
		preConsumedQuota = quota
		if _, image := info.Request.(*dto.ImageRequest); image {
			info.ImageQuotaBeforeGroup = preConsumedTokens * modelRatio
		}
	} else {
		if meta.ImagePriceRatio != 0 {
			modelPrice = modelPrice * meta.ImagePriceRatio
		}
		if _, image := info.Request.(*dto.ImageRequest); image {
			info.ImageQuotaBeforeGroup = modelPrice * common.QuotaPerUnit
		}
	}

	billingFree := isFreeTokenPricing(modelRatio, completionRatio, groupRatioInfo.GroupRatio)
	if usePrice {
		billingFree = isFreeUnitPrice(configuredModelPrice * groupRatioInfo.GroupRatio)
	}
	if billingFree && !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
		preConsumedQuota = 0
		freeModel = true
	}

	priceData := hosttypes.PriceData{
		FreeModel:            freeModel,
		BillingFree:          billingFree,
		ModelPrice:           modelPrice,
		ModelRatio:           modelRatio,
		CompletionRatio:      completionRatio,
		GroupRatioInfo:       groupRatioInfo,
		UsePrice:             usePrice,
		CacheRatio:           cacheRatio,
		ImageRatio:           imageRatio,
		AudioRatio:           audioRatio,
		AudioCompletionRatio: audioCompletionRatio,
		CacheCreationRatio:   cacheCreationRatio,
		CacheCreation5mRatio: cacheCreationRatio5m,
		CacheCreation1hRatio: cacheCreationRatio1h,
		QuotaToPreConsume:    preConsumedQuota,
	}
	if usePrice {
		for name, ratio := range meta.BillingRatios {
			priceData.AddOtherRatio(name, ratio)
		}
	}
	if request, image := info.Request.(*dto.ImageRequest); image {
		count, err := request.ImageCount(false)
		if err != nil {
			return hosttypes.PriceData{}, err
		}
		if usePrice {
			priceData.AddOtherRatio("n", float64(count))
		}
		if !usePrice {
			quota, err := common.QuotaFromFloatStrict(priceData.ApplyOtherRatiosToFloat(info.ImageQuotaBeforeGroup * groupRatioInfo.GroupRatio))
			if err != nil {
				return hosttypes.PriceData{}, err
			}
			priceData.QuotaToPreConsume = quota
		}
	}
	if usePrice {
		quotaToPreConsume := priceData.ApplyOtherRatiosToFloat(modelPrice * common.QuotaPerUnit * groupRatioInfo.GroupRatio)
		quota, err := common.QuotaFromFloatStrict(quotaToPreConsume)
		if err != nil {
			return hosttypes.PriceData{}, err
		}
		priceData.QuotaToPreConsume = quota
	}
	if priceData.FreeModel {
		priceData.QuotaToPreConsume = 0
	}

	// A model priced at zero is not free: charge the configured minimum so a
	// wallet with no balance fails the pre-consume and has to top up first. A
	// zero *group* ratio stays genuinely free — that is an explicit admin
	// decision to open a group, not an unconfigured model. This runs last
	// because the per-call (usePrice) branch above rewrites QuotaToPreConsume.
	if groupRatioInfo.GroupRatio != 0 && operation_setting.ModelPricedAtZero(usePrice, modelRatio, modelPrice) {
		priceData.FreeModel = false
		if minQuota := operation_setting.MinBillableQuota(); minQuota > priceData.QuotaToPreConsume {
			priceData.QuotaToPreConsume = minQuota
		}
	}

	if common.DebugEnabled {
		logger.LogDebug(c, "model_price_helper result: %s", priceData.ToSetting())
	}
	info.PriceData = priceData
	return priceData, nil
}

// ModelPriceHelperPerCall 按次/按量计费的 PriceHelper (MJ、Task)
func ModelPriceHelperPerCall(c *gin.Context, info *relaycommon.RelayInfo) (hosttypes.PriceData, error) {
	originModelName := info.OriginModelName
	if pricingModelName := resolveAutoRoutePricingModel(c, originModelName); pricingModelName != originModelName {
		info.OriginModelName = pricingModelName
		defer func() { info.OriginModelName = originModelName }()
	}
	groupRatioInfo := HandleGroupRatio(c, info)

	modelPrice, success := ratio_setting.GetModelPrice(info.OriginModelName, true)
	usePrice := success
	var modelRatio float64

	if !success {
		defaultPrice, ok := ratio_setting.GetDefaultModelPriceMap()[info.OriginModelName]
		if ok {
			modelPrice = defaultPrice
			usePrice = true
		} else {
			var ratioSuccess bool
			var matchName string
			modelRatio, ratioSuccess, matchName = ratio_setting.GetModelRatio(info.OriginModelName)
			acceptUnsetRatio := operation_setting.SelfUseModeEnabled ||
				info.UserSetting.AcceptUnsetRatioModel
			if !ratioSuccess && !acceptUnsetRatio {
				return hosttypes.PriceData{}, modelPriceNotConfiguredError(matchName, info.UserId)
			}
		}
	}

	var quota int

	if usePrice {
		var err error
		quota, err = common.QuotaFromFloatStrict(modelPrice * common.QuotaPerUnit * groupRatioInfo.GroupRatio)
		if err != nil {
			return hosttypes.PriceData{}, err
		}
	} else {
		// 按量计费：以模型倍率的一半作为预扣额度
		var err error
		quota, err = common.QuotaFromFloatStrict(modelRatio / 2 * common.QuotaPerUnit * groupRatioInfo.GroupRatio)
		if err != nil {
			return hosttypes.PriceData{}, err
		}
		modelPrice = -1
	}
	billingFree := false
	if usePrice {
		billingFree = isFreeUnitPrice(modelPrice * groupRatioInfo.GroupRatio)
	} else {
		completionRatio := ratio_setting.GetCompletionRatio(info.OriginModelName)
		billingFree = isFreeTokenPricing(modelRatio, completionRatio, groupRatioInfo.GroupRatio)
	}
	freeModel := billingFree && !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume
	preConsumedQuota := quota
	if billingFree {
		quota = 0
		if freeModel {
			preConsumedQuota = 0
		}
	}

	// Same zero-price floor as ModelPriceHelper: a model with no configured
	// price is billed the configured minimum rather than nothing. modelPrice is
	// -1 on the ratio branch, so only the ratio field can mark it as unpriced.
	if groupRatioInfo.GroupRatio != 0 && operation_setting.ModelPricedAtZero(usePrice, modelRatio, modelPrice) {
		freeModel = false
		if minQuota := operation_setting.MinBillableQuota(); minQuota > quota {
			quota = minQuota
		}
	}

	priceData := hosttypes.PriceData{
		FreeModel:         freeModel,
		BillingFree:       billingFree,
		ModelPrice:        modelPrice,
		ModelRatio:        modelRatio,
		UsePrice:          usePrice,
		Quota:             quota,
		QuotaToPreConsume: preConsumedQuota,
		GroupRatioInfo:    groupRatioInfo,
	}
	return priceData, nil
}

func HasModelBillingConfig(modelName string) bool {
	if _, ok := ratio_setting.GetModelPrice(modelName, false); ok {
		return true
	}
	if _, ok := ratio_setting.GetModelRatioPrice(modelName); ok {
		return true
	}
	if billing_setting.GetBillingMode(modelName) != billing_setting.BillingModeTieredExpr {
		return false
	}
	expr, ok := billing_setting.GetBillingExpr(modelName)
	return ok && strings.TrimSpace(expr) != ""
}

func HasPriceOrRatioEntry(name string) bool {
	formatted := ratio_setting.FormatMatchingModelName(name)
	if _, ok := ratio_setting.GetModelPrice(formatted, false); ok {
		return true
	}
	if ratio_setting.HasConfiguredModelRatio(formatted) {
		return true
	}
	return billing_setting.GetBillingMode(formatted) == billing_setting.BillingModeTieredExpr
}

func resolveBillingModelName(origin string) string {
	var candidates []string
	if !reasoning.ParseModelModifiers(origin).HasModifiers() {
		candidates = append(candidates, origin)
	}
	candidates = append(candidates, hostreasoning.CanonicalBillingModelNames(origin)...)
	base := hostreasoning.BaseModelName(origin)
	candidates = append(candidates, base)

	seen := make(map[string]struct{}, len(candidates))
	for _, name := range candidates {
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		if HasPriceOrRatioEntry(name) {
			return name
		}
	}
	return base
}

func modelPriceHelperTiered(c *gin.Context, info *relaycommon.RelayInfo, billingModelName string, promptTokens int, groupRatioInfo hosttypes.GroupRatioInfo) (hosttypes.PriceData, error) {
	exprStr, ok := billing_setting.GetBillingExpr(billingModelName)
	if !ok {
		return hosttypes.PriceData{}, fmt.Errorf("model %s is configured as tiered_expr but has no billing expression", billingModelName)
	}
	exprHash := billingexpr.ExprHashString(exprStr)
	if info.RelayFormat == types.RelayFormatOpenAIRealtime && billingexpr.UsesFixedPricingByHash(exprStr, exprHash) {
		return hosttypes.PriceData{}, fmt.Errorf("fixed pricing is not supported for Realtime requests")
	}

	preConsumeMultiplier, err := operation_setting.InputPreConsumeMultiplier()
	if err != nil {
		return hosttypes.PriceData{}, err
	}

	requestInput, err := ResolveIncomingBillingExprRequestInput(c, info)
	if err != nil {
		return hosttypes.PriceData{}, err
	}
	if billingexpr.UsedVarsByHash(exprStr, exprHash)["image_count"] {
		requestInput, err = ResolveImageBillingRequestInput(c, info, requestInput)
		if err != nil {
			return hosttypes.PriceData{}, err
		}
	}

	rawCost, trace, err := billingexpr.RunExprByHashWithRequest(exprStr, exprHash, billingexpr.TokenParams{
		P:   float64(promptTokens),
		C:   0,
		Len: float64(promptTokens),
	}, requestInput)
	if err != nil {
		return hosttypes.PriceData{}, fmt.Errorf("model %s tiered expr run failed: %w", billingModelName, err)
	}

	// Expression coefficients are $/1M tokens prices; convert to quota the same way per-call billing does.
	quotaBeforeGroup := rawCost / 1_000_000 * common.QuotaPerUnit
	// Scale the reservation, preserving the expression's context-length tier
	// and leaving actual settlement and fixed request prices unchanged.
	if trace.BillingUnit != billingexpr.BillingUnitRequest {
		quotaBeforeGroup *= preConsumeMultiplier
	}
	preConsumedQuota, err := billingexpr.QuotaRoundStrict(quotaBeforeGroup * groupRatioInfo.GroupRatio)
	if err != nil {
		return hosttypes.PriceData{}, err
	}

	freePriceCandidates, err := tieredFreePriceCandidates(exprStr, exprHash, requestInput, promptTokens, trace)
	if err != nil {
		return hosttypes.PriceData{}, fmt.Errorf("model %s tiered free-price check failed: %w", billingModelName, err)
	}
	billingFree := groupRatioInfo.GroupRatio == 0
	for _, price := range freePriceCandidates {
		billingFree = billingFree || isFreeUnitPrice(price*groupRatioInfo.GroupRatio)
	}
	freeModel := billingFree && !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume
	if freeModel {
		preConsumedQuota = 0
	}

	snapshot := &billingexpr.BillingSnapshot{
		EstimatedImageCount:       trace.ImageCount,
		BillingMode:               billing_setting.BillingModeTieredExpr,
		ModelName:                 billingModelName,
		ExprString:                exprStr,
		ExprHash:                  exprHash,
		GroupRatio:                groupRatioInfo.GroupRatio,
		EstimatedPromptTokens:     promptTokens,
		EstimatedCompletionTokens: 0,
		PreConsumeMultiplier:      preConsumeMultiplier,
		EstimatedQuotaBeforeGroup: quotaBeforeGroup,
		EstimatedQuotaAfterGroup:  preConsumedQuota,
		EstimatedTier:             trace.MatchedTier,
		EstimatedBillingUnit:      trace.BillingUnit,
		EstimatedFixedPrice:       trace.FixedPrice,
		QuotaPerUnit:              common.QuotaPerUnit,
		ExprVersion:               billingexpr.ExprVersion(exprStr),
		FreePriceCandidates:       freePriceCandidates,
	}
	info.TieredBillingSnapshot = snapshot
	info.BillingRequestInput = &requestInput

	priceData := hosttypes.PriceData{
		FreeModel:         freeModel,
		BillingFree:       billingFree,
		GroupRatioInfo:    groupRatioInfo,
		QuotaToPreConsume: preConsumedQuota,
	}

	logger.LogDebug(c, "model_price_helper_tiered result: model=%s preConsume=%d quotaBeforeGroup=%.2f groupRatio=%.2f tier=%s", billingModelName, preConsumedQuota, quotaBeforeGroup, groupRatioInfo.GroupRatio, trace.MatchedTier)

	info.PriceData = priceData
	return priceData, nil
}

func tieredFreePriceCandidates(exprStr, exprHash string, request billingexpr.RequestInput, promptTokens int, trace billingexpr.TraceResult) ([]float64, error) {
	if trace.BillingUnit == billingexpr.BillingUnitRequest && trace.FixedPrice != nil {
		return []float64{*trace.FixedPrice}, nil
	}

	usedVars := billingexpr.UsedVarsByHash(exprStr, exprHash)
	// image_count is a quantity multiplier. Free-model detection compares the
	// configured unit price, so do not let the requested batch size turn a
	// per-image price into a misleading total request price.
	candidateRequest := request
	if usedVars["image_count"] {
		unitImageCount := 1
		candidateRequest.ImageCount = &unitImageCount
	}
	baseParams := billingexpr.TokenParams{Len: float64(promptTokens)}
	baseCost, _, err := billingexpr.RunExprByHashWithRequest(exprStr, exprHash, baseParams, candidateRequest)
	if err != nil {
		return nil, err
	}
	prices := make([]float64, 0, 3)
	if usedVars["req"] {
		prices = append(prices, baseCost/1_000_000)
	}
	for variable, params := range map[string]billingexpr.TokenParams{
		"p": {P: 1, Len: float64(promptTokens)},
		"c": {C: 1, Len: float64(promptTokens)},
	} {
		if !usedVars[variable] {
			continue
		}
		cost, _, runErr := billingexpr.RunExprByHashWithRequest(exprStr, exprHash, params, candidateRequest)
		if runErr != nil {
			return nil, runErr
		}
		prices = append(prices, cost-baseCost)
	}
	return prices, nil
}
