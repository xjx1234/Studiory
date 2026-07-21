package app

import (
	"context"
	"errors"
	"net/http"

	"backend/internal/auth"
	"backend/internal/config"
	internalhttp "backend/internal/http"
	"backend/internal/http/middleware"
	"backend/internal/metrics"
	"backend/internal/oauth"
	"backend/internal/repo/pg"
	"backend/internal/sender"
	adminservice "backend/internal/service/admin"
	authservice "backend/internal/service/auth"
	todoservice "backend/internal/service/todo"
	userservice "backend/internal/service/user"
	"backend/internal/session"
	"backend/internal/store"
	pkgvalidator "backend/pkg/validator"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// App 是进程内的应用容器，负责持有所有需要关闭的资源。
type App struct {
	Cfg    *config.Config
	Logger *zap.Logger

	PGPool *pgxpool.Pool
	Redis  redis.UniversalClient

	Store *pg.Store

	HTTP   *internalhttp.Deps
	Router *gin.Engine
	Server *http.Server
}

// New 装配整个应用：连接 PG/Redis → 创建 Repo → 创建 Service → 构建路由。
func New(ctx context.Context, cfg *config.Config, logger *zap.Logger) (*App, error) {
	pkgvalidator.Init()
	if cfg.IsProd() {
		gin.SetMode(gin.ReleaseMode)
	}

	tokenIssuer := auth.NewTokenIssuer(cfg.JWTSecret, cfg.JWTAccessTokenTTL, cfg.JWTRefreshTokenTTL)

	pool, err := store.NewPostgres(ctx, cfg.DatabaseURL, store.PostgresOptions{
		MaxConns:    cfg.DBMaxConns,
		MinConns:    cfg.DBMinConns,
		MaxConnIdle: cfg.DBMaxConnIdle,
		MaxConnLife: cfg.DBMaxConnLife,
	}, logger)
	if err != nil {
		return nil, err
	}

	rdb, err := store.NewRedis(ctx, cfg.RedisURL, store.RedisOptions{
		PoolSize: cfg.RedisPoolSize,
	}, logger)
	if err != nil {
		pool.Close()
		return nil, err
	}

	pgStore := pg.NewStore(pool)

	sessionStore := session.NewStore(rdb, cfg.RedisKeyPrefix, cfg.AuthMultiDeviceEnabled, tokenIssuer.RefreshTokenTTL())

	// 可观测：装配 Prometheus 指标（在 app 层注册 collector，保持 handler 不碰基础设施）。
	var metricsMiddleware gin.HandlerFunc
	var metricsHandler http.Handler
	if cfg.MetricsEnabled {
		m := metrics.New()
		m.Registerer().MustRegister(
			metrics.NewPgxPoolCollector(pool),
			metrics.NewRedisCollector(rdb),
		)
		metricsMiddleware = m.Middleware()
		metricsHandler = m.Handler()
	}

	codeSender, err := buildCodeSender(cfg, logger)
	if err != nil {
		pool.Close()
		_ = rdb.Close()
		return nil, err
	}

	deps := &internalhttp.Deps{
		Cfg:    cfg,
		Logger: logger,

		AuthService: authservice.New(pgStore.Users(), authservice.NewRedisCacheStore(rdb),
			authservice.WithTokenIssuer(tokenIssuer),
			authservice.WithOAuthRepo(pgStore.OAuth()),
			authservice.WithUserOAuthTxRunner(pgStore),
			authservice.WithLogger(logger),
			authservice.WithCodePrefix(cfg.RedisKeyPrefix),
			authservice.WithMockCodeFallback(cfg.AuthMockCodeEnabled),
			authservice.WithCodeSender(codeSender),
			authservice.WithOAuthDevMode(cfg.OAuthDevMode),
			authservice.WithOAuthProviders(cfg.OAuthProviders),
			authservice.WithOAuthVerifier(buildOAuthVerifier(cfg, logger)),
			authservice.WithSessionStore(sessionStore),
		),
		UserService: userservice.New(pgStore.Users(),
			userservice.WithLogger(logger),
			userservice.WithRevokeSupport(rdb, cfg.RedisKeyPrefix, tokenIssuer.AccessTokenTTL()),
			userservice.WithSessionStore(sessionStore),
		),
		AdminService: adminservice.New(pgStore.Users(),
			adminservice.WithLogger(logger),
			adminservice.WithRevokeSupport(rdb, cfg.RedisKeyPrefix, tokenIssuer.AccessTokenTTL()),
			adminservice.WithSessionStore(sessionStore),
		),
		TodoService: todoservice.New(pgStore.Todos(), todoservice.WithLogger(logger)),

		AuthMiddleware:          middleware.Auth(tokenIssuer, sessionStore, rdb, cfg.RedisKeyPrefix, logger),
		RateLimitMiddleware:     middleware.RateLimit(cfg.RateLimitPerMinute, rdb, cfg.RedisKeyPrefix),
		UserRateLimitMiddleware: middleware.RateLimitByUser(cfg.RateLimitUserPerMinute, rdb, cfg.RedisKeyPrefix),
		MetricsMiddleware:       metricsMiddleware,
		MetricsHandler:          metricsHandler,
		ReadyChecks: []internalhttp.ReadyCheck{
			{Name: "PostgreSQL", Check: postgresReadyCheck(pgStore.Pool())},
			{Name: "Redis", Check: redisReadyCheck(rdb)},
		},
	}

	router, err := internalhttp.NewRouter(deps)
	if err != nil {
		pool.Close()
		_ = rdb.Close()
		return nil, err
	}

	server := &http.Server{
		Addr:              cfg.ServerAddr,
		Handler:           router,
		ReadHeaderTimeout: cfg.ServerReadHeaderTimeout,
		ReadTimeout:       cfg.ServerReadTimeout,
		WriteTimeout:      cfg.ServerWriteTimeout,
		IdleTimeout:       cfg.ServerIdleTimeout,
	}

	return &App{
		Cfg:    cfg,
		Logger: logger,
		PGPool: pool,
		Redis:  rdb,
		Store:  pgStore,
		HTTP:   deps,
		Router: router,
		Server: server,
	}, nil
}

// postgresReadyCheck 构造 /ready 使用的 PostgreSQL 探针：连接池未初始化或 Ping 失败均视为不可用。
func postgresReadyCheck(pool *pgxpool.Pool) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		if pool == nil {
			return errors.New("postgres pool is nil")
		}
		return pool.Ping(ctx)
	}
}

