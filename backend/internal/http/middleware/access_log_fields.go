package middleware

import (
	"github.com/gin-contrib/requestid"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// AccessLogFields 供 ginzap 追加标准关联字段（request_id、user_id）。
// 在 c.Next() 之后调用，已鉴权路由可读到 Auth 注入的 user_id。
func AccessLogFields(c *gin.Context) []zapcore.Field {
	fields := []zapcore.Field{
		zap.String("request_id", requestid.Get(c)),
	}
	if raw, ok := c.Get(ContextKeyUserID); ok {
		if uid, ok := raw.(string); ok && uid != "" {
			fields = append(fields, zap.String("user_id", uid))
		}
	}
	return fields
}
