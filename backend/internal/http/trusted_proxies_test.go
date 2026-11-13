package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"backend/internal/config"
	"backend/internal/http/middleware"
	"backend/pkg/errcode"

	"github.com/gin-gonic/gin"
)

// TestTrustedProxiesControlsXFF 固定「X-Forwarded-For 只有在 TCP 对端是可信代理时才生效」这条契约。
//
// Gin 出厂默认 trustedProxies = ["0.0.0.0/0", "::/0"]，即信任所有代理：
// 任何人都能在请求头里写任意 XFF，让 c.ClientIP() 返回他挑选的地址，
// 于是按 IP 分桶的限流可以无限开新桶，access log 里的来源地址也能随意伪造。
// NewRouter 现在按 app.trusted_proxies 收紧，默认不信任任何代理。
func TestTrustedProxiesControlsXFF(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("forged xff is ignored when no proxy is trusted", func(t *testing.T) {
		r := newTrustedProxiesTestRouter(t, nil)

		// 同一个 TCP 对端，每次换一个伪造的 XFF —— 期望始终落在同一个限流桶里。
		w, _ := doFromProxy(t, r, "203.0.113.9:5555", "8.8.8.8")
		if w.Code != http.StatusOK {
			t.Fatalf("first request = %d, want 200 (body=%s)", w.Code, w.Body.String())
		}
		w, body := doFromProxy(t, r, "203.0.113.9:5555", "9.9.9.9")
		if w.Code != http.StatusTooManyRequests {
			t.Fatalf("forged XFF bought a fresh rate-limit bucket: status = %d, want 429", w.Code)
		}
		if body.Code != errcode.ErrTooManyRequests.Code {
			t.Errorf("biz code = %d, want %d", body.Code, errcode.ErrTooManyRequests.Code)
		}
	})

	t.Run("xff is honoured when the peer is a configured proxy", func(t *testing.T) {
		r := newTrustedProxiesTestRouter(t, []string{"192.0.2.0/24"})

		// 对端在可信网段内，XFF 最左地址才是真实客户端 —— 不同客户端各占一个桶。
		w, _ := doFromProxy(t, r, "192.0.2.1:40000", "8.8.8.8")
		if w.Code != http.StatusOK {
			t.Fatalf("client A first request = %d, want 200", w.Code)
		}
		w, _ = doFromProxy(t, r, "192.0.2.1:40000", "9.9.9.9")
		if w.Code != http.StatusOK {
			t.Fatalf("client B through trusted proxy = %d, want 200（不能把代理后的用户挤进同一个桶）", w.Code)
		}
		w, _ = doFromProxy(t, r, "192.0.2.1:40000", "8.8.8.8")
		if w.Code != http.StatusTooManyRequests {
			t.Fatalf("client A repeat request = %d, want 429", w.Code)
		}
	})

	t.Run("peer outside the trusted range still cannot use xff", func(t *testing.T) {
		r := newTrustedProxiesTestRouter(t, []string{"192.0.2.0/24"})

		w, _ := doFromProxy(t, r, "198.51.100.7:1", "8.8.8.8")
		if w.Code != http.StatusOK {
			t.Fatalf("first request = %d, want 200", w.Code)
		}
		w, _ = doFromProxy(t, r, "198.51.100.7:1", "9.9.9.9")
		if w.Code != http.StatusTooManyRequests {
			t.Fatalf("untrusted peer forged XFF: status = %d, want 429", w.Code)
		}
	})

	t.Run("invalid proxy entry fails router construction", func(t *testing.T) {
		deps := newTrustedProxiesDeps([]string{"not-a-cidr"})
		if _, err := NewRouter(deps); err == nil {
			t.Fatal("expected NewRouter to reject an invalid trusted proxy entry")
		}
	})
}

// newTrustedProxiesDeps 按 perMinute=1 装配真实 IP 限流中间件（memory store，无外部依赖）：
// 桶是否被 XFF 换掉，一次请求之后就能从状态码上读出来。
func newTrustedProxiesDeps(proxies []string) *Deps {
	return &Deps{
		Cfg: &config.Config{
			AppEnv:               "test",
			CORSAllowOrigins:     []string{"*"},
			RateLimitPerMinute:   1,
			ServerTrustedProxies: proxies,
			RedisKeyPrefix:       "tp-test",
		},
		AuthService:             &fakeAuthService{},
		UserService:             &fakeUserService{},
		TodoService:             &fakeTodoService{},
		AdminService:            &fakeAdminService{},
		AuthMiddleware:          fakeAuthMiddleware,
		RateLimitMiddleware:     middleware.RateLimit(1, nil, "tp-test"),
		UserRateLimitMiddleware: func(c *gin.Context) { c.Next() },
		ReadyChecks: []ReadyCheck{
			{Name: "noop", Check: func(context.Context) error { return nil }},
		},
	}
}

func newTrustedProxiesTestRouter(t *testing.T, proxies []string) *gin.Engine {
	t.Helper()

	r, err := NewRouter(newTrustedProxiesDeps(proxies))
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}
	return r
}

// doFromProxy 打一次 send-code（公开路由，受 IP 限流管辖），可指定 TCP 对端地址与 XFF。
func doFromProxy(t *testing.T, r *gin.Engine, remoteAddr, xff string) (*httptest.ResponseRecorder, apiResponse) {
	t.Helper()

	payload, err := json.Marshal(map[string]string{"type": "sms", "target": "13800000000"})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/send-code", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remoteAddr
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var parsed apiResponse
	if w.Body.Len() > 0 {
		if err := json.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
			t.Fatalf("unmarshal response (%s): %v", w.Body.String(), err)
		}
	}
	return w, parsed
}
