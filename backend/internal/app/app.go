package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"backend/internal/auth"
	"backend/internal/config"
	internalhttp "backend/internal/http"
	"backend/internal/http/middleware"
	"backend/internal/metrics"
	"backend/internal/oauth"
	"backend/internal/repo"
	mysqlrepo "backend/internal/repo/mysql"
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
	SQLDB  *sql.DB
	Redis  redis.UniversalClient

	Store repo.Storage

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

	var (
		err     error
		storage repo.Storage
		pool    *pgxpool.Pool
		sqlDB   *sql.DB
	)
	if cfg.DBDriver == "mysql" {
		sqlDB, err = store.NewMySQL(ctx, cfg.DatabaseURL, store.MySQLOptions{
			MaxOpenConns:    cfg.DBMaxConns,
			MaxIdleConns:    cfg.DBMinConns,
			ConnMaxIdleTime: cfg.DBMaxConnIdle,
			ConnMaxLifetime: cfg.DBMaxConnLife,
		}, logger)
		if err == nil {
			storage = mysqlrepo.NewStore(sqlDB)
		}
	} else {
		pool, err = store.NewPostgres(ctx, cfg.DatabaseURL, store.PostgresOptions{
			MaxConns:         cfg.DBMaxConns,
			MinConns:         cfg.DBMinConns,
			MaxConnIdle:      cfg.DBMaxConnIdle,
			MaxConnLife:      cfg.DBMaxConnLife,
			StatementTimeout: cfg.DBStatementTimeout,
		}, logger)
		if err == nil {
			storage = pg.NewStore(pool)
		}
	}
	if err != nil {
		return nil, err
	}

	rdb, err := store.NewRedis(ctx, cfg.RedisURL, store.RedisOptions{
		PoolSize:     cfg.RedisPoolSize,
		ReadTimeout:  cfg.RedisReadTimeout,
		WriteTimeout: cfg.RedisWriteTimeout,
	}, logger)
	if err != nil {
		storage.Close()
		return nil, err
	}

	sessionStore := session.NewStore(rdb, cfg.RedisKeyPrefix, cfg.AuthMultiDeviceEnabled, tokenIssuer.RefreshTokenTTL(), cfg.AuthRedisFailOpen)

	// 可观测：装配 Prometheus 指标（在 app 层注册 collector，保持 handler 不碰基础设施）。
	var (
		metricsMiddleware gin.HandlerFunc
		metricsHandler    http.Handler
		codeObserver      authservice.CodeSendObserver
	)
	var rateLimitObserver middleware.RateLimitFallbackObserver
	if cfg.MetricsEnabled {
		m := metrics.New()
		if pool != nil {
			m.Registerer().MustRegister(metrics.NewPgxPoolCollector(pool))
		}
		if sqlDB != nil {
			m.Registerer().MustRegister(metrics.NewSQLDBCollector(sqlDB))
		}
		m.Registerer().MustRegister(metrics.NewRedisCollector(rdb))
		metricsMiddleware = m.Middleware()
		metricsHandler = m.Handler()
		codeObserver = m
		rateLimitObserver = m
	}

	// 限流存储降级必须可观测：日志用于即时排障，指标用于告警（多副本下阈值会被放大）。
	rateLimitOpts := []middleware.RateLimitOption{middleware.WithRateLimitLogger(logger)}
	if rateLimitObserver != nil {
		rateLimitOpts = append(rateLimitOpts, middleware.WithRateLimitFallbackObserver(rateLimitObserver))
	}

	codeSender, err := buildCodeSender(cfg, logger)
	if err != nil {
		storage.Close()
		_ = rdb.Close()
		return nil, err
	}
	codeChannels, err := resolveCodeChannels(cfg, codeSender, logger)
	if err != nil {
		storage.Close()
		_ = rdb.Close()
		return nil, err
	}

	authSvc := authservice.New(storage.Users(), authservice.NewRedisCacheStore(rdb),
		authservice.WithTokenIssuer(tokenIssuer),
		authservice.WithOAuthRepo(storage.OAuth()),
		authservice.WithUserOAuthTxRunner(storage),
		authservice.WithLogger(logger),
		authservice.WithCodePrefix(cfg.RedisKeyPrefix),
		authservice.WithMockCodeFallback(cfg.AuthMockCodeEnabled),
		authservice.WithCodeSender(codeSender),
		authservice.WithCodeChannels(codeChannels),
		authservice.WithCodeQuota(cfg.AuthCodeDailyLimitPerTarget, cfg.AuthCodeDailyLimitPerIP),
		authservice.WithCodeSendObserver(codeObserver),
		authservice.WithOAuthDevMode(cfg.OAuthDevMode),
		authservice.WithOAuthProviders(cfg.OAuthProviders),
		authservice.WithOAuthVerifier(buildOAuthVerifier(cfg, logger)),
		authservice.WithSessionStore(sessionStore),
		authservice.WithRedisFailOpen(cfg.AuthRedisFailOpen),
		authservice.WithRequireCodeForPasswordRegister(cfg.AuthRequireCodeForPasswordRegister),
	)
	userSvc := userservice.New(storage.Users(),
		userservice.WithLogger(logger),
		userservice.WithRevokeSupport(rdb, cfg.RedisKeyPrefix, tokenIssuer.AccessTokenTTL()),
		userservice.WithSessionStore(sessionStore),
	)
	adminSvc := adminservice.New(storage.Users(),
		adminservice.WithLogger(logger),
		adminservice.WithAuditRepo(storage.Audit()),
		adminservice.WithRevokeSupport(rdb, cfg.RedisKeyPrefix, tokenIssuer.AccessTokenTTL()),
		adminservice.WithSessionStore(sessionStore),
	)
	todoSvc := todoservice.New(storage.Todos(), todoservice.WithLogger(logger))

	authMiddleware := middleware.Auth(tokenIssuer, sessionStore, cfg.AuthRedisFailOpen, logger)
	userRateLimit := middleware.RateLimitByUser(cfg.RateLimitUserPerMinute, rdb, cfg.RedisKeyPrefix, rateLimitOpts...)

	deps := &internalhttp.Deps{
		Cfg:                 cfg,
		Logger:              logger,
		RateLimitMiddleware: middleware.RateLimit(cfg.RateLimitPerMinute, rdb, cfg.RedisKeyPrefix, rateLimitOpts...),
		MetricsMiddleware:   metricsMiddleware,
		MetricsHandler:      metricsHandler,
		ReadyChecks: []internalhttp.ReadyCheck{
			{Name: cfg.DBDriver, Check: databaseReadyCheck(storage)},
			{Name: "Redis", Check: redisReadyCheck(rdb)},
		},
		// 每新增一个业务模块，只需在此追加一项，router 与 Deps 无需改动。
		Modules: []internalhttp.Module{
			internalhttp.AuthModule{Svc: authSvc},
			internalhttp.UserModule{Svc: userSvc, Auth: authMiddleware, RateLimit: userRateLimit},
			internalhttp.TodoModule{Svc: todoSvc, Auth: authMiddleware, RateLimit: userRateLimit},
			internalhttp.AdminModule{Svc: adminSvc, Auth: authMiddleware, RateLimit: userRateLimit},
		},
	}

	router, err := internalhttp.NewRouter(deps)
	if err != nil {
		storage.Close()
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
		SQLDB:  sqlDB,
		Redis:  rdb,
		Store:  storage,
		HTTP:   deps,
		Router: router,
		Server: server,
	}, nil
}

