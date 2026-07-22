package http

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"backend/internal/http/middleware"
	"backend/internal/repo"

	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewRouter 构建 Gin 路由，挂载全局中间件与所有路由分组。
//
// 中间件顺序：RequestID → … → Safe → RateLimit(IP) → CORS
// 已鉴权分组额外挂载 UserRateLimit(user_id)，在 Auth 之后执行。
func NewRouter(deps *Deps) (*gin.Engine, error) {
	if err := validateDeps(deps); err != nil {
		return nil, err
	}

	r := gin.New()

	r.Use(middleware.RequestID())
	if deps.MetricsMiddleware != nil {
		r.Use(deps.MetricsMiddleware)
	}
	r.Use(middleware.SecurityHeaders(deps.Cfg.IsProd()))
	r.Use(middleware.I18n())
	r.Use(ginzap.GinzapWithConfig(zap.L(), &ginzap.Config{
		TimeFormat:   time.RFC3339,
		UTC:          true,
		DefaultLevel: zapcore.InfoLevel,
		Context:      middleware.AccessLogFields,
	}))
	r.Use(ginzap.RecoveryWithZap(zap.L(), true))
	r.Use(middleware.Safe())
	r.Use(deps.RateLimitMiddleware)
	r.Use(middleware.CORS(middleware.CORSOptions{
		AllowOrigins:     deps.Cfg.CORSAllowOrigins,
		AllowCredentials: deps.Cfg.CORSAllowCredentials,
	}))

	registerHealthRoutes(r, deps)

	if deps.MetricsHandler != nil {
		handler := gin.WrapH(deps.MetricsHandler)
		if deps.Cfg.MetricsToken != "" {
			handler = metricsBearerAuth(deps.Cfg.MetricsToken, handler)
		}
		r.GET("/metrics", handler)
	}

	v1 := r.Group("/api/v1")
	{
		registerAuthRoutes(v1, deps)

		user := v1.Group("/user", deps.AuthMiddleware, deps.UserRateLimitMiddleware)
		registerUserProfileRoutes(user, deps)
		registerUserTodoRoutes(user, deps)

		admin := v1.Group("/admin", deps.AuthMiddleware, deps.UserRateLimitMiddleware, middleware.RequireRole(repo.RoleAdmin))
		registerAdminRoutes(admin, deps)
	}

	return r, nil
}

func validateDeps(deps *Deps) error {
	if deps == nil {
		return errors.New("http deps is nil")
	}
	if deps.AuthService == nil {
		return errors.New("AuthService is required")
	}
	if deps.UserService == nil {
		return errors.New("UserService is required")
	}
	if deps.TodoService == nil {
		return errors.New("TodoService is required")
	}
	if deps.AdminService == nil {
		return errors.New("AdminService is required")
	}
	if deps.AuthMiddleware == nil {
		return errors.New("AuthMiddleware is required")
	}
	if deps.RateLimitMiddleware == nil {
		return errors.New("RateLimitMiddleware is required")
	}
	if deps.UserRateLimitMiddleware == nil {
		return errors.New("UserRateLimitMiddleware is required")
	}
	if len(deps.ReadyChecks) == 0 {
		return errors.New("ReadyChecks is required")
	}
	return nil
}

// metricsBearerAuth 校验 /metrics 请求的 Bearer token。
// 兼容 Prometheus scrape_config 的 bearer_token 字段（发送 Authorization: Bearer <token>）。
func metricsBearerAuth(token string, next gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if auth == "" {
			// 兼容部分采集器通过 query 参数传递 token
			if q := strings.TrimSpace(c.Query("token")); q != "" {
				auth = "Bearer " + q
			}
		}
		if auth != "Bearer "+token {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	}
}
