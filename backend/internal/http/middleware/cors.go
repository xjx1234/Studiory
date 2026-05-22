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
func CORS(opts CORSOptions) gin.HandlerFunc {
	allowOrigins := opts.AllowOrigins
	if len(allowOrigins) == 0 {
		allowOrigins = []string{"http://localhost:5173", "http://localhost:3000"}
	}

	return cors.New(cors.Config{
		AllowOrigins: allowOrigins,
		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders: []string{
			"Origin",
			"Content-Type",
			"Authorization",
			"X-Requested-With",
		},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: opts.AllowCredentials,
		MaxAge:           12 * time.Hour,
	})
}
