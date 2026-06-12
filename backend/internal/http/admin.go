package http

import (
	"backend/internal/repo"
	"backend/pkg/resp"

	"github.com/gin-gonic/gin"
)

func registerAdminRoutes(rg *gin.RouterGroup, _ *Deps) {
	rg.GET("/ping", func(c *gin.Context) {
		resp.OK(c, gin.H{"status": "ok", "scope": repo.RoleAdmin})
	})
}
