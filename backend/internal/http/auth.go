package http

import (
	"backend/internal/auth"
	"backend/pkg/errcode"
	"backend/pkg/request"
	"backend/pkg/resp"

	"github.com/gin-gonic/gin"
)

func registerAuthRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/auth")

	g.POST("/login", loginHandler)
	g.POST("/send-code", sendCodeHandler)
	g.POST("/refresh", refreshTokenHandler)
	g.POST("/logout", logoutHandler)
}

// loginHandler 统一登录入口，通过 grant_type 路由到对应策略。
func loginHandler(c *gin.Context) {
	var req auth.LoginRequest
	if !request.Bind(c, &req) {
		return
	}

	strategy := auth.Resolver(req.GrantType)
	if strategy == nil {
		resp.Fail(c, errcode.ErrUnsupportedGrant)
		return
	}

	result, err := strategy.Login(&req)
	if err != nil {
		resp.FailWithMessage(c, errcode.ErrInvalidCredentials, err.Error())
		return
	}

	resp.OK(c, result)
}

// sendCodeHandler 发送短信/邮件验证码。
func sendCodeHandler(c *gin.Context) {
	var req struct {
		Type   string `json:"type" binding:"required,oneof=sms email"`
		Target string `json:"target" binding:"required"`
	}

	if !request.Bind(c, &req) {
		return
	}

	// TODO: 生成验证码 → 写入 Redis（TTL 5min）→ 调用短信/邮件服务
	resp.OK(c, nil)
}

// refreshTokenHandler 使用 Refresh Token 换取新的 Access Token。
func refreshTokenHandler(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}

	if !request.Bind(c, &req) {
		return
	}

	claims, err := auth.ParseRefreshToken(req.RefreshToken)
	if err != nil {
		resp.Fail(c, errcode.ErrInvalidToken)
		return
	}

	pair, err := auth.IssueTokenPair(claims.UserID, claims.Role)
	if err != nil {
		resp.Fail(c, errcode.ErrInternal)
		return
	}

	resp.OK(c, pair)
}

// logoutHandler 登出（当前为占位，真实场景将 refresh token 加入 Redis 黑名单）。
func logoutHandler(c *gin.Context) {
	// TODO: 将 refresh_token 的 jti 加入 Redis 黑名单
	resp.OK(c, nil)
}
