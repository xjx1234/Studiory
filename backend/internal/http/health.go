package http

import (
	"backend/pkg/resp"

	"github.com/gin-gonic/gin"
)

func registerHealthRoutes(r *gin.Engine) {
	r.GET("/health", func(c *gin.Context) {
		resp.OK(c, gin.H{"status": "ok", "service": "拾习社 API"})
	})
}
