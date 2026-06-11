package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSecurityHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		isProd     bool
		wantHSTS   bool
	}{
		{"dev: no HSTS", false, false},
		{"prod: has HSTS", true, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.Use(SecurityHeaders(tc.isProd))
			r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			r.ServeHTTP(w, req)

			h := w.Header()
			assertHeader(t, h, "X-Content-Type-Options", "nosniff")
			assertHeader(t, h, "X-Frame-Options", "DENY")
			assertHeader(t, h, "X-XSS-Protection", "0")
			assertHeader(t, h, "Referrer-Policy", "strict-origin-when-cross-origin")

			hsts := h.Get("Strict-Transport-Security")
			if tc.wantHSTS && hsts == "" {
				t.Error("expected Strict-Transport-Security header in prod, got none")
			}
			if !tc.wantHSTS && hsts != "" {
				t.Errorf("expected no Strict-Transport-Security in dev, got %q", hsts)
			}
		})
	}
}

func assertHeader(t *testing.T, h http.Header, key, want string) {
	t.Helper()
	if got := h.Get(key); got != want {
		t.Errorf("%s: want %q, got %q", key, want, got)
	}
}
