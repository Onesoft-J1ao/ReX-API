package controller

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

// Distributor groups
//
// A distributor is an ordinary user group whose members pay a different price
// per group. The whole mechanism already exists: GetUserGroupRatio consults
// GroupGroupRatio[userGroup][group] before falling back to the group's own
// ratio. These handlers are the admin surface for that table, so an operator
// can set it without hand-editing JSON.

const (
	// A ratio far above any real price is a typo, not a discount.
	maxDistributorRatio = 1000.0
	// Groups that ship with the product and back free/default traffic must not
	// silently become a distributor, or every new user inherits the override.
	distributorReservedGroups = "default,free1,free2,vip,svip"
)

func distributorReserved(group string) bool {
	for _, name := range strings.Split(distributorReservedGroups, ",") {
		if strings.EqualFold(strings.TrimSpace(name), group) {
			return true
		}
	}
	return false
}

// GetDistributors returns the current overrides plus the inputs the editor
// needs: every group that can actually serve traffic, and the users currently
// sitting in a distributor group.
//
// The group list comes from the ability table, not from the ratio table: a
// ratio entry with no enabled channel is a leftover, and offering it in the
// editor is how an empty "distributor" row ended up on the model square.
func GetDistributors(c *gin.Context) {
	overrides := ratio_setting.GetGroupGroupRatioCopy()
	served, err := model.GetGroupsWithEnabledChannel()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	groups := make([]string, 0, len(served))
	for name := range served {
		groups = append(groups, name)
	}
	sort.Strings(groups)

	distributorGroups := make([]string, 0, len(overrides))
	for name := range overrides {
		distributorGroups = append(distributorGroups, name)
	}
	sort.Strings(distributorGroups)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"overrides":    overrides,
			"groups":       groups,
			"distributors": distributorGroups,
		},
	})
}

type distributorRequest struct {
	UserGroup string             `json:"user_group"`
	Ratios    map[string]float64 `json:"ratios"`
}

// SaveDistributor replaces one distributor group's overrides.
//
// The whole map is replaced rather than merged so removing an entry is just
// omitting it; a partial merge would make "stop discounting this group"
// impossible without a second endpoint.
func SaveDistributor(c *gin.Context) {
	var req distributorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request body"})
		return
	}
	userGroup := strings.TrimSpace(req.UserGroup)
	if userGroup == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "user_group is required"})
		return
	}
	if distributorReserved(userGroup) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": userGroup + " is a built-in group and cannot be a distributor",
		})
		return
	}

	known := ratio_setting.GetGroupRatioCopy()
	// Only groups with an enabled channel can actually serve a request. An entry
	// for a group whose channel was disabled afterwards is dead config: it
	// cannot price anything, and the editor never renders it, so a save would
	// resubmit text the operator cannot see or remove. Drop such entries instead
	// of failing the whole request; a typo in the group name is still rejected.
	served, err := model.GetGroupsWithEnabledChannel()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	cleaned := make(map[string]float64, len(req.Ratios))
	for group, ratio := range req.Ratios {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		if _, ok := known[group]; !ok {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "unknown group: " + group,
			})
			return
		}
		if _, ok := served[group]; !ok {
			// A stale entry from a group whose channel was disabled later. It
			// cannot price anything, and the editor never rendered it, so
			// rejecting the whole save would trap the operator: every save
			// resubmits the text they cannot see or remove. Drop it instead.
			continue
		}
		if ratio <= 0 || ratio > maxDistributorRatio {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "ratio for " + group + " must be greater than 0 and at most 1000",
			})
			return
		}
		cleaned[group] = ratio
	}
	if len(cleaned) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "at least one group ratio is required",
		})
		return
	}

	overrides := ratio_setting.GetGroupGroupRatioCopy()
	overrides[userGroup] = cleaned
	encoded, err := json.Marshal(overrides)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	if err := model.UpdateOption("group_ratio_setting.group_group_ratio", string(encoded)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	if err := ratio_setting.UpdateGroupGroupRatioByJSONString(string(encoded)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": cleaned})
}

// DeleteDistributor removes a distributor group entirely, returning its members
// to normal group pricing.
func DeleteDistributor(c *gin.Context) {
	userGroup := strings.TrimSpace(c.Query("user_group"))
	if userGroup == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "user_group is required"})
		return
	}
	overrides := ratio_setting.GetGroupGroupRatioCopy()
	if _, ok := overrides[userGroup]; !ok {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "distributor not found"})
		return
	}
	delete(overrides, userGroup)
	encoded, err := json.Marshal(overrides)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	if err := model.UpdateOption("group_ratio_setting.group_group_ratio", string(encoded)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	if err := ratio_setting.UpdateGroupGroupRatioByJSONString(string(encoded)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}
