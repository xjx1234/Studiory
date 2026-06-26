package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
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
