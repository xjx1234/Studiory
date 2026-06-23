package app

import (
	"context"
	"errors"
	"net/http"
	"time"

	"backend/internal/auth"
	"backend/internal/config"
	internalhttp "backend/internal/http"
	"backend/internal/http/middleware"
	"backend/internal/metrics"
	"backend/internal/repo/pg"
	"backend/internal/sender"
	adminservice "backend/internal/service/admin"
	authservice "backend/internal/service/auth"
	todoservice "backend/internal/service/todo"
	userservice "backend/internal/service/user"
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

	deps := &internalhttp.Deps{
		Cfg:    cfg,
		Logger: logger,

		AuthService: authservice.New(pgStore.Users(), rdb,
			authservice.WithTokenIssuer(tokenIssuer),
			authservice.WithOAuthRepo(pgStore.OAuth()),
			authservice.WithUserOAuthTxRunner(pgStore),
			authservice.WithLogger(logger),
			authservice.WithCodePrefix(cfg.RedisKeyPrefix),
			authservice.WithMockCodeFallback(cfg.AuthMockCodeEnabled),
			authservice.WithCodeSender(buildCodeSender(cfg, logger)),
			authservice.WithOAuthDevMode(cfg.OAuthDevMode),
			authservice.WithOAuthProviders(cfg.OAuthProviders),
		),
		UserService: userservice.New(pgStore.Users(),
			userservice.WithLogger(logger),
			userservice.WithRevokeSupport(rdb, cfg.RedisKeyPrefix, tokenIssuer.AccessTokenTTL()),
		),
		AdminService: adminservice.New(pgStore.Users(),
			adminservice.WithLogger(logger),
			adminservice.WithRevokeSupport(rdb, cfg.RedisKeyPrefix, tokenIssuer.AccessTokenTTL()),
		),
		TodoService: todoservice.New(pgStore.Todos(), todoservice.WithLogger(logger)),

		AuthMiddleware:      middleware.Auth(tokenIssuer, rdb, cfg.RedisKeyPrefix, logger),
		RateLimitMiddleware: middleware.RateLimit(cfg.RateLimitPerMinute, rdb, cfg.RedisKeyPrefix),
		MetricsMiddleware:   metricsMiddleware,
		MetricsHandler:      metricsHandler,
		ReadyChecks: []internalhttp.ReadyCheck{
			{
				Name: "PostgreSQL",
				Check: func(ctx context.Context) error {
					if pgStore.Pool() == nil {
						return errors.New("postgres pool is nil")
					}
					return pgStore.Pool().Ping(ctx)
				},
			},
			{
				Name: "Redis",
				Check: func(ctx context.Context) error {
					return rdb.Ping(ctx).Err()
				},
			},
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
		ReadHeaderTimeout: 5 * time.Second,
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

// buildCodeSender 按配置装配验证码下发器。
//
// 接入更多服务商（如阿里云/腾讯云短信）：实现 sender.Provider，按渠道追加到 providers，
// NewRouter 会在同一渠道内按注册顺序做故障转移。
func buildCodeSender(cfg *config.Config, logger *zap.Logger) sender.Sender {
	var providers []sender.Provider

	// 邮件：配置了 SMTP 即启用真实邮件下发
	if cfg.SMTPHost != "" {
		providers = append(providers, sender.NewSMTPProvider(
			cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUsername, cfg.SMTPPassword, cfg.SMTPFrom,
		))
	}

	// 短信：示例脚手架未内置真实运营商，按上方注释接入后追加到 providers。

	// 兜底：开发模式或未配置任何真实 Provider 时，用 mock（仅日志）保证流程可跑通。
	if cfg.AuthMockCodeEnabled || len(providers) == 0 {
		if !cfg.AuthMockCodeEnabled {
			logger.Warn("未配置真实验证码服务商，回退到 mock（仅日志，生产环境请接入真实短信/邮件服务）")
		}
		providers = append(providers, sender.NewMockProvider(logger))
	}

	return sender.NewRouter(logger, providers...)
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
