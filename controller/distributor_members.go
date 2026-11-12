package controller

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

// Distributor membership endpoints.
//
// The existing distributor page edits the MULTIPLIERS a group pays. Granting the
// identity was previously only possible by editing a user's group field by hand,
// which meant looking the user up in the user table first. These two endpoints
// make the page able to do it: search by id / username / email, then assign.

type distributorUserRow struct {
	Id       int    `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Group    string `json:"group"`
	Quota    int    `json:"quota"`
	Status   int    `json:"status"`
}

// GetDistributorMembers returns the distributor groups plus the users currently
// sitting in any of them, so the page can list membership without a second call.
func GetDistributorMembers(c *gin.Context) {
	overrides := ratio_setting.GetGroupGroupRatioCopy()
	groups := make([]string, 0, len(overrides))
	for name := range overrides {
		groups = append(groups, name)
	}
	sort.Strings(groups)
	if len(groups) == 0 {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"groups": []string{}, "members": []distributorUserRow{}}})
		return
	}

	var users []model.User
	// Select with the struct field name rather than a quoted column: the column
	// is a reserved word, and model.commonGroupCol is private to that package.
	if err := model.DB.Select("Id", "Username", "Email", "Group", "Quota", "Status").
		Where("`group` IN ?", groups).
		Order("id asc").
		Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	members := make([]distributorUserRow, 0, len(users))
	for _, u := range users {
		members = append(members, distributorUserRow{
			Id: u.Id, Username: u.Username, Email: u.Email,
			Group: u.Group, Quota: u.Quota, Status: u.Status,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    gin.H{"groups": groups, "members": members},
	})
}

// SearchDistributorCandidates finds users to promote, by id, username, or email.
//
// Bounded to 20 rows: this is a picker, not a user browser. An empty query lists
// the most recent users so the operator can pick without typing.
func SearchDistributorCandidates(c *gin.Context) {
	query := strings.TrimSpace(c.Query("q"))
	limit := 20

	var users []model.User
	db := model.DB.Select("Id", "Username", "Email", "Group", "Quota", "Status")
	if query == "" {
		db = db.Order("id desc").Limit(limit)
	} else if id, err := strconv.Atoi(query); err == nil {
		db = db.Where("id = ?", id).Limit(limit)
	} else {
		like := "%" + query + "%"
		db = db.Where("username LIKE ? OR email LIKE ?", like, like).
			Order("id asc").Limit(limit)
	}
	if err := db.Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}

	rows := make([]distributorUserRow, 0, len(users))
	for _, u := range users {
		rows = append(rows, distributorUserRow{
			Id: u.Id, Username: u.Username, Email: u.Email,
			Group: u.Group, Quota: u.Quota, Status: u.Status,
		})
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows})
}

type distributorAssignRequest struct {
	UserId    int    `json:"user_id"`
	UserGroup string `json:"user_group"`
}

// AssignDistributor moves one user into a distributor group, or back out.
//
// An empty user_group restores the platform default. The target group must
// already exist as a distributor (i.e. carry overrides): assigning someone to a
// group with no overrides would silently give them the normal price while
// looking like a discount.
func AssignDistributor(c *gin.Context) {
	var req distributorAssignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request body"})
		return
	}
	if req.UserId <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "user_id is required"})
		return
	}

	user, err := model.GetUserById(req.UserId, false)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "user not found"})
		return
	}

	target := strings.TrimSpace(req.UserGroup)
	if target == "" {
		target = "default"
	} else {
		overrides := ratio_setting.GetGroupGroupRatioCopy()
		if _, ok := overrides[target]; !ok {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "not a distributor group: " + target,
			})
			return
		}
		if distributorReserved(target) {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": target + " is a built-in group and cannot be assigned",
			})
			return
		}
	}

	previous := user.Group
	if err := model.DB.Model(&model.User{}).Where("id = ?", req.UserId).
		Update("group", target).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	if err := model.RefreshUserGroupCache(req.UserId); err != nil {
		common.SysError("failed to refresh group cache after distributor assignment: " + err.Error())
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"user_id":  req.UserId,
			"previous": previous,
			"group":    target,
		},
	})
}
