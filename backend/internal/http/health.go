package http

import (
	"context"
	"time"

	"backend/internal/buildinfo"
	"backend/pkg/errcode"
	"backend/pkg/resp"

	"github.com/gin-gonic/gin"
)

func registerHealthRoutes(r *gin.Engine, deps *Deps) {
	r.GET("/health", func(c *gin.Context) {
		info := buildinfo.Current()
		resp.OK(c, gin.H{
			"status":     "ok",
			"service":    "api",
			"version":    info.Version,
			"commit":     info.Commit,
			"build_time": info.BuildTime,
		})
	})

	r.GET("/ready", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		for _, check := range deps.ReadyChecks {
			if check.Check == nil {
				resp.FailWithMessage(c, errcode.ErrServiceUnavailable, check.Name+" 未初始化")
				return
			}
			if err := check.Check(ctx); err != nil {
				resp.FailWithMessage(c, errcode.ErrServiceUnavailable, check.Name+" 不可用")
				return
			}
		}

		resp.OK(c, gin.H{"status": "ready"})
	})
}
