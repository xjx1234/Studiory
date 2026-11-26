package middleware

import (
	"strings"
	"time"

	"backend/internal/auth"
	"backend/internal/session"
	"backend/pkg/errcode"
	"backend/pkg/resp"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const ContextKeyUserID = "userID"
const ContextKeyUserRole = "userRole"

// Auth 验证请求头中的 JWT Access Token，将用户信息注入 Gin Context。
// 验证失败时直接返回 401，不继续执行后续 Handler。
//
// 吊销检查与 session 校验合并为一次 Redis MGET（session.Store.CheckAccess），
// 每个已鉴权请求只需一次往返。sessions 为 nil 时跳过这两项检查（仅用于单测）。
// failOpen 控制 Redis 故障时的降级策略：true=放行（可用性优先）；false=拒绝（安全性优先）。
func Auth(issuer *auth.TokenIssuer, sessions *session.Store, failOpen bool, logger *zap.Logger) gin.HandlerFunc {
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

		claims, err := issuer.ParseAccessToken(parts[1])
		if err != nil {
			resp.Fail(c, errcode.ErrInvalidToken)
			return
		}

		if sessions != nil {
			var issuedAt *time.Time
			if claims.IssuedAt != nil {
				iat := claims.IssuedAt.Time
				issuedAt = &iat
			}
			valid, stateErr := sessions.CheckAccess(c.Request.Context(), claims.UserID, claims.SessionID, issuedAt)
			if stateErr != nil {
				strategy := "fail-open"
				if !failOpen {
					strategy = "fail-closed"
				}
				if logger != nil {
					logger.Warn("access token state check unavailable, "+strategy,
						zap.Error(stateErr),
						zap.String("user_id", claims.UserID),
					)
				}
				if !failOpen {
					resp.Fail(c, errcode.ErrInvalidToken)
					return
				}
			} else if !valid {
				resp.Fail(c, errcode.ErrInvalidToken)
				return
			}
		}

		c.Set(ContextKeyUserID, claims.UserID)
		c.Set(ContextKeyUserRole, claims.Role)
		c.Next()
	}
}

// RequireRole 要求当前用户具备指定角色之一。
// 必须挂在 Auth() 之后，否则上下文中不会存在 userRole。
func RequireRole(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		allowed[role] = struct{}{}
	}

	return func(c *gin.Context) {
		rawRole, exists := c.Get(ContextKeyUserRole)
		if !exists {
			resp.Fail(c, errcode.ErrUnauthorized)
			return
		}

		role, ok := rawRole.(string)
		if !ok {
			resp.Fail(c, errcode.ErrForbidden)
			return
		}
		if _, ok := allowed[role]; !ok {
			resp.Fail(c, errcode.ErrForbidden)
			return
		}

		c.Next()
	}
}
