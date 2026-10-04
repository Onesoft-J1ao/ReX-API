package controller

import (
	"maps"
	"slices"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

func filterPricingByUsableGroups(pricing []model.Pricing, usableGroup map[string]string) []model.Pricing {
	if len(pricing) == 0 {
		return pricing
	}
	if len(usableGroup) == 0 {
		return []model.Pricing{}
	}

	filtered := make([]model.Pricing, 0, len(pricing))
	for _, item := range pricing {
		if common.StringsContains(item.EnableGroup, "all") {
			filtered = append(filtered, item)
			continue
		}
		for _, group := range item.EnableGroup {
			if _, ok := usableGroup[group]; ok {
				filtered = append(filtered, item)
				break
			}
		}
	}
	return filtered
}

func GetPricing(c *gin.Context) {
	pricing := model.GetPricing()
	userId, exists := c.Get("id")
	usableGroup := map[string]string{}
	groupRatio := map[string]float64{}
	maps.Copy(groupRatio, ratio_setting.GetGroupRatioCopy())
	// Which pricing source each group applies to each model. Without it the
	// square can only multiply one global price by a group ratio, so a model
	// billed per call in one group and per token in another shows the same
	// figure in both.
	groupBillingSource := filterGroupBillingSourceByGroups(
		billing_setting.GetGroupPricingSourceCopy(),
		ratio_setting.GetGroupRatioCopy(),
	)
	var group string
	if exists {
		user, err := model.GetUserCache(userId.(int))
		if err == nil {
			group = user.Group
			for g := range groupRatio {
				ratio, ok := ratio_setting.GetGroupGroupRatio(group, g)
				if ok {
					groupRatio[g] = ratio
				}
			}
		}
	}

	userID := 0
	if exists {
		userID, _ = userId.(int)
	}
	usableGroup = service.GetUserUsableGroupsForUser(userID, group)
	visibleGroups := console_setting.GetModelSquareVisibleGroups()
	visibleDescriptions := console_setting.GetModelSquareVisibleGroupDescriptions()

	// A group whose channels are all disabled cannot serve anything, so drop it
	// from both the picker and the model list rather than advertising a group
	// that will always fail. Fails open on a query error: an empty model square
	// is worse than briefly showing a disabled group.
	if console_setting.GetConsoleSetting().HideModelsWithoutEnabledChannel {
		served, err := model.GetGroupsWithEnabledChannel()
		if err != nil {
			common.SysError("failed to load groups with enabled channels: " + err.Error())
		} else {
			for name := range usableGroup {
				if _, ok := served[name]; !ok {
					delete(usableGroup, name)
				}
			}
			// The visible list is what the picker renders, so a group pruned
			// from usable_group must not survive there either.
			visibleGroups = slices.DeleteFunc(visibleGroups, func(name string) bool {
				_, ok := served[name]
				return !ok
			})
			for name := range visibleDescriptions {
				if _, ok := served[name]; !ok {
					delete(visibleDescriptions, name)
				}
			}
		}
	}

	pricingGroups := make(map[string]string, len(usableGroup)+len(visibleGroups))
	for groupName, description := range usableGroup {
		pricingGroups[groupName] = description
	}
	for _, groupName := range visibleGroups {
		if _, exists := pricingGroups[groupName]; !exists {
			if description, ok := visibleDescriptions[groupName]; ok && description != "" {
				pricingGroups[groupName] = description
			} else {
				pricingGroups[groupName] = setting.GetUsableGroupDescription(groupName)
			}
		}
	}
	pricing = filterPricingByUsableGroups(pricing, pricingGroups)
	// check groupRatio contains usableGroup
	for group := range ratio_setting.GetGroupRatioCopy() {
		if _, ok := pricingGroups[group]; !ok {
			delete(groupRatio, group)
			delete(groupBillingSource, group)
		}
	}

	c.JSON(200, gin.H{
		"success":                         true,
		"data":                            pricing,
		"vendors":                         model.GetVendors(),
		"group_ratio":                     groupRatio,
		"group_billing_source":            groupBillingSource,
		"usable_group":                    usableGroup,
		"model_square_groups":             visibleGroups,
		"model_square_group_descriptions": visibleDescriptions,
		"supported_endpoint":              model.GetSupportedEndpointMap(),
		"auto_groups":                     service.GetUserAutoGroupForUser(userID, group),
		"pricing_version":                 "a42d372ccf0b5dd13ecf71203521f9d2",
	})
}

// filterGroupBillingSourceByGroups drops overrides for groups that have no
// group ratio. Such an override can never apply — the request would be priced
// against a group that does not exist — so publishing it would offer the viewer
// a price no request would ever get.
func filterGroupBillingSourceByGroups(
	sources map[string]map[string]string,
	groupRatios map[string]float64,
) map[string]map[string]string {
	if len(sources) == 0 {
		return nil
	}
	filtered := make(map[string]map[string]string, len(sources))
	for group, byModel := range sources {
		if _, active := groupRatios[group]; !active && group != "auto" {
			continue
		}
		if len(byModel) == 0 {
			continue
		}
		filtered[group] = byModel
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func ResetModelRatio(c *gin.Context) {
	defaultStr := ratio_setting.DefaultModelRatio2JSONString()
	err := model.UpdateOption("ModelRatio", defaultStr)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	err = ratio_setting.UpdateModelRatioByJSONString(defaultStr)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"success": true,
		"message": "重置模型倍率成功",
	})
}
