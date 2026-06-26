package middleware

import (
	"strings"
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

// RateLimit 对未登录/公开路由按客户端 IP 限流。
//
// /api/v1/user 与 /api/v1/admin 由 RateLimitByUser 单独按 user_id 限流，此处跳过以免同 NAT 误伤。
// rdb 非空时使用 Redis 分布式 store，否则回退进程内 memory store。
func RateLimit(perMinute int, rdb redis.UniversalClient, redisKeyPrefix string) gin.HandlerFunc {
	return newRateLimiter(perMinute, rdb, redisKeyPrefix, "ip", func(c *gin.Context) string {
		if isUserScopedAPIPath(c.Request.URL.Path) {
			return ""
		}
		return c.ClientIP()
	})
}

// RateLimitByUser 对已鉴权路由按 user_id 限流，应挂在 Auth 中间件之后。
func RateLimitByUser(perMinute int, rdb redis.UniversalClient, redisKeyPrefix string) gin.HandlerFunc {
	return newRateLimiter(perMinute, rdb, redisKeyPrefix, "uid", func(c *gin.Context) string {
		raw, exists := c.Get(ContextKeyUserID)
		if !exists {
			return ""
		}
		id, ok := raw.(string)
		if !ok || id == "" {
			return ""
		}
		return id
	})
}

func isUserScopedAPIPath(path string) bool {
	return strings.HasPrefix(path, "/api/v1/user") || strings.HasPrefix(path, "/api/v1/admin")
}

func newRateLimiter(
	perMinute int,
	rdb redis.UniversalClient,
	redisKeyPrefix, scope string,
	keyGetter ginmiddleware.KeyGetter,
) gin.HandlerFunc {
	if perMinute <= 0 {
		perMinute = 120
	}
	rate := limiter.Rate{
		Period: 1 * time.Minute,
		Limit:  int64(perMinute),
	}

	store := newLimiterStore(rdb, redisKeyPrefix, scope)
	instance := limiter.New(store, rate)

	return ginmiddleware.NewMiddleware(instance,
		ginmiddleware.WithKeyGetter(keyGetter),
		ginmiddleware.WithExcludedKey(func(key string) bool { return key == "" }),
		ginmiddleware.WithLimitReachedHandler(func(c *gin.Context) {
			resp.Fail(c, errcode.ErrTooManyRequests)
		}),
	)
}

func newLimiterStore(rdb redis.UniversalClient, redisKeyPrefix, scope string) limiter.Store {
	prefix := redisKeyPrefix
	if prefix == "" {
		prefix = "app"
	}

	if rdb != nil {
		s, err := limiteredis.NewStoreWithOptions(rdb, limiter.StoreOptions{
			Prefix:   prefix + ":limiter:" + scope,
			MaxRetry: 3,
		})
		if err == nil && s != nil {
			return s
		}
	}
	return memory.NewStore()
}
