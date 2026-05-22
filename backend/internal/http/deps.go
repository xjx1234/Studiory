package http

import (
	"backend/internal/config"
	"backend/internal/repo/pg"
	authservice "backend/internal/service/auth"
	todoservice "backend/internal/service/todo"
	userservice "backend/internal/service/user"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Deps 是 http 层所需依赖的集合（由 internal/app 负责装配注入）。
//
// 约束：
// - handler 只通过 Deps 访问下层能力，不在 http 层创建 DB/Redis 等对象。
// - 新增业务模块时：在 service 层实现逻辑，在此追加 Service 字段，并在 app.New() 中注入。
type Deps struct {
	Cfg    *config.Config
	Logger *zap.Logger
	Store  *pg.Store
	Redis  redis.UniversalClient

	AuthService authservice.Service
	UserService userservice.Service

	// TodoService 示例模块（可复制此模式添加真实业务）
	TodoService todoservice.Service
}
