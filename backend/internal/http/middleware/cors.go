package middleware

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

type CORSOptions struct {
	AllowOrigins     []string
	AllowCredentials bool
}

// CORS 返回跨域中间件。
//
// AllowOrigins 必须由配置提供且非空：config.Validate 已拒绝空列表，
// 这里不再兜底 localhost——兜底会让「忘了配生产域名」的部署静默放开本地开发源。
// 万一绕过 Validate 传了空列表，返回「不加任何跨域头」的空中间件，
// 而不是让 cors.New 直接 panic。
func CORS(opts CORSOptions) gin.HandlerFunc {
	if len(opts.AllowOrigins) == 0 {
		return func(c *gin.Context) { c.Next() }
	}

	return cors.New(cors.Config{
		AllowOrigins: opts.AllowOrigins,
		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders: []string{
			"Origin",
			"Content-Type",
			"Authorization",
			"X-Requested-With",
		},
		// X-Request-Id 由 RequestID 中间件写到响应头，但浏览器默认读不到；
		// 不暴露的话前端报错时无法把页面异常和后端日志对上。
		ExposeHeaders:    []string{"Content-Length", "X-Request-Id"},
		AllowCredentials: opts.AllowCredentials,
		MaxAge:           12 * time.Hour,
	})
}
