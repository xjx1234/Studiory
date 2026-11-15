package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"backend/internal/http/middleware"

	"github.com/gin-gonic/gin"
)

// TestCORSPrecedesRateLimit 固定两条契约：
//  1. 预检 OPTIONS 由 CORS 直接 204，不占用限流配额；
//  2. 被限流（429）的响应仍然带 Access-Control-Allow-Origin，前端能读到真实错误，
//     而不是只看到一条 CORS 失败。
func TestCORSPrecedesRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const origin = "https://app.example.com"

	deps := newTrustedProxiesDeps(nil)
	deps.Cfg.CORSAllowOrigins = []string{origin}
	deps.Cfg.CORSAllowCredentials = true
	deps.Cfg.RedisKeyPrefix = "cors-order"
	deps.RateLimitMiddleware = middleware.RateLimit(1, nil, "cors-order")

	r, err := NewRouter(deps)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}

	send := func(method string, extraHeaders map[string]string) *httptest.ResponseRecorder {
		t.Helper()
		payload, marshalErr := json.Marshal(map[string]string{"type": "sms", "target": "13800000000"})
		if marshalErr != nil {
			t.Fatalf("marshal body: %v", marshalErr)
		}
		req := httptest.NewRequest(method, "/api/v1/auth/send-code", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", origin)
		for k, v := range extraHeaders {
			req.Header.Set(k, v)
		}
		req.RemoteAddr = "203.0.113.7:4444"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	preflight := send(http.MethodOptions, map[string]string{
		"Access-Control-Request-Method":  http.MethodPost,
		"Access-Control-Request-Headers": "Content-Type",
	})
	if preflight.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", preflight.Code)
	}

	// perMinute=1：如果预检被计一次数，这里就会直接 429。
	first := send(http.MethodPost, nil)
	if first.Code != http.StatusOK {
		t.Fatalf("first request = %d, want 200 (preflight consumed the quota? body=%s)", first.Code, first.Body.String())
	}
	if got := first.Header().Get("Access-Control-Allow-Origin"); got != origin {
		t.Errorf("first response ACAO = %q, want %q", got, origin)
	}

	second := send(http.MethodPost, nil)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second request = %d, want 429", second.Code)
	}
	if got := second.Header().Get("Access-Control-Allow-Origin"); got != origin {
		t.Errorf("429 response must carry CORS header, got ACAO = %q", got)
	}
}
