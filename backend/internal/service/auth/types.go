package authservice

import "backend/pkg/strutil"

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

// Canonicalize 把输入里的手机号/邮箱收敛成项目内唯一的标识形式（幂等）。
//
// 必须在查重、验码、写库之前完成：注册存 "Alice@x.com"、登录查 "alice@x.com"
// 在 PG 上就是两个账号，而 MySQL 的 utf8mb4_unicode_ci 又只有一个——
// 换驱动即换语义。写库形式与验证码键、锁定键必须由同一次归一化产生。
func (i *RegisterInput) Canonicalize() {
	if i == nil {
		return
	}
	i.Phone = strutil.NormalizePhone(i.Phone)
	i.Email = strutil.NormalizeEmail(i.Email)
}
