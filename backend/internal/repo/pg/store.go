package pg

import (
	"context"

	"backend/internal/repo"
	sqlcgen "backend/internal/repo/sqlc/gen"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store 是 Postgres 的 repo 实现入口，负责持有连接池与 sqlc Queries。
//
// 说明：
// - 上层（usecase）应依赖 internal/repo 的接口，不直接依赖 sqlcgen。
// - 需要事务时使用 WithinTx/WithTx 获取 tx 绑定的 Store。
type Store struct {
	pool *pgxpool.Pool
	q    *sqlcgen.Queries
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{
		pool: pool,
		q:    sqlcgen.New(pool),
	}
}

func (s *Store) Users() repo.UserRepo {
	return &userRepo{q: s.q}
}

func (s *Store) OAuth() repo.OAuthRepo {
	return &oauthRepo{q: s.q}
}

func (s *Store) Todos() repo.TodoRepo {
	return &todoRepo{q: s.q}
}

// WithUserOAuthTx 在事务内执行 fn，成功 commit，失败 rollback。
func (s *Store) WithUserOAuthTx(ctx context.Context, fn func(repo.UserRepo, repo.OAuthRepo) error) error {
	return s.WithinTx(ctx, func(txStore *Store) error {
		return fn(txStore.Users(), txStore.OAuth())
	})
}

func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}

// WithinTx 在一个事务内执行 fn，成功则 commit，失败则 rollback。
func (s *Store) WithinTx(ctx context.Context, fn func(txStore *Store) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	txStore := s.WithTx(tx)
	if err := fn(txStore); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// WithTx 返回绑定到指定事务的 Store（仅对 sqlc Queries 生效）。
func (s *Store) WithTx(tx pgx.Tx) *Store {
	return &Store{
		pool: s.pool,
		q:    s.q.WithTx(tx),
	}
}
