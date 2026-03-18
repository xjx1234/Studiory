package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// NewPostgres 创建 PostgreSQL 连接池。
// 调用方需在程序退出时调用 pool.Close()。
func NewPostgres(ctx context.Context, databaseURL string, log *zap.Logger) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	if log != nil {
		log.Info("PostgreSQL 连接成功")
	}

	return pool, nil
}
