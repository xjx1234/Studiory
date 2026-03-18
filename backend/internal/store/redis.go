package store

import (
	"context"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// NewRedis 创建 Redis 客户端。
// 调用方需在程序退出时调用 client.Close()。
func NewRedis(ctx context.Context, redisURL string, log *zap.Logger) (redis.UniversalClient, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}

	client := redis.NewClient(opt)

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}

	if log != nil {
		log.Info("Redis 连接成功")
	}

	return client, nil
}
