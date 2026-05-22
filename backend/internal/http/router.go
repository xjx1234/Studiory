package http

import (
	"backend/internal/http/middleware"
	"time"

	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// NewRouter 构建 Gin 路由，挂载全局中间件与所有路由分组。
//
// 中间件顺序：RequestID → I18n → Zap日志 → Recovery → Safe → RateLimit → CORS
func NewRouter(deps *Deps) *gin.Engine {
	r := gin.New()

	r.Use(middleware.RequestID())
	r.Use(middleware.I18n())
	r.Use(ginzap.Ginzap(zap.L(), time.RFC3339, true))
	r.Use(ginzap.RecoveryWithZap(zap.L(), true))
	r.Use(middleware.Safe())
	r.Use(middleware.RateLimit(deps.Cfg.RateLimitPerMinute, deps.Redis, deps.Cfg.RedisKeyPrefix))
	r.Use(middleware.CORS(middleware.CORSOptions{
		AllowOrigins:     deps.Cfg.CORSAllowOrigins,
		AllowCredentials: deps.Cfg.CORSAllowCredentials,
	}))

	registerHealthRoutes(r, deps)

	v1 := r.Group("/api/v1")
	{
		registerAuthRoutes(v1, deps)

		user := v1.Group("/user", middleware.Auth())
		registerUserProfileRoutes(user, deps)
		registerUserTodoRoutes(user, deps)

		admin := v1.Group("/admin", middleware.Auth(), middleware.RequireRole("admin"))
		registerAdminRoutes(admin, deps)
	}

	return r
}
