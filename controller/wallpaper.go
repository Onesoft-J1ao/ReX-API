package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type wallpaperUpdateRequest struct {
	WallpaperUrl string `json:"wallpaper_url"`
}

// wallpaperErrorCode maps a wallpaper failure onto a stable identifier. The
// console turns the code into localized copy, so the paid path never has to
// render a raw server string for the two states users hit most: no changes
// left this month, and not enough balance.
func wallpaperErrorCode(err error) string {
	switch {
	case errors.Is(err, model.ErrWallpaperDisabled):
		return "WALLPAPER_DISABLED"
	case errors.Is(err, model.ErrWallpaperMonthlyLimit):
		return "WALLPAPER_MONTHLY_LIMIT_REACHED"
	case errors.Is(err, model.ErrWallpaperInsufficient):
		return "WALLPAPER_INSUFFICIENT_QUOTA"
	case errors.Is(err, model.ErrWallpaperUnchanged):
		return "WALLPAPER_UNCHANGED"
	case errors.Is(err, model.ErrWallpaperURLRequired),
		errors.Is(err, model.ErrWallpaperURLTooLong),
		errors.Is(err, model.ErrWallpaperURLInvalid):
		return "WALLPAPER_INVALID_URL"
	}
	return ""
}

func wallpaperError(c *gin.Context, err error) {
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"code":    wallpaperErrorCode(err),
		"message": err.Error(),
	})
}

// GetUserWallpaper returns the caller's own wallpaper plus the price and
// remaining changes for the current month.
func GetUserWallpaper(c *gin.Context) {
	status, err := model.GetUserWallpaperStatus(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, status)
}

// UpdateUserWallpaper performs one paid change against the caller's own wallet.
func UpdateUserWallpaper(c *gin.Context) {
	var req wallpaperUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		wallpaperError(c, model.ErrWallpaperURLInvalid)
		return
	}
	wallpaper, err := model.SetUserWallpaper(c.GetInt("id"), req.WallpaperUrl)
	if err != nil {
		wallpaperError(c, err)
		return
	}
	status, err := model.GetUserWallpaperStatus(wallpaper.UserId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, status)
}

// DeleteUserWallpaper clears the caller's own wallpaper. It is free: it cannot
// hand the caller anything they did not already have.
func DeleteUserWallpaper(c *gin.Context) {
	if err := model.DeleteUserWallpaper(c.GetInt("id")); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func AdminGetUserWallpaper(c *gin.Context) {
	userId, err := strconv.Atoi(c.Param("id"))
	if err != nil || userId <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	status, err := model.GetUserWallpaperStatus(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, status)
}

func AdminDeleteUserWallpaper(c *gin.Context) {
	userId, err := strconv.Atoi(c.Param("id"))
	if err != nil || userId <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	if err := model.DeleteUserWallpaper(userId); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
