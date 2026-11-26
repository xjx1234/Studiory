package http

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"backend/internal/config"
	"backend/pkg/errcode"

	"github.com/gin-gonic/gin"
)

// readyPayload 是 /ready 响应 data 字段的解码结构。
type readyPayload struct {
	Status string             `json:"status"`
	Checks []readyCheckResult `json:"checks"`
}

// newReadyRouter 构造只关心 /ready 的最小路由；限流中间件用 no-op 替身，
// 避免探活用例依赖 Redis。
func newReadyRouter(t *testing.T, checks []ReadyCheck) *gin.Engine {
	t.Helper()

	deps := &Deps{
		Cfg: &config.Config{
			AppEnv:             "test",
			CORSAllowOrigins:   []string{"*"},
			RateLimitPerMinute: 1000,
		},
		RateLimitMiddleware: func(c *gin.Context) { c.Next() },
		Modules: newTestModules(
			&fakeAuthService{}, &fakeUserService{}, &fakeTodoService{}, &fakeAdminService{},
			fakeAuthMiddleware, func(c *gin.Context) { c.Next() },
		),
		ReadyChecks: checks,
	}

	r, err := NewRouter(deps)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}
	return r
}

// TestReadyReportsPerCheckResults 依赖故障时返回 503，但要逐项给出结果，
// 且不能把底层错误（连接串、内网地址）回显给未鉴权的探活端点。
func TestReadyReportsPerCheckResults(t *testing.T) {
	r := newReadyRouter(t, []ReadyCheck{
		{Name: "PostgreSQL", Check: func(context.Context) error { return nil }},
		{Name: "Redis", Check: func(context.Context) error {
			return errors.New("dial tcp 10.0.0.5:6379: connect: connection refused")
		}},
	})

	w, body := doJSON(t, r, "GET", "/ready", nil)
	assertStatusCode(t, w, body, 503, errcode.ErrServiceUnavailable.Code)

	var payload readyPayload
	if err := json.Unmarshal(body.Data, &payload); err != nil {
		t.Fatalf("unmarshal ready data: %v (raw=%s)", err, body.Data)
	}
	if payload.Status != "unready" {
		t.Errorf("status = %q, want unready", payload.Status)
	}
	if len(payload.Checks) != 2 {
		t.Fatalf("checks = %d, want 2", len(payload.Checks))
	}
	if !payload.Checks[0].OK || payload.Checks[0].Name != "PostgreSQL" {
		t.Errorf("check[0] = %+v, want PostgreSQL ok", payload.Checks[0])
	}
	if payload.Checks[1].OK || payload.Checks[1].Name != "Redis" || payload.Checks[1].Error == "" {
		t.Errorf("check[1] = %+v, want Redis failed with reason", payload.Checks[1])
	}
	if strings.Contains(payload.Checks[1].Error, "connection refused") {
		t.Errorf("internal error detail leaked to /ready: %q", payload.Checks[1].Error)
	}
}

func TestReadyAllHealthy(t *testing.T) {
	r := newReadyRouter(t, []ReadyCheck{
		{Name: "PostgreSQL", Check: func(context.Context) error { return nil }},
		{Name: "Redis", Check: func(context.Context) error { return nil }},
	})

	w, body := doJSON(t, r, "GET", "/ready", nil)
	assertStatusCode(t, w, body, 200, 0)

	var payload readyPayload
	if err := json.Unmarshal(body.Data, &payload); err != nil {
		t.Fatalf("unmarshal ready data: %v", err)
	}
	if payload.Status != "ready" || len(payload.Checks) != 2 {
		t.Fatalf("payload = %+v, want ready with 2 checks", payload)
	}
	for _, check := range payload.Checks {
		if !check.OK {
			t.Errorf("check %q reported not ok", check.Name)
		}
	}
}

// TestReadyRunsChecksConcurrently 两个探针都必须等到「对方也启动」才返回：
// 串行执行时第一个探针会一直等到 2s 预算耗尽而失败，响应将是 503。
func TestReadyRunsChecksConcurrently(t *testing.T) {
	var mu sync.Mutex
	started := 0
	bothStarted := make(chan struct{})

	check := func(ctx context.Context) error {
		mu.Lock()
		started++
		if started == 2 {
			close(bothStarted)
		}
		mu.Unlock()

		select {
		case <-bothStarted:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	r := newReadyRouter(t, []ReadyCheck{
		{Name: "a", Check: check},
		{Name: "b", Check: check},
	})

	w, body := doJSON(t, r, "GET", "/ready", nil)
	assertStatusCode(t, w, body, 200, 0)
}
