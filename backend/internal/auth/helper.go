package auth

// issueResult 是各策略的公共收尾函数：颁发 JWT 并组装 LoginResult。
func issueResult(userID, role, nickname, avatar string) (*LoginResult, error) {
	pair, err := IssueTokenPair(userID, role)
	if err != nil {
		return nil, err
	}

	return &LoginResult{
		Tokens: pair,
		User: &UserInfo{
			ID:       userID,
			Nickname: nickname,
			Avatar:   avatar,
			Role:     role,
		},
	}, nil
}
