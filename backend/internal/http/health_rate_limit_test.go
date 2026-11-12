package http

import (
	"context"
	"testing"

	"backend/internal/config"
	"backend/internal/http/middleware"
	"backend/pkg/errcode"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// newRedisBackedTestRouter 复刻生产链路：全局限流走 Redis store，/ready 里也有 Redis 探活。
//
// 这是 internal/app 那个 Docker 集成用例（TestNew_StartupHealthCheckAndShutdown）的
// 无网络版本——它保证了「Redis 掉线时 /ready 报 503」这条断言在本机 go test 下也能跑。
func newRedisBackedTestRouter(t *testing.T, rdb *redis.Client) *gin.Engine {
	t.Helper()

	deps := &Deps{
		Cfg: &config.Config{
			AppEnv:             "test",
			CORSAllowOrigins:   []string{"*"},
			RateLimitPerMinute: 1000,
		},
		AuthService:             &fakeAuthService{},
		UserService:             &fakeUserService{},
		TodoService:             &fakeTodoService{},
		AdminService:            &fakeAdminService{},
		AuthMiddleware:          fakeAuthMiddleware,
		RateLimitMiddleware:     middleware.RateLimit(1000, rdb, "test"),
		UserRateLimitMiddleware: func(c *gin.Context) { c.Next() },
		ReadyChecks: []ReadyCheck{
			{Name: "PostgreSQL", Check: func(context.Context) error { return nil }},
			{Name: "Redis", Check: func(ctx context.Context) error { return rdb.Ping(ctx).Err() }},
		},
	}

	r, err := NewRouter(deps)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}
	return r
}

// TestReadyEndpointWhenRedisDown Redis 掉线后 /ready 必须给出统一 JSON 的 503。
//
// 回归点：限流中间件同样依赖 Redis，limiter 库默认 OnError 会 panic，
// 被 Recovery 兜成「500 + 空 body」——LB 只看状态码会把「依赖故障」误判成「应用崩溃」，
// 客户端也拿不到 code。/ready 现在既跳过 IP 限流，限流层出错也不再 panic。
func TestReadyEndpointWhenRedisDown(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	r := newRedisBackedTestRouter(t, rdb)

	t.Run("ready is ok while redis is up", func(t *testing.T) {
		w, body := doJSON(t, r, "GET", "/ready", nil)
		assertStatusCode(t, w, body, 200, 0)
	})

	t.Run("health ignores redis", func(t *testing.T) {
		mr.Close()

		w, body := doJSON(t, r, "GET", "/health", nil)
		assertStatusCode(t, w, body, 200, 0)
	})

	t.Run("ready reports 503 with json envelope when redis is down", func(t *testing.T) {
		w, body := doJSON(t, r, "GET", "/ready", nil)
		assertStatusCode(t, w, body, 503, errcode.ErrServiceUnavailable.Code)
	})

	t.Run("business routes degrade to 503 json instead of a panicking limiter", func(t *testing.T) {
		w, body := doJSON(t, r, "POST", "/api/v1/auth/send-code", map[string]string{"phone": "13800000000"})
		assertStatusCode(t, w, body, 503, errcode.ErrServiceUnavailable.Code)
	})
}
