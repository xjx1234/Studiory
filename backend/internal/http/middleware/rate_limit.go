package middleware

import (
	"time"

	"backend/pkg/errcode"
	"backend/pkg/resp"

	"github.com/gin-gonic/gin"
	"github.com/ulule/limiter/v3"
	ginmiddleware "github.com/ulule/limiter/v3/drivers/middleware/gin"
	memory "github.com/ulule/limiter/v3/drivers/store/memory"
)

// RateLimit 使用 ulule/limiter 实现 IP 维度限流。
//
// 默认策略：同一 IP 每分钟最多 120 次请求。
// 后续可按路由分组设置不同策略，或改为 Redis store 做分布式限流。
func RateLimit() gin.HandlerFunc {
	rate := limiter.Rate{
		Period: 1 * time.Minute,
		Limit:  120,
	}

	store := memory.NewStore()
	instance := limiter.New(store, rate)

	return ginmiddleware.NewMiddleware(instance,
		ginmiddleware.WithLimitReachedHandler(func(c *gin.Context) {
			resp.Fail(c, errcode.ErrTooManyRequests)
		}),
	)
}
