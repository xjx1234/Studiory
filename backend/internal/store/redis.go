package store

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type RedisOptions struct {
	PoolSize int
	// ReadTimeout / WriteTimeout 限制单次 Redis 往返，避免网络抖动拖住请求 goroutine。
	// 0 表示沿用 go-redis 默认值（3s）。
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
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
	if opts.ReadTimeout > 0 {
		opt.ReadTimeout = opts.ReadTimeout
	}
	if opts.WriteTimeout > 0 {
		opt.WriteTimeout = opts.WriteTimeout
	}

	client := redis.NewClient(opt)

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}

	if log != nil {
		log.Info("Redis 连接成功",
			zap.Int("pool_size", opt.PoolSize),
			zap.Duration("read_timeout", opt.ReadTimeout),
			zap.Duration("write_timeout", opt.WriteTimeout),
		)
	}

	return client, nil
}
