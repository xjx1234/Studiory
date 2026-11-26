package http

import (
	"context"
	"net/http"

	"backend/internal/config"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ReadyCheck 是 /ready 使用的依赖探针。
// app 层负责把 PG/Redis 等基础设施包装成探针，http handler 不直接持有基础设施对象。
type ReadyCheck struct {
	Name  string
	Check func(ctx context.Context) error
}

// Deps 只保留「全局共享」的依赖：
//   - 配置、日志、健康检查、指标等路由器自身需要的东西；
//   - Modules：各业务模块自己持有 service 与路由中间件，集中在这里按顺序注册。
//
// 新增业务模块不需要再改本结构体或 validateDeps，只需实现 Module 并在 app.New 的
// Modules 列表里追加一项，详见 module.go。
type Deps struct {
	Cfg    *config.Config
	Logger *zap.Logger

	// RateLimitMiddleware 是全局 IP 限流，挂在所有路由上（含未鉴权的 /auth）。
	RateLimitMiddleware gin.HandlerFunc
	ReadyChecks         []ReadyCheck

	// MetricsMiddleware / MetricsHandler 为可选项（metrics.enabled=false 时为 nil）。
	// 由 app 层装配：handler 不直接持有基础设施，仅挂载 Prometheus 暴露端点。
	MetricsMiddleware gin.HandlerFunc
	MetricsHandler    http.Handler

	// Modules 是 /api/v1 下的业务路由模块，按声明顺序注册。
	Modules []Module
}
