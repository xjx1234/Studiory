package middleware

import (
	"backend/pkg/reqid"

	"github.com/gin-contrib/requestid"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequestID 为每个请求注入 request id，并回写到响应头（X-Request-Id）。
//
// 在调用 requestid 中间件之前就把 id 放进 request context：
// 库内部会立刻 c.Next() 执行后续链路，等它返回再注入就晚了（业务日志已经打完）。
func RequestID() gin.HandlerFunc {
	inner := requestid.New()

	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" {
			id = uuid.NewString()
			c.Request.Header.Set("X-Request-ID", id)
		}
		c.Request = c.Request.WithContext(reqid.WithID(c.Request.Context(), id))
		inner(c)
	}
}
