package http

import (
	"context"
	"sync"
	"time"

	"backend/internal/buildinfo"
	"backend/pkg/errcode"
	"backend/pkg/resp"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// readyCheckTimeout 是 /ready 所有探针共享的总预算。
const readyCheckTimeout = 2 * time.Second

// readyCheckResult 是 /ready 的单项探活结果。
// 失败只回显通用原因，具体错误写日志——探活端点通常不鉴权，
// 不能把连接串、内网地址之类的信息暴露出去。
type readyCheckResult struct {
	Name      string `json:"name"`
	OK        bool   `json:"ok"`
	LatencyMS int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

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

	// /ready 并发探测各依赖：串行时总耗时是各项之和，任一慢依赖都会拖长整体探活时间，
	// 超过 LB 的探测超时就会把健康实例摘掉。所有探针共享同一个 2s 预算，
	// 任何一项失败都返回 503，但会带上逐项结果——运维一眼能看出是哪个依赖挂了。
	r.GET("/ready", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), readyCheckTimeout)
		defer cancel()

		results := make([]readyCheckResult, len(deps.ReadyChecks))
		var wg sync.WaitGroup
		for i, check := range deps.ReadyChecks {
			wg.Add(1)
			go func(i int, check ReadyCheck) {
				defer wg.Done()
				results[i] = runReadyCheck(ctx, check, deps.Logger)
			}(i, check)
		}
		wg.Wait()

		payload := gin.H{"status": "ready", "checks": results}
		for _, result := range results {
			if result.OK {
				continue
			}
			payload["status"] = "unready"
			resp.FailWithData(c, errcode.ErrServiceUnavailable, payload)
			return
		}
		resp.OK(c, payload)
	})
}

// runReadyCheck 执行单个探针并记录耗时；ctx 超时与依赖报错都算不健康。
func runReadyCheck(ctx context.Context, check ReadyCheck, logger *zap.Logger) readyCheckResult {
	start := time.Now()
	result := readyCheckResult{Name: check.Name}

	switch check.Check {
	case nil:
		result.Error = "未初始化"
	default:
		if err := check.Check(ctx); err != nil {
			result.Error = "不可用"
			if logger != nil {
				logger.Warn("readiness check failed", zap.String("check", check.Name), zap.Error(err))
			}
		} else {
			result.OK = true
		}
	}

	result.LatencyMS = time.Since(start).Milliseconds()
	return result
}
