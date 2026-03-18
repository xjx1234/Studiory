package middleware

import (
	"github.com/gin-contrib/requestid"
	"github.com/gin-gonic/gin"
)

// RequestID 为每个请求注入 request id，并回写到响应头（X-Request-Id）。
// 使用 gin-contrib/requestid 的成熟实现。
func RequestID() gin.HandlerFunc {
	return requestid.New()
}

