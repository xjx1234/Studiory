package http

import (
	"errors"
	"time"

	"backend/internal/http/middleware"
	"backend/internal/repo"

	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// NewRouter 构建 Gin 路由，挂载全局中间件与所有路由分组。
//
// 中间件顺序：RequestID → SecurityHeaders → I18n → Zap日志 → Recovery → Safe → RateLimit → CORS
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
	r.Use(ginzap.Ginzap(zap.L(), time.RFC3339, true))
	r.Use(ginzap.RecoveryWithZap(zap.L(), true))
	r.Use(middleware.Safe())
	r.Use(deps.RateLimitMiddleware)
	r.Use(middleware.CORS(middleware.CORSOptions{
		AllowOrigins:     deps.Cfg.CORSAllowOrigins,
		AllowCredentials: deps.Cfg.CORSAllowCredentials,
	}))

	registerHealthRoutes(r, deps)

	if deps.MetricsHandler != nil {
		r.GET("/metrics", gin.WrapH(deps.MetricsHandler))
	}

	v1 := r.Group("/api/v1")
	{
		registerAuthRoutes(v1, deps)

		user := v1.Group("/user", deps.AuthMiddleware)
		registerUserProfileRoutes(user, deps)
		registerUserTodoRoutes(user, deps)

		admin := v1.Group("/admin", deps.AuthMiddleware, middleware.RequireRole(repo.RoleAdmin))
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
	if len(deps.ReadyChecks) == 0 {
		return errors.New("ReadyChecks is required")
	}
	return nil
}
