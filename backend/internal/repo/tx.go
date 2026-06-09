package repo

import "context"

// UserOAuthTxRunner 在单事务内执行用户与 OAuth 绑定相关操作。
type UserOAuthTxRunner interface {
	WithUserOAuthTx(ctx context.Context, fn func(UserRepo, OAuthRepo) error) error
}
