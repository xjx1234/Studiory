package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

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
