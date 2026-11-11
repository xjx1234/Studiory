package repo

import "context"

// Storage groups database-specific repositories behind the same application contract.
type Storage interface {
	UserOAuthTxRunner
	Users() UserRepo
	OAuth() OAuthRepo
	Todos() TodoRepo
	Ping(context.Context) error
	Close()
}
