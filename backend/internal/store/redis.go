package store

import (
	"context"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type RedisOptions struct {
	PoolSize int
}

// NewRedis 创建 Redis 客户端。
// 调用方需在程序退出时调用 client.Close()。
func NewRedis(ctx context.Context, redisURL string, opts RedisOptions, log *zap.Logger) (redis.UniversalClient, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	if opts.PoolSize > 0 {
		opt.PoolSize = opts.PoolSize
	}

	client := redis.NewClient(opt)

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}

	if log != nil {
		log.Info("Redis 连接成功", zap.Int("pool_size", opt.PoolSize))
	}

	return client, nil
}
