package auth

import "errors"

// PasswordStrategy 处理「账号 + 密码」登录。
// 账号字段可以是手机号、邮箱或用户名，具体由数据库查询逻辑区分。
type PasswordStrategy struct{}

func (s *PasswordStrategy) Login(req *LoginRequest) (*LoginResult, error) {
	if req.Account == "" || req.Password == "" {
		return nil, errors.New("账号和密码不能为空")
	}

	// TODO: 查询数据库，验证账号是否存在，校验密码 hash
	// user, err := userRepo.FindByAccount(req.Account)
	// if err != nil || !checkPasswordHash(req.Password, user.PasswordHash) {
	//     return nil, errors.New("账号或密码错误")
	// }

	// TODO: 替换为真实用户数据
	return issueResult("mock_user_id", "user", "用户昵称", "")
}
