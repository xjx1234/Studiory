package auth

import "errors"

// EmailCodeStrategy 处理「邮箱 + 邮件验证码」登录/注册。
type EmailCodeStrategy struct{}

func (s *EmailCodeStrategy) Login(req *LoginRequest) (*LoginResult, error) {
	if req.Email == "" || req.Code == "" {
		return nil, errors.New("邮箱和验证码不能为空")
	}

	// TODO: 从 Redis 中查询该邮箱对应的验证码并比对
	// stored, err := redis.Get(ctx, "email:"+req.Email)
	// if err != nil || stored != req.Code {
	//     return nil, errors.New("验证码错误或已过期")
	// }
	// redis.Del(ctx, "email:"+req.Email)

	// TODO: 查询用户是否存在，不存在则自动注册
	// user, _ := userRepo.FindOrCreateByEmail(req.Email)

	return issueResult("mock_user_id", "user", "用户昵称", "")
}
