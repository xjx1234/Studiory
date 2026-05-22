package auth

// LoginRequest 登录统一入参，通过 GrantType 字段区分具体策略。
// 认证流程由 internal/service/auth 实现，本包仅定义共享 DTO。
type LoginRequest struct {
	GrantType string `json:"grant_type" binding:"required"`

	Account  string `json:"account,omitempty"`
	Password string `json:"password,omitempty"`
	Phone    string `json:"phone,omitempty"`
	Email    string `json:"email,omitempty"`
	Code     string `json:"code,omitempty"`
	Provider string `json:"provider,omitempty"` // wechat / apple / google
	OpenID   string `json:"open_id,omitempty"`
}

// LoginResult 登录成功后的统一返回结构。
type LoginResult struct {
	Tokens *TokenPair `json:"tokens"`
	User   *UserInfo  `json:"user"`
}

// UserInfo 是登录成功后下发给前端的用户信息（最小集）。
type UserInfo struct {
	ID       string `json:"id"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
	Role     string `json:"role"`
}

// GrantType 常量，与前端约定保持一致。
const (
	GrantTypePassword  = "password"
	GrantTypeSMSCode   = "sms_code"
	GrantTypeEmailCode = "email_code"
	GrantTypeOAuth     = "oauth"
)
