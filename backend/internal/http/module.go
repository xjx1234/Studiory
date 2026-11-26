package http

import (
	"errors"

	"backend/internal/http/middleware"
	"backend/internal/repo"
	adminservice "backend/internal/service/admin"
	authservice "backend/internal/service/auth"
	todoservice "backend/internal/service/todo"
	userservice "backend/internal/service/user"
	"backend/pkg/resp"

	"github.com/gin-gonic/gin"
)

// Module 是可独立注册的路由模块。
//
// 新增业务模块只需两步：
//  1. 在 http 包内实现 Module，模块自己持有 service 与其路由所需的中间件；
//  2. 在 app.New 的 Modules 列表里追加一项。
//
// 路由前缀、鉴权/限流中间件都由模块自己声明：router.go 与 Deps 不再随模块增加而修改，
// 缺依赖时 Register 返回错误，启动即失败（替代原先集中在 validateDeps 里的逐字段检查）。
type Module interface {
	Name() string
	// Register 把本模块的路由挂到 /api/v1 分组下。
	Register(v1 *gin.RouterGroup) error
}

// AuthModule 注册 /auth（注册、登录、刷新、登出），无需鉴权。
type AuthModule struct {
	Svc authservice.Service
}

func (m AuthModule) Name() string { return "auth" }

func (m AuthModule) Register(v1 *gin.RouterGroup) error {
	if m.Svc == nil {
		return errors.New("AuthService is required")
	}
	h := NewAuthHandler(m.Svc)
	g := v1.Group("/auth")
	g.POST("/register", h.Register)
	g.POST("/login", h.Login)
	g.POST("/send-code", h.SendCode)
	g.POST("/refresh", h.Refresh)
	g.POST("/logout", h.Logout)
	return nil
}

// UserModule 注册 /user（个人资料、改密，需登录 + 用户级限流）。
type UserModule struct {
	Svc       userservice.Service
	Auth      gin.HandlerFunc
	RateLimit gin.HandlerFunc
}

func (m UserModule) Name() string { return "user" }

func (m UserModule) Register(v1 *gin.RouterGroup) error {
	if err := requireAuthedDeps("user", m.Svc, m.Auth, m.RateLimit); err != nil {
		return err
	}
	h := NewUserProfileHandler(m.Svc)
	rg := v1.Group("/user", m.Auth, m.RateLimit)
	rg.GET("/profile", h.GetProfile)
	rg.PATCH("/profile", h.UpdateProfile)
	rg.PATCH("/password", h.ChangePassword)
	return nil
}

// TodoModule 注册 /user/todos（示例业务模块，需登录 + 用户级限流）。
type TodoModule struct {
	Svc       todoservice.Service
	Auth      gin.HandlerFunc
	RateLimit gin.HandlerFunc
}

func (m TodoModule) Name() string { return "todo" }

func (m TodoModule) Register(v1 *gin.RouterGroup) error {
	if err := requireAuthedDeps("todo", m.Svc, m.Auth, m.RateLimit); err != nil {
		return err
	}
	h := NewUserTodoHandler(m.Svc)
	g := v1.Group("/user", m.Auth, m.RateLimit).Group("/todos")
	g.GET("", h.List)
	g.POST("", h.Create)
	g.GET("/:id", h.Get)
	g.PATCH("/:id", h.Update)
	g.DELETE("/:id", h.Delete)
	return nil
}

// AdminModule 注册 /admin（后台用户管理，需登录 + admin 角色 + 用户级限流）。
type AdminModule struct {
	Svc       adminservice.Service
	Auth      gin.HandlerFunc
	RateLimit gin.HandlerFunc
}

func (m AdminModule) Name() string { return "admin" }

func (m AdminModule) Register(v1 *gin.RouterGroup) error {
	if err := requireAuthedDeps("admin", m.Svc, m.Auth, m.RateLimit); err != nil {
		return err
	}
	h := NewAdminUserHandler(m.Svc)
	rg := v1.Group("/admin", m.Auth, m.RateLimit, middleware.RequireRole(repo.RoleAdmin))
	rg.GET("/ping", func(c *gin.Context) {
		resp.OK(c, gin.H{"status": "ok", "scope": repo.RoleAdmin})
	})
	users := rg.Group("/users")
	users.GET("", h.List)
	users.GET("/:id", h.Get)
	users.PATCH("/:id/role", h.UpdateRole)
	users.PATCH("/:id/status", h.SetStatus)
	return nil
}

// requireAuthedDeps 校验「需登录模块」的公共依赖，错误信息带上模块名便于定位。
func requireAuthedDeps(name string, svc any, auth, rateLimit gin.HandlerFunc) error {
	if svc == nil {
		return errors.New(name + " module: service is required")
	}
	if auth == nil || rateLimit == nil {
		return errors.New(name + " module: auth/rate limit middleware is required")
	}
	return nil
}
