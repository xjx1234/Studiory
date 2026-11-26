package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newCORSRouter(opts CORSOptions) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORS(opts))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

func doCORSPreflight(r *gin.Engine, origin string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", origin)
	req.Header.Set("Access-Control-Request-Method", "GET")
	r.ServeHTTP(w, req)
	return w
}

func TestCORS_AllowsConfiguredOrigin(t *testing.T) {
	r := newCORSRouter(CORSOptions{AllowOrigins: []string{"https://app.example.com"}})

	w := doCORSPreflight(r, "https://app.example.com")

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "https://app.example.com")
	}
}

func TestCORS_RejectsUnconfiguredOrigin(t *testing.T) {
	r := newCORSRouter(CORSOptions{AllowOrigins: []string{"https://app.example.com"}})

	w := doCORSPreflight(r, "https://evil.example.com")

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("expected no Access-Control-Allow-Origin for unconfigured origin, got %q", got)
	}
}

func TestCORS_NoFallbackWhenUnconfigured(t *testing.T) {
	r := newCORSRouter(CORSOptions{})

	w := doCORSPreflight(r, "http://localhost:5173")

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("empty allow_origins must not fall back to dev origins, got %q", got)
	}
}

func TestCORS_ExposesRequestIDHeader(t *testing.T) {
	r := newCORSRouter(CORSOptions{AllowOrigins: []string{"https://app.example.com"}})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://app.example.com")
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Expose-Headers"); !strings.Contains(got, "X-Request-Id") {
		t.Errorf("Access-Control-Expose-Headers = %q, want to contain X-Request-Id", got)
	}
}

func TestCORS_AllowCredentialsHeader(t *testing.T) {
	cases := []struct {
		name             string
		allowCredentials bool
		want             string
	}{
		{"enabled", true, "true"},
		{"disabled", false, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newCORSRouter(CORSOptions{
				AllowOrigins:     []string{"https://app.example.com"},
				AllowCredentials: tc.allowCredentials,
			})

			w := doCORSPreflight(r, "https://app.example.com")

			if got := w.Header().Get("Access-Control-Allow-Credentials"); got != tc.want {
				t.Errorf("Access-Control-Allow-Credentials = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCORS_ActualRequestSucceeds(t *testing.T) {
	r := newCORSRouter(CORSOptions{AllowOrigins: []string{"https://app.example.com"}})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://app.example.com")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "https://app.example.com")
	}
}
