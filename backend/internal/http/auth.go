package http

import (
	"backend/internal/auth"
	authservice "backend/internal/service/auth"
	"backend/pkg/errcode"
	"backend/pkg/request"
	"backend/pkg/resp"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	deps *Deps
}

func NewAuthHandler(deps *Deps) *AuthHandler {
	return &AuthHandler{deps: deps}
}

func registerAuthRoutes(rg *gin.RouterGroup, deps *Deps) {
	g := rg.Group("/auth")
	h := NewAuthHandler(deps)

	g.POST("/register", h.Register)
	g.POST("/login", h.Login)
	g.POST("/send-code", h.SendCode)
	g.POST("/refresh", h.Refresh)
	g.POST("/logout", h.Logout)
}

// ── 请求/响应 DTO ─────────────────────────────────────────────────────────────

type RegisterReq struct {
	// GrantType: "password" 或 "code"
	GrantType string `json:"grant_type" binding:"required,oneof=password code"`

	// CodeType: "sms" 或 "email"（GrantType=code 时必填）
	CodeType string `json:"code_type" binding:"omitempty,oneof=sms email"`

	Phone    string `json:"phone"    binding:"omitempty,phone_cn"`
	Email    string `json:"email"    binding:"omitempty,email"`
	Code     string `json:"code"     binding:"omitempty,len=6"`
	Password string `json:"password" binding:"omitempty,strong_password"`
	Nickname string `json:"nickname" binding:"omitempty,max=30"`
}

type LoginReq struct {
	// grant_type: password / sms_code / email_code / oauth
	GrantType string `json:"grant_type" binding:"required,oneof=password sms_code email_code oauth"`

	// 密码登录：account（手机号或邮箱）+ password
	Account  string `json:"account"  binding:"omitempty"`
	Phone    string `json:"phone"    binding:"omitempty,phone_cn"`
	Email    string `json:"email"    binding:"omitempty,email"`
	Password string `json:"password" binding:"omitempty"`

	// 验证码登录：phone/email + code
	Code string `json:"code" binding:"omitempty,len=6"`

	// 第三方登录：provider + open_id（开发模式 oauth.dev_mode=true 时可用）
	Provider string `json:"provider" binding:"omitempty,oneof=wechat apple google"`
	OpenID   string `json:"open_id"  binding:"omitempty"`
}

type SendCodeReq struct {
	Type   string `json:"type"   binding:"required,oneof=sms email"`
	Target string `json:"target" binding:"required"`
}

type RefreshReq struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type LogoutReq struct {
	RefreshToken string `json:"refresh_token"`
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// Register 注册（密码注册 或 验证码注册）。
func (h *AuthHandler) Register(c *gin.Context) {
	if h.deps.AuthService == nil {
		resp.Fail(c, errcode.ErrInternal)
		return
	}

	var req RegisterReq
	if !request.Bind(c, &req) {
		return
	}

	result, e := h.deps.AuthService.Register(c.Request.Context(), &authservice.RegisterInput{
		GrantType: req.GrantType,
		CodeType:  req.CodeType,
		Phone:     req.Phone,
		Email:     req.Email,
		Code:      req.Code,
		Password:  req.Password,
		Nickname:  req.Nickname,
	})
	if e != nil {
		resp.Fail(c, e)
		return
	}

	resp.OK(c, result)
}

// Login 统一登录入口，通过 grant_type 路由到对应策略。
func (h *AuthHandler) Login(c *gin.Context) {
	if h.deps.AuthService == nil {
		resp.Fail(c, errcode.ErrInternal)
		return
	}

	var req LoginReq
	if !request.Bind(c, &req) {
		return
	}

	result, e := h.deps.AuthService.Login(c.Request.Context(), &auth.LoginRequest{
		GrantType: req.GrantType,
		Account:   req.Account,
		Phone:     req.Phone,
		Email:     req.Email,
		Password:  req.Password,
		Code:      req.Code,
		Provider:  req.Provider,
		OpenID:    req.OpenID,
	})
	if e != nil {
		resp.Fail(c, e)
		return
	}

	resp.OK(c, result)
}

// SendCode 发送短信/邮件验证码。
func (h *AuthHandler) SendCode(c *gin.Context) {
	if h.deps.AuthService == nil {
		resp.Fail(c, errcode.ErrInternal)
		return
	}

	var req SendCodeReq
	if !request.Bind(c, &req) {
		return
	}

	e := h.deps.AuthService.SendCode(c.Request.Context(), req.Type, req.Target)
	if e != nil {
		resp.Fail(c, e)
		return
	}

	resp.OK(c, nil)
}

// Refresh 使用 Refresh Token 换取新的 Access Token。
func (h *AuthHandler) Refresh(c *gin.Context) {
	if h.deps.AuthService == nil {
		resp.Fail(c, errcode.ErrInternal)
		return
	}

	var req RefreshReq
	if !request.Bind(c, &req) {
		return
	}

	pair, e := h.deps.AuthService.Refresh(c.Request.Context(), req.RefreshToken)
	if e != nil {
		resp.Fail(c, e)
		return
	}

	resp.OK(c, pair)
}

// Logout 登出，将 refresh token 加入黑名单。
func (h *AuthHandler) Logout(c *gin.Context) {
	if h.deps.AuthService == nil {
		resp.OK(c, nil)
		return
	}

	var req LogoutReq
	if !request.Bind(c, &req) {
		return
	}

	_ = h.deps.AuthService.Logout(c.Request.Context(), req.RefreshToken)
	resp.OK(c, nil)
}
