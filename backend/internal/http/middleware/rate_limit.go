package middleware

import (
	"time"

	"backend/pkg/errcode"
	"backend/pkg/resp"

	"github.com/gin-gonic/gin"
	"github.com/ulule/limiter/v3"
	ginmiddleware "github.com/ulule/limiter/v3/drivers/middleware/gin"
	memory "github.com/ulule/limiter/v3/drivers/store/memory"
	limiteredis "github.com/ulule/limiter/v3/drivers/store/redis"

	"github.com/redis/go-redis/v9"
)

// RateLimit 使用 ulule/limiter 实现 IP 维度限流。
//
// 后续可按路由分组设置不同策略，或改为 Redis store 做分布式限流。
// 当 rdb 传入非空时优先使用 Redis 分布式 store；否则回退到进程内 memory store。
func RateLimit(perMinute int, rdb redis.UniversalClient, redisKeyPrefix string) gin.HandlerFunc {
	if perMinute <= 0 {
		perMinute = 120
	}
	rate := limiter.Rate{
		Period: 1 * time.Minute,
		Limit:  int64(perMinute),
	}

	var store limiter.Store
	if rdb != nil {
		prefix := redisKeyPrefix
		if prefix == "" {
			prefix = "app"
		}

		s, err := limiteredis.NewStoreWithOptions(rdb, limiter.StoreOptions{
			Prefix:   prefix + ":limiter",
			MaxRetry: 3,
		})
		if err == nil && s != nil {
			store = s
		}
	}

	if store == nil {
		store = memory.NewStore()
	}
	instance := limiter.New(store, rate)

	return ginmiddleware.NewMiddleware(instance,
		ginmiddleware.WithLimitReachedHandler(func(c *gin.Context) {
			resp.Fail(c, errcode.ErrTooManyRequests)
		}),
	)
}
