package http

import (
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
	svc userservice.Service
}

func NewUserProfileHandler(svc userservice.Service) *UserProfileHandler {
	return &UserProfileHandler{svc: svc}
}

type updateProfileReq struct {
	Nickname string `json:"nickname" binding:"omitempty,max=30"`
	Avatar   string `json:"avatar"   binding:"omitempty,max=500"`
}

// GetProfile 获取当前登录用户的个人资料。
func (h *UserProfileHandler) GetProfile(c *gin.Context) {
	userID, ok := mustUserID(c)
	if !ok {
		return
	}

	profile, e := h.svc.GetProfile(c.Request.Context(), userID)
	if e != nil {
		resp.Fail(c, e)
		return
	}

	resp.OK(c, profile)
}

// UpdateProfile PATCH /api/v1/user/profile
func (h *UserProfileHandler) UpdateProfile(c *gin.Context) {
	userID, ok := mustUserID(c)
	if !ok {
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

	profile, e := h.svc.UpdateProfile(c.Request.Context(), userID, &userservice.UpdateProfileInput{
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
	userID, ok := mustUserID(c)
	if !ok {
		return
	}

	var req changePasswordReq
	if !request.Bind(c, &req) {
		return
	}

	revoked, e := h.svc.ChangePassword(c.Request.Context(), userID, &userservice.ChangePasswordInput{
		OldPassword: req.OldPassword,
		NewPassword: req.NewPassword,
	})
	if e != nil {
		resp.Fail(c, e)
		return
	}

	// 密码已修改；sessions_revoked=false 表示旧会话/令牌的吊销未完成（Redis 异常），
	// 前端应提示用户「其他设备可能仍在线」并建议稍后重试或联系运维。
	resp.OK(c, gin.H{"sessions_revoked": revoked})
}
