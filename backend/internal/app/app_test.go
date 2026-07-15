package app

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"backend/internal/config"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// ── 健康检查探针 ────────────────────────────────────────────────────────────

func TestPostgresReadyCheck_NilPoolReturnsError(t *testing.T) {
	check := postgresReadyCheck(nil)
	if err := check(context.Background()); err == nil {
		t.Fatal("expected error when postgres pool is nil")
	}
}

func TestRedisReadyCheck_NilClientReturnsError(t *testing.T) {
	check := redisReadyCheck(nil)
	if err := check(context.Background()); err == nil {
		t.Fatal("expected error when redis client is nil")
	}
}

func TestRedisReadyCheck_HealthyReturnsNil(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = rdb.Close() }()

	check := redisReadyCheck(rdb)
	if err := check(context.Background()); err != nil {
		t.Fatalf("expected healthy redis to pass ready check, got: %v", err)
	}
}

func TestRedisReadyCheck_UnhealthyReturnsError(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = rdb.Close() }()

	mr.Close() // 模拟 Redis 不可用

	check := redisReadyCheck(rdb)
	if err := check(context.Background()); err == nil {
		t.Fatal("expected error when redis is down")
	}
}

// ── 优雅关闭 ────────────────────────────────────────────────────────────────

func TestApp_Close_NilResourcesDoNotPanic(t *testing.T) {
	a := &App{}
	a.Close() // 不应 panic
}

func TestApp_Shutdown_NilServerStillClosesResources(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	a := &App{Logger: zap.NewNop(), Redis: rdb}
	a.Shutdown(context.Background())

	if err := rdb.Ping(context.Background()).Err(); err == nil {
		t.Fatal("expected redis client to be closed after Shutdown")
	}
}

func TestApp_Shutdown_StopsServerAndClosesRedis(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := &http.Server{Handler: http.NewServeMux()}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(ln) }()

	a := &App{Logger: zap.NewNop(), Redis: rdb, Server: server}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	a.Shutdown(ctx)

	// Serve() 必须以 http.ErrServerClosed 返回，这是 Server 已真正停止监听的权威信号
	// （直接探测端口是否可连接在时序上不稳定，容易产生偶发误报）。
	select {
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("Serve() error = %v, want http.ErrServerClosed", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Serve() to return after Shutdown")
	}

	// Redis 已关闭。
	if err := rdb.Ping(context.Background()).Err(); err == nil {
		t.Fatal("expected redis client to be closed after Shutdown")
	}
}

// TestApp_Shutdown_LogsWarningButStillClosesResourcesOnTimeout 验证：
// 即便 HTTP Server 在 ctx 超时前未能完成关闭（比如还有慢请求在处理），
// Shutdown 也必须记录警告日志，并继续执行 Close() 释放 PG/Redis，
// 不能因为 Server.Shutdown 出错而“卡住”资源不释放。
func TestApp_Shutdown_LogsWarningButStillClosesResourcesOnTimeout(t *testing.T) {
	reqStarted := make(chan struct{})
	unblock := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		close(reqStarted)
		<-unblock
		w.WriteHeader(http.StatusOK)
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := &http.Server{Handler: mux}
	go func() { _ = server.Serve(ln) }()
	defer close(unblock) // 保证测试结束时处理函数一定会退出，Serve 才能正常收尾

	go func() {
		resp, err := http.Get("http://" + ln.Addr().String() + "/")
		if err == nil {
			_ = resp.Body.Close()
		}
	}()
	<-reqStarted // 等待请求真正进入 handler，确保连接处于 active（非 idle）状态

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	core, logs := observer.New(zap.WarnLevel)
	logger := zap.New(core)

	a := &App{Logger: logger, Server: server, Redis: rdb}

	// 传入一个已经过期的 ctx：由于还有一个 active 连接未结束，
	// Server.Shutdown 会在 ctx.Done() 触发时返回 context.DeadlineExceeded。
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	time.Sleep(5 * time.Millisecond)

	a.Shutdown(shutdownCtx)

	if logs.Len() == 0 {
		t.Fatal("expected a warning log when Server.Shutdown times out")
	}
	if err := rdb.Ping(context.Background()).Err(); err == nil {
		t.Fatal("expected redis client to still be closed despite server shutdown timeout")
	}
}

// ── 启动流程（New）─────────────────────────────────────────────────────────

// TestNew_ReturnsErrorWhenPostgresUnreachable 验证启动失败时快速返回错误、不 panic，
// 无需真实数据库：连接一个保证拒绝连接的本地端口即可触发该分支。
func TestNew_ReturnsErrorWhenPostgresUnreachable(t *testing.T) {
	cfg := &config.Config{
		AppEnv:      "test",
		DatabaseURL: "postgres://user:pass@127.0.0.1:1/nonexistent?sslmode=disable&connect_timeout=1",
		RedisURL:    "redis://127.0.0.1:1/0",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	a, err := New(ctx, cfg, zap.NewNop())
	if err == nil {
		t.Fatal("expected error when postgres is unreachable")
	}
	if a != nil {
		t.Fatal("expected nil App on startup failure")
	}
}
