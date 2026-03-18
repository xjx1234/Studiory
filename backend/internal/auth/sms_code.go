package auth

import "errors"

// SMSCodeStrategy 处理「手机号 + 短信验证码」登录/注册。
type SMSCodeStrategy struct{}

func (s *SMSCodeStrategy) Login(req *LoginRequest) (*LoginResult, error) {
	if req.Phone == "" || req.Code == "" {
		return nil, errors.New("手机号和验证码不能为空")
	}

	// TODO: 从 Redis 中查询该手机号对应的验证码并比对
	// stored, err := redis.Get(ctx, "sms:"+req.Phone)
	// if err != nil || stored != req.Code {
	//     return nil, errors.New("验证码错误或已过期")
	// }
	// redis.Del(ctx, "sms:"+req.Phone)

	// TODO: 查询用户是否存在，不存在则自动注册
	// user, _ := userRepo.FindOrCreateByPhone(req.Phone)

	return issueResult("mock_user_id", "user", "用户昵称", "")
}
