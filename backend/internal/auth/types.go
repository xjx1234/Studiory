package auth

// LoginRequest 登录统一入参，通过 GrantType 字段区分具体策略。
// 认证流程由 internal/service/auth 实现，本包只放共享 DTO 与少量身份策略常量
// （如 PasswordHashCost），不依赖任何 service/repo。
type LoginRequest struct {
	GrantType string `json:"grant_type" binding:"required"`

	Account     string `json:"account,omitempty"`
	Password    string `json:"password,omitempty"`
	Phone       string `json:"phone,omitempty"`
	Email       string `json:"email,omitempty"`
	Code        string `json:"code,omitempty"`
	Provider    string `json:"provider,omitempty"` // wechat / apple / google
	OpenID      string `json:"open_id,omitempty"`  // 仅 oauth.dev_mode=true 时可直接传入
	AccessToken string `json:"access_token,omitempty"`
	IDToken     string `json:"id_token,omitempty"`
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

// PasswordHashCost 是 bcrypt 的目标 cost（N-10）。
//
// 取 12：相对长期沿用的 10 提升约 4 倍计算量，在通用 2 vCPU 云主机上单次约
// 200–400ms，仍在 5s 请求预算内；过高（>=14）会明显放大登录延迟并成为限流外的
// 资源放大点。注册/改密直接用它，登录时发现库内 hash 低于它则就地升级
// （见 authservice.upgradePasswordHash）。
const PasswordHashCost = 12
