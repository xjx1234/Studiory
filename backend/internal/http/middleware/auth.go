package middleware

import (
	"strings"

	"backend/internal/auth"
	"backend/pkg/errcode"
	"backend/pkg/resp"

	"github.com/gin-gonic/gin"
)

const ContextKeyUserID   = "userID"
const ContextKeyUserRole = "userRole"

// Auth 验证请求头中的 JWT Access Token，将用户信息注入 Gin Context。
// 验证失败时直接返回 401，不继续执行后续 Handler。
func Auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := c.GetHeader("Authorization")
		if raw == "" {
			resp.Fail(c, errcode.ErrUnauthorized)
			return
		}

		parts := strings.SplitN(raw, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			resp.Fail(c, errcode.ErrInvalidToken)
			return
		}

		claims, err := auth.ParseAccessToken(parts[1])
		if err != nil {
			resp.Fail(c, errcode.ErrInvalidToken)
			return
		}

		c.Set(ContextKeyUserID, claims.UserID)
		c.Set(ContextKeyUserRole, claims.Role)
		c.Next()
	}
}
