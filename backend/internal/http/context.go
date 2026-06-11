package http

import (
	"backend/internal/http/middleware"
	"backend/pkg/errcode"
	"backend/pkg/resp"

	"github.com/gin-gonic/gin"
)

// mustUserID 从 Gin Context 中取出 userID 字符串。
// 若不存在或类型异常，直接写 401 并返回 ("", false)，调用方应立即 return。
func mustUserID(c *gin.Context) (string, bool) {
	raw, exists := c.Get(middleware.ContextKeyUserID)
	if !exists {
		resp.Fail(c, errcode.ErrUnauthorized)
		return "", false
	}
	id, ok := raw.(string)
	if !ok || id == "" {
		resp.Fail(c, errcode.ErrUnauthorized)
		return "", false
	}
	return id, true
}
