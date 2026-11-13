package http

import (
	"errors"

	"backend/internal/auth"
	authservice "backend/internal/service/auth"
	"backend/pkg/errcode"
	"backend/pkg/request"
	"backend/pkg/resp"
	"backend/pkg/strutil"

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

	// 第三方登录：provider + token（生产）或 provider + open_id（oauth.dev_mode=true）
	Provider    string `json:"provider" binding:"omitempty,oneof=wechat apple google"`
	OpenID      string `json:"open_id" binding:"omitempty"`
	AccessToken string `json:"access_token" binding:"omitempty"`
	IDToken     string `json:"id_token" binding:"omitempty"`
}

type SendCodeReq struct {
	Type   string `json:"type"   binding:"required,oneof=sms email"`
	Target string `json:"target" binding:"required"`
}

// Validate 按 type 校验 target 格式，防止 SMTP 头注入。
func (r *SendCodeReq) Validate() error {
	switch r.Type {
	case "sms":
		if !strutil.LooksLikePhone(r.Target) {
			return errInvalidPhoneTarget
		}
	case "email":
		if !strutil.LooksLikeEmail(r.Target) {
			return errInvalidEmailTarget
		}
	}
	return nil
}

var (
	errInvalidPhoneTarget = errors.New("invalid phone target")
	errInvalidEmailTarget = errors.New("invalid email target")
)

// Validate 补齐绑定标签表达不了的跨字段规则（密码注册）。
//
//   - 至少要有一个标识：两者皆空会注册出「无法登录、无法找回」的僵尸账号；
//   - 密码注册只允许一个标识：只校验其中一个的话，另一个就是未验证的悬空地址；
//   - code_type 若显式传入，必须与实际填写的标识一致，否则等于对着没填的渠道验码。
//
// 安全兜底在 service 层（authservice.registrationTarget 同样会拒掉），这里只是更早、更便宜的 400。
func (r *RegisterReq) Validate() error {
	if r.GrantType != "password" {
		return nil
	}

	switch {
	case r.Phone == "" && r.Email == "":
		return errRegisterNeedsIdentifier
	case r.Phone != "" && r.Email != "":
		return errRegisterSingleIdentifier
	}

	wantType := "sms"
	if r.Email != "" {
		wantType = "email"
	}
	if r.CodeType != "" && r.CodeType != wantType {
		return errRegisterCodeTypeMismatch
	}
	return nil
}

var (
	errRegisterNeedsIdentifier  = errors.New("password register requires phone or email")
	errRegisterSingleIdentifier = errors.New("password register accepts either phone or email")
	errRegisterCodeTypeMismatch = errors.New("code_type does not match the submitted identifier")
)

type RefreshReq struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type LogoutReq struct {
	RefreshToken string `json:"refresh_token"`
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// Register 注册（密码注册 或 验证码注册）。
func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterReq
	if !request.Bind(c, &req) {
		return
	}

	// 跨字段规则（标识必填/唯一、code_type 一致性）先在这里挡掉，service 层还有同样的兜底。
	if err := req.Validate(); err != nil {
		resp.Fail(c, errcode.ErrBadRequest)
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
	var req LoginReq
	if !request.Bind(c, &req) {
		return
	}

	result, e := h.deps.AuthService.Login(c.Request.Context(), &auth.LoginRequest{
		GrantType:   req.GrantType,
		Account:     req.Account,
		Phone:       req.Phone,
		Email:       req.Email,
		Password:    req.Password,
		Code:        req.Code,
		Provider:    req.Provider,
		OpenID:      req.OpenID,
		AccessToken: req.AccessToken,
		IDToken:     req.IDToken,
	})
	if e != nil {
		resp.Fail(c, e)
		return
	}

	resp.OK(c, result)
}

// SendCode 发送短信/邮件验证码。
func (h *AuthHandler) SendCode(c *gin.Context) {
	var req SendCodeReq
	if !request.Bind(c, &req) {
		return
	}

	// 按 type 校验 target 格式，防止 SMTP 头注入。
	if err := req.Validate(); err != nil {
		resp.Fail(c, errcode.ErrBadRequest)
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
	var req LogoutReq
	if !request.Bind(c, &req) {
		return
	}

	if e := h.deps.AuthService.Logout(c.Request.Context(), req.RefreshToken); e != nil {
		resp.Fail(c, e)
		return
	}
	resp.OK(c, nil)
}
