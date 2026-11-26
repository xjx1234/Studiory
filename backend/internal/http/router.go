package http

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"time"

	"backend/internal/http/middleware"

	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewRouter 构建 Gin 路由：先挂全局中间件，再逐个注册 deps.Modules。
//
// 中间件顺序：RequestID → … → Safe → Timeout → CORS → RateLimit(IP)
// CORS 必须在限流之前：429/503 也是浏览器要读的响应，缺了 Access-Control-Allow-Origin
// 前端只会看到 CORS 报错，看不到「请求过于频繁」或「服务不可用」。
// 预检 OPTIONS 由 CORS 直接 204 结束，不会被计入 IP 限流。
// 已鉴权路由由各模块自行挂载 Auth / UserRateLimit，见 module.go。
func NewRouter(deps *Deps) (*gin.Engine, error) {
	if err := validateDeps(deps); err != nil {
		return nil, err
	}

	r := gin.New()
	// Gin 出厂默认信任所有代理（0.0.0.0/0），客户端只要带上 X-Forwarded-For 就能自定义
	// 限流分桶键，IP 限流形同虚设。这里改成按配置收紧：trusted_proxies 为空时不信任任何
	// 代理，XFF 被完全忽略，只认 TCP 对端地址；部署在 nginx/SLB 之后才填入代理网段。
	if err := r.SetTrustedProxies(deps.Cfg.ServerTrustedProxies); err != nil {
		return nil, fmt.Errorf("invalid server.trusted_proxies: %w", err)
	}
	if deps.Cfg.IsProd() && len(deps.Cfg.ServerTrustedProxies) == 0 {
		zap.L().Warn("server.trusted_proxies 为空：忽略 X-Forwarded-For，限流按 TCP 对端地址计数；" +
			"若部署在反向代理/LB 之后，所有用户会共享同一个 IP 限流桶，请配置代理网段")
	}

	r.Use(middleware.RequestID())
	if deps.MetricsMiddleware != nil {
		r.Use(deps.MetricsMiddleware)
	}
	r.Use(middleware.SecurityHeaders(deps.Cfg.IsProd()))
	r.Use(middleware.I18n())
	r.Use(ginzap.GinzapWithConfig(zap.L(), &ginzap.Config{
		TimeFormat:   time.RFC3339,
		UTC:          true,
		DefaultLevel: zapcore.InfoLevel,
		Context:      middleware.AccessLogFields,
	}))
	r.Use(ginzap.RecoveryWithZap(zap.L(), true))
	r.Use(middleware.Safe())
	// 请求级超时：ctx 透传给 CORS/限流/Auth 以及 handler 内的 DB/Redis 调用。
	// 放在访问日志与 Recovery 之内、业务中间件之外——这样 503 仍会被 ginzap 记录为最终状态码，
	// 同时 CORS 头也能加到超时响应上。
	r.Use(middleware.Timeout(deps.Cfg.ServerRequestTimeout))
	// CORS 先于限流：让 429/503 也带上跨域头；预检在这里直接结束，不占用限流配额。
	r.Use(middleware.CORS(middleware.CORSOptions{
		AllowOrigins:     deps.Cfg.CORSAllowOrigins,
		AllowCredentials: deps.Cfg.CORSAllowCredentials,
	}))
	r.Use(deps.RateLimitMiddleware)

	registerHealthRoutes(r, deps)

	if deps.MetricsHandler != nil {
		handler := gin.WrapH(deps.MetricsHandler)
		if deps.Cfg.MetricsToken != "" {
			handler = metricsBearerAuth(deps.Cfg.MetricsToken, handler)
		}
		r.GET("/metrics", handler)
	}

	v1 := r.Group("/api/v1")
	for _, module := range deps.Modules {
		if err := module.Register(v1); err != nil {
			return nil, fmt.Errorf("%s module: %w", module.Name(), err)
		}
	}

	return r, nil
}

// validateDeps 只校验路由器自身依赖；各模块的 service / 中间件由 Module.Register 自行校验，
// 缺依赖时同样在启动阶段返回错误。
func validateDeps(deps *Deps) error {
	if deps == nil {
		return errors.New("http deps is nil")
	}
	if deps.Cfg == nil {
		return errors.New("cfg is required")
	}
	if deps.RateLimitMiddleware == nil {
		return errors.New("RateLimitMiddleware is required")
	}
	if len(deps.ReadyChecks) == 0 {
		return errors.New("ReadyChecks is required")
	}
	if len(deps.Modules) == 0 {
		return errors.New("modules is required")
	}
	return nil
}

// metricsBearerAuth 校验 /metrics 请求的 Bearer token。
// 对应 Prometheus scrape_config 的 bearer_token 字段（发送 Authorization: Bearer <token>）。
// 不支持 query 参数传递：URL 中的 token 会随访问日志落盘，违背密钥不落日志原则。
func metricsBearerAuth(token string, next gin.HandlerFunc) gin.HandlerFunc {
	expected := []byte("Bearer " + token)
	return func(c *gin.Context) {
		auth := []byte(c.GetHeader("Authorization"))
		// 常数时间比较，避免通过响应时延逐字符猜解 token
		if subtle.ConstantTimeCompare(auth, expected) != 1 {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		next(c)
	}
}