// databaseReadyCheck 构造 /ready 使用的数据库探针。
func databaseReadyCheck(db repo.Storage) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		if db == nil {
			return errors.New("database storage is nil")
		}
		return db.Ping(ctx)
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
func buildCodeSender(cfg *config.Config, logger *zap.Logger) (*sender.Router, error) {
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

// resolveCodeChannels 决定哪些渠道对业务可见。
//
// 语义：
//   - auth.code_channels 为空（默认）：自动按已有 Provider 推导。只接了 SMTP 时 sms 自动禁用，
//     避免「启动成功、运行期请求 sms 得到 500」；同时打 Warn 提醒运维。
//   - auth.code_channels 显式配置：每个渠道都必须有 Provider，否则启动失败（fail fast）。
//
// 这样默认行为不破坏「只配 SMTP」的既有部署，而需要严格约束的部署可以显式声明。
func resolveCodeChannels(cfg *config.Config, router *sender.Router, logger *zap.Logger) ([]string, error) {
	supported := make(map[string]struct{}, len(sender.AllChannels))
	for _, ch := range router.Channels() {
		supported[string(ch)] = struct{}{}
	}

	if len(cfg.AuthCodeChannels) > 0 {
		out := make([]string, 0, len(cfg.AuthCodeChannels))
		seen := make(map[string]struct{}, len(cfg.AuthCodeChannels))
		for _, raw := range cfg.AuthCodeChannels {
			ch := strings.ToLower(strings.TrimSpace(raw))
			if _, dup := seen[ch]; dup {
				continue
			}
			if _, ok := supported[ch]; !ok {
				return nil, fmt.Errorf("auth.code_channels 配置了渠道 %q，但没有可用的 Provider（当前可用：%s）",
					ch, strings.Join(channelNames(router.Channels()), ", "))
			}
			seen[ch] = struct{}{}
			out = append(out, ch)
		}
		return out, nil
	}

	out := channelNames(router.Channels())
	if len(out) == 0 {
		return nil, errors.New("没有任何可用的验证码下发渠道")
	}
	if !containsString(out, string(sender.ChannelSMS)) {
		logger.Warn("短信渠道未配置服务商，已自动禁用 sms；如需启用请接入短信 Provider 或在 auth.code_channels 中显式声明")
	}
	return out, nil
}

func channelNames(channels []sender.Channel) []string {
	out := make([]string, 0, len(channels))
	for _, ch := range channels {
		out = append(out, string(ch))
	}
	return out
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
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
	if a.Store != nil {
		a.Store.Close()
	} else if a.PGPool != nil {
		a.PGPool.Close()
	} else if a.SQLDB != nil {
		_ = a.SQLDB.Close()
	}
	if a.Redis != nil {
		_ = a.Redis.Close()
	}
}
