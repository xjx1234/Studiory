package authservice

// MockVerificationCode 是开发阶段固定的验证码，生产环境接入真实短信/邮件服务后替换。
const MockVerificationCode = "123456"

// 支持的 OAuth 提供商（与 config oauth.providers 默认值保持一致）。
var DefaultOAuthProviders = []string{"wechat", "apple", "google"}

// RegisterInput 注册请求输入。
// GrantType 决定走密码注册还是验证码注册。
type RegisterInput struct {
	// GrantType: "password" 或 "code"
	GrantType string

	// CodeType: "sms" 或 "email"（GrantType=code 时必填）
	CodeType string

	// Phone 手机号（sms 验证码注册/密码注册时填）
	Phone string

	// Email 邮箱（email 验证码注册/密码注册时填）
	Email string

	// Code 验证码（GrantType=code 时必填）
	Code string

	// Password 密码（GrantType=password 时必填）
	Password string

	// Nickname 昵称（可选，默认为手机号后4位或邮箱前缀）
	Nickname string
}
