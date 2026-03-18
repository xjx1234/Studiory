package auth

import "errors"

// OAuthStrategy 处理第三方授权登录（微信 / Apple / Google 等）。
// 前端获取到授权平台返回的临时 code 后，由后端换取 openid/unionid 并完成登录。
type OAuthStrategy struct{}

func (s *OAuthStrategy) Login(req *LoginRequest) (*LoginResult, error) {
	if req.Provider == "" || req.Code == "" {
		return nil, errors.New("provider 和 code 不能为空")
	}

	switch req.Provider {
	case "wechat":
		return s.loginWithWechat(req.Code)
	case "apple":
		return s.loginWithApple(req.Code)
	case "google":
		return s.loginWithGoogle(req.Code)
	default:
		return nil, errors.New("不支持的第三方平台: " + req.Provider)
	}
}

// loginWithWechat 微信授权登录。
// 流程：code → 调用微信 API 换取 openid/session_key → 查询或创建用户。
func (s *OAuthStrategy) loginWithWechat(code string) (*LoginResult, error) {
	// TODO: 调用微信接口 https://api.weixin.qq.com/sns/jscode2session
	// resp, err := wechatClient.Code2Session(code)
	// user, _ := userRepo.FindOrCreateByOpenID("wechat", resp.OpenID)
	_ = code
	return issueResult("mock_wx_user_id", "user", "微信用户", "")
}

// loginWithApple Apple ID 登录（Sign in with Apple）。
func (s *OAuthStrategy) loginWithApple(code string) (*LoginResult, error) {
	// TODO: 验证 Apple identity token，解析 sub 字段作为唯一标识
	_ = code
	return issueResult("mock_apple_user_id", "user", "Apple 用户", "")
}

// loginWithGoogle Google OAuth 登录。
func (s *OAuthStrategy) loginWithGoogle(code string) (*LoginResult, error) {
	// TODO: 使用 Google OAuth2 code 换取用户信息
	_ = code
	return issueResult("mock_google_user_id", "user", "Google 用户", "")
}
