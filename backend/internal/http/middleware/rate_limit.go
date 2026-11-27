package middleware

import (
	"errors"
	"strings"
	"time"

	"backend/pkg/errcode"
	"backend/pkg/resp"

	"github.com/gin-gonic/gin"
	"github.com/ulule/limiter/v3"
	ginmiddleware "github.com/ulule/limiter/v3/drivers/middleware/gin"
	memory "github.com/ulule/limiter/v3/drivers/store/memory"
	limiteredis "github.com/ulule/limiter/v3/drivers/store/redis"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// RateLimitFallbackObserver 接收限流存储降级事件（由 metrics 实现，可为 nil）。
type RateLimitFallbackObserver interface {
	ObserveRateLimitFallback(scope string)
}

type rateLimitOptions struct {
	logger   *zap.Logger
	observer RateLimitFallbackObserver
}

// RateLimitOption 配置限流中间件的可观测行为。
type RateLimitOption func(*rateLimitOptions)

// WithRateLimitLogger 注入日志器：Redis 存储不可用时用它记录 Error。
func WithRateLimitLogger(l *zap.Logger) RateLimitOption {
	return func(o *rateLimitOptions) {
		if l != nil {
			o.logger = l
		}
	}
}

// WithRateLimitFallbackObserver 注入降级指标观察者。
func WithRateLimitFallbackObserver(observer RateLimitFallbackObserver) RateLimitOption {
	return func(o *rateLimitOptions) {
		if observer != nil {
			o.observer = observer
		}
	}
}

// RateLimit 对未登录/公开路由按客户端 IP 限流。
//
// /api/v1/user 与 /api/v1/admin 由 RateLimitByUser 单独按 user_id 限流，此处跳过以免同 NAT 误伤。
// /health、/ready、/metrics 是探针与监控端点，同样跳过：LB 高频探活不该被 429 干扰，
// 更重要的是 /ready 必须真实反映依赖状态，不能被限流层（也依赖 Redis）短路掉。
// rdb 非空时使用 Redis 分布式 store，否则回退进程内 memory store。
func RateLimit(perMinute int, rdb redis.UniversalClient, redisKeyPrefix string, opts ...RateLimitOption) gin.HandlerFunc {
	return newRateLimiter(perMinute, rdb, redisKeyPrefix, "ip", func(c *gin.Context) string {
		if isInfraPath(c.Request.URL.Path) || isUserScopedAPIPath(c.Request.URL.Path) {
			return ""
		}
		return c.ClientIP()
	}, opts...)
}

// isInfraPath 判断是否为不参与限流的基础设施端点。
func isInfraPath(path string) bool {
	switch path {
	case "/health", "/ready", "/metrics":
		return true
	default:
		return false
	}
}

// RateLimitByUser 对已鉴权路由按 user_id 限流，应挂在 Auth 中间件之后。
func RateLimitByUser(perMinute int, rdb redis.UniversalClient, redisKeyPrefix string, opts ...RateLimitOption) gin.HandlerFunc {
	return newRateLimiter(perMinute, rdb, redisKeyPrefix, "uid", func(c *gin.Context) string {
		raw, exists := c.Get(ContextKeyUserID)
		if !exists {
			return ""
		}
		id, ok := raw.(string)
		if !ok || id == "" {
			return ""
		}
		return id
	}, opts...)
}

// AuthRateLimit 对登录/注册按客户端 IP 限流，配额独立于公开路由的公共桶（scope=auth）。
//
// 为什么要单独一层：这两个端点每次请求都要做一次 bcrypt（cost 12 约 200–400ms），
// 而公共桶按 60–120/min/IP 放行，等于允许单 IP 每分钟打满数秒 CPU——撞库和资源放大
// 都卡不住。这里用更小的额度单独计数，且登录与注册共用一个桶，
// 避免在两端的之间交替请求把额度翻倍。
// 客户端 IP 取值沿用全局配置的 trusted_proxies：未配置时只认 TCP 对端，XFF 一律忽略。
func AuthRateLimit(perMinute int, rdb redis.UniversalClient, redisKeyPrefix string, opts ...RateLimitOption) gin.HandlerFunc {
	return newRateLimiter(perMinute, rdb, redisKeyPrefix, "auth", func(c *gin.Context) string {
		return c.ClientIP()
	}, opts...)
}

func isUserScopedAPIPath(path string) bool {
	return strings.HasPrefix(path, "/api/v1/user") || strings.HasPrefix(path, "/api/v1/admin")
}

func newRateLimiter(
	perMinute int,
	rdb redis.UniversalClient,
	redisKeyPrefix, scope string,
	keyGetter ginmiddleware.KeyGetter,
	opts ...RateLimitOption,
) gin.HandlerFunc {
	if perMinute <= 0 {
		perMinute = 120
	}
	rate := limiter.Rate{
		Period: 1 * time.Minute,
		Limit:  int64(perMinute),
	}

	var cfg rateLimitOptions
	for _, opt := range opts {
		opt(&cfg)
	}

	store := newLimiterStore(rdb, redisKeyPrefix, scope, cfg)
	instance := limiter.New(store, rate)

	return ginmiddleware.NewMiddleware(instance,
		ginmiddleware.WithKeyGetter(keyGetter),
		ginmiddleware.WithExcludedKey(func(key string) bool { return key == "" }),
		ginmiddleware.WithLimitReachedHandler(func(c *gin.Context) {
			resp.Fail(c, errcode.ErrTooManyRequests)
		}),
		// limiter 库默认的 OnError 会 panic，被 Recovery 兜成「500 + 空 body」——
		// Redis 抖动时全站接口都会变成无法解析的错误响应。这里换成统一 JSON 信封，
		// 并把原始错误挂到 c.Errors 供访问日志与排障使用。
		ginmiddleware.WithErrorHandler(func(c *gin.Context, err error) {
			_ = c.Error(err) //nolint:errcheck // gin 内部记录用，不影响响应
			resp.Fail(c, errcode.ErrServiceUnavailable)
		}),
	)
}

// newLimiterStore 创建限流存储。
//
// Redis 不可用时退回进程内 memory store 以保可用性，但必须留下可观测信号：
// 多副本部署下每个 Pod 各算一份配额，实际阈值会变成 per_minute × 副本数。
// 修复前这里是完全静默的，线上无从发现。
func newLimiterStore(rdb redis.UniversalClient, redisKeyPrefix, scope string, opts rateLimitOptions) limiter.Store {
	prefix := redisKeyPrefix
	if prefix == "" {
		prefix = "app"
	}

	if rdb != nil {
		s, err := limiteredis.NewStoreWithOptions(rdb, limiter.StoreOptions{
			Prefix:   prefix + ":limiter:" + scope,
			MaxRetry: 3,
		})
		if err == nil && s != nil {
			return s
		}
		if err == nil {
			err = errors.New("limiter redis store returned nil")
		}

		logger := opts.logger
		if logger == nil {
			logger = zap.L()
		}
		logger.Error("限流存储初始化失败，降级为进程内内存限流；多副本部署下实际阈值会放大为 per_minute × 副本数",
			zap.String("scope", scope),
			zap.Error(err),
		)
		if opts.observer != nil {
			opts.observer.ObserveRateLimitFallback(scope)
		}
	}
	return memory.NewStore()
}
