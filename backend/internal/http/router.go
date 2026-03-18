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
// 全局中间件顺序：
//   RequestID → I18n → Zap日志 → Recovery → RateLimit → CORS
func NewRouter() *gin.Engine {
	r := gin.New()

	r.Use(middleware.RequestID())                        // 请求 ID（X-Request-Id）
	r.Use(middleware.I18n())                             // 国际化（注入 Localizer）
	r.Use(ginzap.Ginzap(zap.L(), time.RFC3339, true))   // Zap 请求日志
	r.Use(ginzap.RecoveryWithZap(zap.L(), true))         // Panic 恢复
	r.Use(middleware.Safe())                             // 安全（Body 大小限制等）
	r.Use(middleware.RateLimit())                        // 限流
	r.Use(middleware.CORS())                             // 跨域

	// 健康检查（无需鉴权）
	registerHealthRoutes(r)

	v1 := r.Group("/api/v1")
	{
		// 认证（无需鉴权）
		registerAuthRoutes(v1)

		// 用户端（需鉴权）
		user := v1.Group("/user", middleware.Auth())
		registerUserEnglishWordRoutes(user)

		// 管理端（需鉴权）
		admin := v1.Group("/admin", middleware.Auth())
		registerAdminEnglishWordRoutes(admin)
	}

	return r
}
