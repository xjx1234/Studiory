package http

import (
	"backend/internal/http/middleware"
	userservice "backend/internal/service/user"
	"backend/pkg/errcode"
	"backend/pkg/request"
	"backend/pkg/resp"

	"github.com/gin-gonic/gin"
)

type changePasswordReq struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,strong_password"`
}

type UserProfileHandler struct {
	deps *Deps
}

func NewUserProfileHandler(deps *Deps) *UserProfileHandler {
	return &UserProfileHandler{deps: deps}
}

func registerUserProfileRoutes(rg *gin.RouterGroup, deps *Deps) {
	h := NewUserProfileHandler(deps)

	rg.GET("/profile", h.GetProfile)
	rg.PATCH("/profile", h.UpdateProfile)
	rg.PATCH("/password", h.ChangePassword)
}

type updateProfileReq struct {
	Nickname string `json:"nickname" binding:"omitempty,max=30"`
	Avatar   string `json:"avatar"   binding:"omitempty,max=500"`
}

// GetProfile 获取当前登录用户的个人资料。
func (h *UserProfileHandler) GetProfile(c *gin.Context) {
	userID, exists := c.Get(middleware.ContextKeyUserID)
	if !exists {
		resp.Fail(c, errcode.ErrUnauthorized)
		return
	}

	profile, e := h.deps.UserService.GetProfile(c.Request.Context(), userID.(string))
	if e != nil {
		resp.Fail(c, e)
		return
	}

	resp.OK(c, profile)
}

// UpdateProfile PATCH /api/v1/user/profile
func (h *UserProfileHandler) UpdateProfile(c *gin.Context) {
	userID, exists := c.Get(middleware.ContextKeyUserID)
	if !exists {
		resp.Fail(c, errcode.ErrUnauthorized)
		return
	}

	var req updateProfileReq
	if !request.Bind(c, &req) {
		return
	}
	if req.Nickname == "" && req.Avatar == "" {
		resp.Fail(c, errcode.ErrValidation)
		return
	}

	profile, e := h.deps.UserService.UpdateProfile(c.Request.Context(), userID.(string), &userservice.UpdateProfileInput{
		Nickname: req.Nickname,
		Avatar:   req.Avatar,
	})
	if e != nil {
		resp.Fail(c, e)
		return
	}

	resp.OK(c, profile)
}

// ChangePassword PATCH /api/v1/user/password
func (h *UserProfileHandler) ChangePassword(c *gin.Context) {
	userID, exists := c.Get(middleware.ContextKeyUserID)
	if !exists {
		resp.Fail(c, errcode.ErrUnauthorized)
		return
	}

	var req changePasswordReq
	if !request.Bind(c, &req) {
		return
	}

	e := h.deps.UserService.ChangePassword(c.Request.Context(), userID.(string), &userservice.ChangePasswordInput{
		OldPassword: req.OldPassword,
		NewPassword: req.NewPassword,
	})
	if e != nil {
		resp.Fail(c, e)
		return
	}

	resp.OK(c, nil)
}