// redisReadyCheck 构造 /ready 使用的 Redis 探针。
func redisReadyCheck(rdb redis.UniversalClient) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		if rdb == nil {
			return errors.New("redis client is nil")
		}
		return rdb.Ping(ctx).Err()
	}
}

// buildCodeSender 按配置装配验证码下发器。
//
// 接入更多服务商（如阿里云/腾讯云短信）：实现 sender.Provider，按渠道追加到 providers，
// NewRouter 会在同一渠道内按注册顺序做故障转移。
//
// 生产环境（APP_ENV=production）必须配置至少一个真实 Provider（SMTP 或短信），
// 否则返回 error 让调用方决定如何处理——避免在装配阶段调用 logger.Fatal
// 导致资源未清理（PG pool、Redis conn 等已创建但无法 Close）。
func buildCodeSender(cfg *config.Config, logger *zap.Logger) (sender.Sender, error) {
	var providers []sender.Provider

	// 邮件：配置了 SMTP 即启用真实邮件下发
	if cfg.SMTPHost != "" {
		providers = append(providers, sender.NewSMTPProvider(
			cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUsername, cfg.SMTPPassword, cfg.SMTPFrom,
		))
	}

	// 短信：示例脚手架未内置真实运营商，按上方注释接入后追加到 providers。

	if cfg.AuthMockCodeEnabled {
		// 开发模式显式启用 mock：追加 MockProvider，验证码固定 123456
		providers = append(providers, sender.NewMockProvider(logger))
	} else if len(providers) == 0 {
		// 未配置任何真实 Provider：开发环境回退 mock（仅日志），生产环境返回 error
		if cfg.IsProd() {
			return nil, errors.New("生产环境未配置真实验证码服务商（SMTP 或短信），请在 .env 中设置 SMTP_* 或接入短信运营商")
		}
		logger.Warn("未配置真实验证码服务商，回退到 mock（仅日志，生产环境请接入真实短信/邮件服务）")
		providers = append(providers, sender.NewMockProvider(logger))
	}

	return sender.NewRouter(logger, providers...), nil
}

// buildOAuthVerifier 按配置装配第三方登录 token 校验器。
//
// 接入更多平台：实现 oauth.Provider 并追加到 providers。
// dev_mode=true 时 Router 允许客户端仅传 open_id 跳过远程校验（本地联调）。
func buildOAuthVerifier(cfg *config.Config, logger *zap.Logger) oauth.Verifier {
	var providers []oauth.Provider

	providers = append(providers,
		oauth.NewWechatProvider(oauth.WechatConfig{AppID: cfg.OAuthWechatAppID}),
		oauth.NewAppleProvider(oauth.AppleConfig{ClientID: cfg.OAuthAppleClientID}),
		oauth.NewGoogleProvider(oauth.GoogleConfig{ClientID: cfg.OAuthGoogleClientID}),
	)

	return oauth.NewRouter(logger, cfg.OAuthDevMode, providers...)
}

// Shutdown 优雅关闭：先停 HTTP Server，再关闭数据库与缓存。
func (a *App) Shutdown(ctx context.Context) {
	if a.Server != nil {
		if err := a.Server.Shutdown(ctx); err != nil {
			a.Logger.Warn("HTTP server shutdown failed", zap.Error(err))
		}
	}
	a.Close()
}

// Close 关闭基础资源（PG、Redis）。
func (a *App) Close() {
	if a.PGPool != nil {
		a.PGPool.Close()
	}
	if a.Redis != nil {
		_ = a.Redis.Close()
	}
}
