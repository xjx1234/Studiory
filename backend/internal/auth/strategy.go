package auth

// LoginRequest 登录统一入参，通过 GrantType 字段区分具体策略。
type LoginRequest struct {
	GrantType string `json:"grant_type" binding:"required"`

	// 账号+密码
	Account  string `json:"account,omitempty"`
	Password string `json:"password,omitempty"`

	// 手机验证码
	Phone string `json:"phone,omitempty"`

	// 邮箱验证码
	Email string `json:"email,omitempty"`

	// 手机/邮箱验证码公用字段
	Code string `json:"code,omitempty"`

	// 第三方登录
	Provider string `json:"provider,omitempty"` // wechat / apple / google
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
	Role     string `json:"role"` // user / admin
}

// Strategy 定义统一的登录策略接口，每种登录方式各自实现。
type Strategy interface {
	Login(req *LoginRequest) (*LoginResult, error)
}

// GrantType 常量，与前端约定保持一致。
const (
	GrantTypePassword  = "password"
	GrantTypeSMSCode   = "sms_code"
	GrantTypeEmailCode = "email_code"
	GrantTypeOAuth     = "oauth"
)

// Resolver 根据 grant_type 选择对应的登录策略。
func Resolver(grantType string) Strategy {
	switch grantType {
	case GrantTypePassword:
		return &PasswordStrategy{}
	case GrantTypeSMSCode:
		return &SMSCodeStrategy{}
	case GrantTypeEmailCode:
		return &EmailCodeStrategy{}
	case GrantTypeOAuth:
		return &OAuthStrategy{}
	default:
		return nil
	}
}
