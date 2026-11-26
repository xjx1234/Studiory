package middleware

import (
	"context"
	"errors"
	"time"

	"backend/pkg/errcode"
	"backend/pkg/resp"

	"github.com/gin-gonic/gin"
)

// RouteBudget 为单条路由覆盖请求预算。Path 必须是 Gin 的路由模板
// （如 /api/v1/auth/send-code），Budget <= 0 的条目被忽略（沿用全局预算）。
type RouteBudget struct {
	Path   string
	Budget time.Duration
}

// Timeout 为每个请求挂上 context 超时，并允许按路由模板放宽个别端点。
//
// 关键点：
//   - 预算必须在创建 ctx 之前定好：context.WithTimeout 只会缩短、不会延长。在 5s 外层
//     deadline 之内再给某条路由挂一层 20s 的 Timeout，实际生效的仍是 5s——放宽只能发生
//     在这里，不能靠嵌套中间件实现。
//   - c.FullPath() 由 gin 在进入中间件链之前赋值为路由模板（非真实路径），因此按模板匹配；
//     未匹配路由（404）拿全局预算。
//   - 不额外起 goroutine：handler 与 DB/Redis 共享同一个 ctx，超时会沿 ctx 传播到
//     pgx / go-redis，让阻塞的查询尽快返回并把连接还给连接池；
//   - handler 若在超时后把底层错误当作普通内部错误返回，resp.Fail 会统一改写成 503，
//     避免「依赖超时」被误报成 500；
//   - handler 未写响应就返回时，这里补一个统一的 503 信封，保证客户端一定能解析。
//
// 全局预算与命中的覆盖预算都 <= 0 时不做任何事（配置校验要求 > 0，这里只兜底单测等场景）。
func Timeout(d time.Duration, overrides ...RouteBudget) gin.HandlerFunc {
	return func(c *gin.Context) {
		budget := d
		for _, o := range overrides {
			if o.Budget > 0 && o.Path == c.FullPath() {
				budget = o.Budget
				break
			}
		}
		if budget <= 0 {
			c.Next()
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), budget)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)

		c.Next()

		if !c.Writer.Written() && errors.Is(ctx.Err(), context.DeadlineExceeded) {
			resp.Fail(c, errcode.ErrServiceUnavailable)
		}
	}
}
