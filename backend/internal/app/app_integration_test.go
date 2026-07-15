//go:build integration

// 集成测试：用 testcontainers 起一个临时 PostgreSQL + miniredis 验证 app.New() 的
// 完整启动流程（PG/Redis 连接 → Repo/Service 装配 → 路由构建）与健康检查端到端行为。
//
// 运行方式（需要本机有 Docker）：
//
//	go test -tags=integration ./internal/app/...
//
// 不带 -tags=integration 时本文件不参与编译，普通单测与 CI 主测试任务不受影响。
package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"backend/internal/config"
	"backend/internal/testutil/integration"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func integrationTestConfig(databaseURL, redisURL string) *config.Config {
	return &config.Config{
		AppEnv:                  "test",
		ServerAddr:              ":0",
		ServerReadHeaderTimeout: 5 * time.Second,
		ServerReadTimeout:       15 * time.Second,
		ServerWriteTimeout:      30 * time.Second,
		ServerIdleTimeout:       120 * time.Second,
		DatabaseURL:             databaseURL,
		RedisURL:                redisURL,
		RedisKeyPrefix:          "app-it",
		JWTSecret:               "app-integration-test-secret",
		JWTAccessTokenTTL:       time.Hour,
		JWTRefreshTokenTTL:      168 * time.Hour,
		AuthMockCodeEnabled:     true,
		AuthMultiDeviceEnabled:  true,
		OAuthDevMode:            true,
		OAuthProviders:          []string{"wechat", "apple", "google"},
		LogLevel:                "error",
		LogFormat:               "json",
		RateLimitPerMinute:      1000,
		RateLimitUserPerMinute:  1000,
		MetricsEnabled:          false,
		CORSAllowOrigins:        []string{"http://localhost:5173"},
		CORSAllowCredentials:    true,
	}
}

type readyResponse struct {
	Code int `json:"code"`
}

func doReadyRequest(router *gin.Engine, path string) (*httptest.ResponseRecorder, readyResponse) {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var body readyResponse
	if w.Body.Len() > 0 {
		_ = json.Unmarshal(w.Body.Bytes(), &body)
	}
	return w, body
}

// TestNew_StartupHealthCheckAndShutdown 覆盖完整的启动 → 健康检查 → 优雅关闭链路：
//  1. app.New() 成功装配所有资源（PG/Redis/Router/Server）；
//  2. /health 与 /ready 在依赖健康时返回成功；
//  3. Redis 故障时 /ready 正确探测到并返回 503；
//  4. Shutdown 后 PG/Redis 连接均被释放。
func TestNew_StartupHealthCheckAndShutdown(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := context.Background()

	pgEnv, err := integration.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	defer pgEnv.Close(ctx)

	mr := miniredis.RunT(t)

	cfg := integrationTestConfig(pgEnv.DSN, "redis://"+mr.Addr()+"/0")

	a, err := New(ctx, cfg, zap.NewNop())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if a.PGPool == nil || a.Redis == nil || a.Router == nil || a.Server == nil || a.Store == nil {
		t.Fatalf("expected fully wired App, got %+v", a)
	}

	t.Run("health endpoint reports ok", func(t *testing.T) {
		w, body := doReadyRequest(a.Router, "/health")
		if w.Code != http.StatusOK || body.Code != 0 {
			t.Fatalf("/health: http=%d code=%d body=%s", w.Code, body.Code, w.Body.String())
		}
	})

	t.Run("ready endpoint reports ready when dependencies are healthy", func(t *testing.T) {
		w, body := doReadyRequest(a.Router, "/ready")
		if w.Code != http.StatusOK || body.Code != 0 {
			t.Fatalf("/ready: http=%d code=%d body=%s", w.Code, body.Code, w.Body.String())
		}
	})

	t.Run("ready endpoint reports failure when redis is down", func(t *testing.T) {
		mr.Close()

		w, body := doReadyRequest(a.Router, "/ready")
		if w.Code == http.StatusOK || body.Code == 0 {
			t.Fatalf("/ready: expected failure while redis is down, got http=%d code=%d body=%s",
				w.Code, body.Code, w.Body.String())
		}
	})

	t.Run("shutdown releases postgres and redis connections", func(t *testing.T) {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		a.Shutdown(shutdownCtx)

		if err := a.PGPool.Ping(context.Background()); err == nil {
			t.Fatal("expected postgres pool to be closed after Shutdown")
		}
		if err := a.Redis.Ping(context.Background()).Err(); err == nil {
			t.Fatal("expected redis client to be closed after Shutdown")
		}
	})
}
