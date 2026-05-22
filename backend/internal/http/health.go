package http

import (
	"context"
	"time"

	"backend/pkg/errcode"
	"backend/pkg/resp"

	"github.com/gin-gonic/gin"
)

func registerHealthRoutes(r *gin.Engine, deps *Deps) {
	r.GET("/health", func(c *gin.Context) {
		resp.OK(c, gin.H{"status": "ok", "service": "api"})
	})

	r.GET("/ready", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		if deps.Store == nil || deps.Store.Pool() == nil {
			resp.FailWithMessage(c, errcode.ErrServiceUnavailable, "PostgreSQL 未初始化")
			return
		}
		if err := deps.Store.Pool().Ping(ctx); err != nil {
			resp.FailWithMessage(c, errcode.ErrServiceUnavailable, "PostgreSQL 不可用")
			return
		}
		if deps.Redis == nil {
			resp.FailWithMessage(c, errcode.ErrServiceUnavailable, "Redis 未初始化")
			return
		}
		if err := deps.Redis.Ping(ctx).Err(); err != nil {
			resp.FailWithMessage(c, errcode.ErrServiceUnavailable, "Redis 不可用")
			return
		}

		resp.OK(c, gin.H{"status": "ready"})
	})
}
