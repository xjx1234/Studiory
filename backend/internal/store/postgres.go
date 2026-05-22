package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

type PostgresOptions struct {
	MaxConns    int
	MinConns    int
	MaxConnIdle time.Duration
	MaxConnLife time.Duration
}

// NewPostgres 创建 PostgreSQL 连接池。
// 调用方需在程序退出时调用 pool.Close()。
func NewPostgres(ctx context.Context, databaseURL string, opts PostgresOptions, log *zap.Logger) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	if opts.MaxConns > 0 {
		cfg.MaxConns = int32(opts.MaxConns)
	}
	if opts.MinConns > 0 {
		cfg.MinConns = int32(opts.MinConns)
	}
	if opts.MaxConnIdle > 0 {
		cfg.MaxConnIdleTime = opts.MaxConnIdle
	}
	if opts.MaxConnLife > 0 {
		cfg.MaxConnLifetime = opts.MaxConnLife
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
		log.Info("PostgreSQL 连接成功",
			zap.Int32("max_conns", cfg.MaxConns),
			zap.Int32("min_conns", cfg.MinConns),
		)
	}

	return pool, nil
}
