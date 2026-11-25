package middleware

import (
	"context"
	"errors"
	"time"

	"backend/pkg/errcode"
	"backend/pkg/resp"

	"github.com/gin-gonic/gin"
)

// Timeout 为每个请求挂上 context 超时。
//
// 关键点：
//   - 不额外起 goroutine：handler 与 DB/Redis 共享同一个 ctx，超时会沿 ctx 传播到
//     pgx / go-redis，让阻塞的查询尽快返回并把连接还给连接池；
//   - handler 若在超时后把底层错误当作普通内部错误返回，resp.Fail 会统一改写成 503，
//     避免「依赖超时」被误报成 500；
//   - handler 未写响应就返回时，这里补一个统一的 503 信封，保证客户端一定能解析。
//
// d <= 0 时不做任何事（配置校验要求 > 0，这里只兜底单测等场景）。
func Timeout(d time.Duration) gin.HandlerFunc {
	if d <= 0 {
		return func(c *gin.Context) { c.Next() }
	}

	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), d)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)

		c.Next()

		if !c.Writer.Written() && errors.Is(ctx.Err(), context.DeadlineExceeded) {
			resp.Fail(c, errcode.ErrServiceUnavailable)
		}
	}
}
