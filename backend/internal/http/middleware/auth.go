package middleware

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"backend/internal/auth"
	"backend/internal/session"
	"backend/pkg/errcode"
	"backend/pkg/resp"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const ContextKeyUserID = "userID"
const ContextKeyUserRole = "userRole"

// Auth 验证请求头中的 JWT Access Token，将用户信息注入 Gin Context。
// 验证失败时直接返回 401，不继续执行后续 Handler。
func Auth(issuer *auth.TokenIssuer, sessions *session.Store, rdb redis.UniversalClient, keyPrefix string, logger *zap.Logger) gin.HandlerFunc {
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

		if isAccessTokenRevoked(c.Request.Context(), rdb, keyPrefix, claims, logger) {
			resp.Fail(c, errcode.ErrInvalidToken)
			return
		}

		if claims.SessionID != "" && sessions != nil && !sessions.Validate(c.Request.Context(), claims.UserID, claims.SessionID) {
			resp.Fail(c, errcode.ErrInvalidToken)
			return
		}

		c.Set(ContextKeyUserID, claims.UserID)
		c.Set(ContextKeyUserRole, claims.Role)
		c.Next()
	}
}

func isAccessTokenRevoked(ctx context.Context, rdb redis.UniversalClient, keyPrefix string, claims *auth.Claims, logger *zap.Logger) bool {
	if rdb == nil || claims == nil || claims.IssuedAt == nil {
		return false
	}

	revokeKey := fmt.Sprintf("%s:revoke:uid:%s", keyPrefix, claims.UserID)
	revokeAt, err := rdb.Get(ctx, revokeKey).Int64()
	if err != nil {
		if !errors.Is(err, redis.Nil) && logger != nil {
			logger.Warn("access token revoke check unavailable, failing open",
				zap.Error(err),
				zap.String("user_id", claims.UserID),
			)
		}
		return false
	}
	return claims.IssuedAt.Unix() <= revokeAt
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
