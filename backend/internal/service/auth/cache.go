package authservice

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// CacheStore 封装 auth service 所需的缓存操作，将 Redis 细节隔离在适配层，
// 业务代码只依赖最小接口集，便于单元测试 mock 和后续替换存储实现。
type CacheStore interface {
	SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	Eval(ctx context.Context, script string, keys []string, args ...any) (any, error)
	Del(ctx context.Context, keys ...string) error
	Exists(ctx context.Context, key string) (bool, error)
}

// redisCacheStore 是 CacheStore 的 Redis 实现，适配 redis.UniversalClient。
type redisCacheStore struct {
	rdb redis.UniversalClient
}

// NewRedisCacheStore 将 redis.UniversalClient 包装为 CacheStore。
func NewRedisCacheStore(rdb redis.UniversalClient) CacheStore {
	return &redisCacheStore{rdb: rdb}
}

func (s *redisCacheStore) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	return s.rdb.SetNX(ctx, key, value, ttl).Result()
}

func (s *redisCacheStore) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return s.rdb.Set(ctx, key, value, ttl).Err()
}

func (s *redisCacheStore) Eval(ctx context.Context, script string, keys []string, args ...any) (any, error) {
	return s.rdb.Eval(ctx, script, keys, args...).Result()
}

func (s *redisCacheStore) Del(ctx context.Context, keys ...string) error {
	return s.rdb.Del(ctx, keys...).Err()
}

func (s *redisCacheStore) Exists(ctx context.Context, key string) (bool, error) {
	n, err := s.rdb.Exists(ctx, key).Result()
	return n > 0, err
}
