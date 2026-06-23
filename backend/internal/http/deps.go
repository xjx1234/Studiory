package http

import (
	"context"
	"net/http"

	"backend/internal/config"
	adminservice "backend/internal/service/admin"
	authservice "backend/internal/service/auth"
	todoservice "backend/internal/service/todo"
	userservice "backend/internal/service/user"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ReadyCheck 是 /ready 使用的依赖探针。
// app 层负责把 PG/Redis 等基础设施包装成探针，http handler 不直接持有基础设施对象。
type ReadyCheck struct {
	Name  string
	Check func(ctx context.Context) error
}

// Deps 是 http 层所需依赖的集合（由 internal/app 负责装配注入）。
//
// 约束：
// - handler 只通过 Deps 访问 service，不直接持有 Store/Redis 等基础设施。
// - 新增业务模块时：在 service 层实现逻辑，在此追加 Service 字段，并在 app.New() 中注入。
type Deps struct {
	Cfg    *config.Config
	Logger *zap.Logger

	AuthService authservice.Service
	UserService userservice.Service

	// AdminService 后台用户管理示例模块
	AdminService adminservice.Service

	// TodoService 示例模块（可复制此模式添加真实业务）
	TodoService todoservice.Service

	AuthMiddleware      gin.HandlerFunc
	RateLimitMiddleware gin.HandlerFunc
	ReadyChecks         []ReadyCheck

	// MetricsMiddleware / MetricsHandler 为可选项（metrics.enabled=false 时为 nil）。
	// 由 app 层装配：handler 不直接持有基础设施，仅挂载 Prometheus 暴露端点。
	MetricsMiddleware gin.HandlerFunc
	MetricsHandler    http.Handler
}
