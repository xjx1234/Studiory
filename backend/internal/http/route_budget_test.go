package http

import (
	"context"
	"net/http"
	"testing"
	"time"

	"backend/internal/auth"
	"backend/internal/config"
	"backend/pkg/errcode"

	"github.com/gin-gonic/gin"
)

// budgetDeps 构造只改了请求预算的 Deps，其余与 newTestServer 一致。
// 不复用后者是因为它把 Cfg 钉成「无超时」，而这里要验证预算确实下发到了中间件。
func budgetDeps(t *testing.T, global, sendCode time.Duration) (*gin.Engine, *testServer) {
	t.Helper()

	ts := &testServer{
		auth:  &fakeAuthService{},
		user:  &fakeUserService{},
		todo:  &fakeTodoService{},
		admin: &fakeAdminService{},
	}
	deps := &Deps{
		Cfg: &config.Config{
			AppEnv:                       "test",
			CORSAllowOrigins:             []string{"*"},
			ServerRequestTimeout:         global,
			ServerSendCodeRequestTimeout: sendCode,
		},
		RateLimitMiddleware: func(c *gin.Context) { c.Next() },
		Modules:             newTestModules(ts.auth, ts.user, ts.todo, ts.admin, fakeAuthMiddleware, func(c *gin.Context) { c.Next() }),
		ReadyChecks: []ReadyCheck{
			{Name: "noop", Check: func(context.Context) error { return nil }},
		},
	}

	r, err := NewRouter(deps)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}
	return r, ts
}

// remainingBudget 取 handler 看到的 ctx 剩余预算；读不到 deadline 直接判失败。
func remainingBudget(t *testing.T, ctx context.Context, where string) time.Duration {
	t.Helper()
	d, ok := ctx.Deadline()
	if !ok {
		t.Errorf("%s: handler 看到的 ctx 没有 deadline，请求预算未生效", where)
		return 0
	}
	return time.Until(d)
}

// TestSendCodeRouteBudget 验证两件事，缺一不可：
//  1. 发码路由拿到的是放宽后的预算，而不是被全局预算封顶——若把放宽实现成「在该路由上
//     再挂一层更长的 Timeout」，context.WithTimeout 只会缩短不会延长，这里会读到全局值；
//  2. 其他路由仍按全局预算走，放宽没有外溢。
func TestSendCodeRouteBudget(t *testing.T) {
	const (
		global   = 100 * time.Millisecond
		sendCode = 3 * time.Second
	)

	r, ts := budgetDeps(t, global, sendCode)

	var sendCodeBudget, loginBudget time.Duration
	ts.auth.sendCodeFn = func(ctx context.Context, _, _, _ string) *errcode.Error {
		sendCodeBudget = remainingBudget(t, ctx, "send-code")
		return nil
	}
	ts.auth.loginFn = func(ctx context.Context, _ *auth.LoginRequest) (*auth.LoginResult, *errcode.Error) {
		loginBudget = remainingBudget(t, ctx, "login")
		return defaultLoginResult(), nil
	}

	w, body := doJSON(t, r, http.MethodPost, sendCodeRoute, map[string]string{
		"type": "email", "target": "user@example.com",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("send-code status = %d body=%s", w.Code, body.Message)
	}
	if sendCodeBudget <= 0 {
		t.Fatal("send-code handler 未被调用，用例前置条件失效")
	}
	if sendCodeBudget < sendCode-time.Second {
		t.Errorf("send-code 预算 = %s，期望接近 %s（读到全局 %s 说明放宽没生效）", sendCodeBudget, sendCode, global)
	}

	w, body = doJSON(t, r, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"grant_type": "password", "account": "13800000000", "password": "Secret123!",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", w.Code, body.Message)
	}
	if loginBudget <= 0 {
		t.Fatal("login handler 未被调用，用例前置条件失效")
	}
	if loginBudget >= global {
		t.Errorf("login 预算 = %s，期望不超过全局 %s（放宽外溢到了普通路由）", loginBudget, global)
	}
}

// TestSendCodeRouteMatchesRegistration 盯住 sendCodeRoute 这个写死的路由模板：
// 它就是用来选中放宽预算的那条路由，一旦 AuthModule 改了路径而常量没跟上，
// 发码会安静地退回全局预算，线上表现是慢 SMTP 下 503 复发。
func TestSendCodeRouteMatchesRegistration(t *testing.T) {
	r, _ := newTestServer(t)

	matches := 0
	for _, rt := range r.Routes() {
		if rt.Path != sendCodeRoute {
			continue
		}
		matches++
		if rt.Method != http.MethodPost {
			t.Errorf("路由 %s 注册方法是 %s，期望 POST", sendCodeRoute, rt.Method)
		}
	}
	if matches != 1 {
		t.Fatalf("真实注册表里 %s 出现 %d 次（应为 1）；sendCodeRoute 常量与 AuthModule 注册已漂移，预算会静默失效",
			sendCodeRoute, matches)
	}
}
