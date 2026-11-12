package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"backend/pkg/errcode"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func TestRateLimitByIP_SkipsUserScopedPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var hits int
	r := gin.New()
	r.Use(RateLimit(2, nil, "test"))
	r.GET("/api/v1/auth/ping", func(c *gin.Context) {
		hits++
		c.Status(http.StatusOK)
	})
	r.GET("/api/v1/user/profile", func(c *gin.Context) {
		hits++
		c.Status(http.StatusOK)
	})

	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/user/profile", nil)
		req.RemoteAddr = "1.2.3.4:1234"
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("user path should skip IP limiter, request %d: %d", i+1, w.Code)
		}
	}
	if hits != 3 {
		t.Fatalf("expected 3 handler hits, got %d", hits)
	}

	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/ping", nil)
		req.RemoteAddr = "1.2.3.4:1234"
		r.ServeHTTP(w, req)
		if i < 2 && w.Code != http.StatusOK {
			t.Fatalf("auth request %d: expected 200, got %d", i+1, w.Code)
		}
		if i == 2 && w.Code != http.StatusTooManyRequests {
			t.Fatalf("auth request 3: expected 429, got %d", w.Code)
		}
	}
}

func TestRateLimitByUser_UsesUserIDKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(ContextKeyUserID, c.GetHeader("X-Test-User"))
		c.Next()
	})
	r.Use(RateLimitByUser(2, nil, "test"))
	r.GET("/api/v1/user/profile", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	do := func(user string) int {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/user/profile", nil)
		req.Header.Set("X-Test-User", user)
		r.ServeHTTP(w, req)
		return w.Code
	}

	if code := do("user-a"); code != http.StatusOK {
		t.Fatalf("user-a first: %d", code)
	}
	if code := do("user-a"); code != http.StatusOK {
		t.Fatalf("user-a second: %d", code)
	}
	if code := do("user-a"); code != http.StatusTooManyRequests {
		t.Fatalf("user-a third: expected 429, got %d", code)
	}
	if code := do("user-b"); code != http.StatusOK {
		t.Fatalf("user-b should have separate bucket: %d", code)
	}
}

func TestRateLimitByUser_SkipsWhenUserIDMissingOrInvalid(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		switch c.GetHeader("X-Case") {
		case "missing":
			// 不设置 ContextKeyUserID
		case "wrong-type":
			c.Set(ContextKeyUserID, 42)
		case "empty":
			c.Set(ContextKeyUserID, "")
		}
		c.Next()
	})
	r.Use(RateLimitByUser(1, nil, "test"))
	r.GET("/api/v1/user/profile", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	for _, tc := range []string{"missing", "wrong-type", "empty"} {
		for i := 0; i < 3; i++ {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/user/profile", nil)
			req.Header.Set("X-Case", tc)
			r.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("case %s request %d: expected skip limiter (200), got %d", tc, i+1, w.Code)
			}
		}
	}
}

func TestRateLimit_DefaultPerMinuteAndRedisStore(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	// perMinute<=0 应回退到默认 120；rdb 非空走 Redis store。
	r := gin.New()
	r.Use(RateLimit(0, rdb, ""))
	r.GET("/api/v1/auth/ping", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/ping", nil)
	req.RemoteAddr = "9.9.9.9:1234"
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with redis-backed limiter, got %d", w.Code)
	}
}

// TestRateLimit_SkipsInfraEndpoints 固定探针/监控端点不参与 IP 限流：
// LB 高频探活不该被 429 干扰，/ready 更要真实反映依赖状态而不是被限流层短路。
func TestRateLimit_SkipsInfraEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var hits int
	handler := func(c *gin.Context) {
		hits++
		c.Status(http.StatusOK)
	}

	r := gin.New()
	r.Use(RateLimit(1, nil, "test"))
	for _, path := range []string{"/health", "/ready", "/metrics", "/api/v1/auth/ping"} {
		r.GET(path, handler)
	}

	do := func(path string) int {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = "1.2.3.4:1234"
		r.ServeHTTP(w, req)
		return w.Code
	}

	for _, path := range []string{"/health", "/ready", "/metrics"} {
		for i := 0; i < 3; i++ {
			if code := do(path); code != http.StatusOK {
				t.Fatalf("%s request %d: infra path should skip IP limiter, got %d", path, i+1, code)
			}
		}
	}

	// 对照组：业务路由仍按 IP 限流（perMinute=1）。
	if code := do("/api/v1/auth/ping"); code != http.StatusOK {
		t.Fatalf("first business request: expected 200, got %d", code)
	}
	if code := do("/api/v1/auth/ping"); code != http.StatusTooManyRequests {
		t.Fatalf("second business request: expected 429, got %d", code)
	}

	// 3 条 infra × 3 次 + 业务首次；被限流的第二次不会进入 handler。
	if want := 10; hits != want {
		t.Fatalf("expected %d handler hits, got %d", want, hits)
	}
}

// TestRateLimit_StoreErrorReturnsJSON503 Redis 掉线时限流中间件必须给出统一 JSON 信封。
// limiter 库默认 OnError 会 panic，被 Recovery 兜成「500 + 空 body」，客户端无法解析。
func TestRateLimit_StoreErrorReturnsJSON503(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	// 故意不挂 Recovery：中间件若 panic 会直接冒出 ServeHTTP，测试即失败。
	r := gin.New()
	r.Use(RateLimit(100, rdb, "test"))
	r.GET("/api/v1/auth/ping", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	do := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/ping", nil)
		req.RemoteAddr = "5.6.7.8:1234"
		r.ServeHTTP(w, req)
		return w
	}

	// 建立 Redis store（构造期预加载 Lua 脚本需要 Redis 在线）。
	if w := do(); w.Code != http.StatusOK {
		t.Fatalf("request with healthy redis: expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	mr.Close()

	w := do()
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("redis down: expected 503, got %d body=%s", w.Code, w.Body.String())
	}

	var body struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("redis down: body is not json: %v body=%s", err, w.Body.String())
	}
	if body.Code != errcode.ErrServiceUnavailable.Code {
		t.Fatalf("redis down: expected code %d, got %d body=%s",
			errcode.ErrServiceUnavailable.Code, body.Code, w.Body.String())
	}
}
