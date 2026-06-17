package metrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newTestRouter(m *Metrics) *gin.Engine {
	r := gin.New()
	r.Use(m.Middleware())
	r.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })
	r.GET("/boom", func(c *gin.Context) { c.String(http.StatusInternalServerError, "boom") })
	r.GET("/metrics", gin.WrapH(m.Handler()))
	return r
}

func scrape(t *testing.T, r *gin.Engine) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("/metrics status = %d, want 200", w.Code)
	}
	return w.Body.String()
}

func do(r *gin.Engine, method, path string) {
	req := httptest.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
}

func TestMiddleware_RecordsRequestCountAndStatus(t *testing.T) {
	m := New()
	r := newTestRouter(m)

	do(r, http.MethodGet, "/ping")
	do(r, http.MethodGet, "/ping")
	do(r, http.MethodGet, "/boom")

	body := scrape(t, r)

	// 路由模板作为 label，状态码区分
	wantPing := `http_requests_total{method="GET",route="/ping",status="200"} 2`
	if !strings.Contains(body, wantPing) {
		t.Errorf("missing or wrong ping counter.\nwant substring: %s\nbody:\n%s", wantPing, body)
	}
	wantBoom := `http_requests_total{method="GET",route="/boom",status="500"} 1`
	if !strings.Contains(body, wantBoom) {
		t.Errorf("missing or wrong boom counter.\nwant substring: %s\nbody:\n%s", wantBoom, body)
	}

	// 直方图存在
	if !strings.Contains(body, "http_request_duration_seconds_bucket") {
		t.Errorf("missing duration histogram in body:\n%s", body)
	}
	// 在途请求 gauge 存在
	if !strings.Contains(body, "http_requests_in_flight") {
		t.Errorf("missing in_flight gauge in body:\n%s", body)
	}
}

func TestMiddleware_UnmatchedRouteLabel(t *testing.T) {
	m := New()
	r := newTestRouter(m)

	// 命中不存在的路由 → route 应归为 "unmatched"，避免高基数
	do(r, http.MethodGet, "/no/such/path/123")

	body := scrape(t, r)
	if !strings.Contains(body, `route="unmatched"`) {
		t.Errorf("expected unmatched route label, body:\n%s", body)
	}
	// 真实路径不应出现在 label 中
	if strings.Contains(body, "/no/such/path/123") {
		t.Errorf("raw path leaked into metrics labels:\n%s", body)
	}
}

func TestMiddleware_SkipsMetricsEndpoint(t *testing.T) {
	m := New()
	r := newTestRouter(m)

	// 抓取 /metrics 自身不应产生 /metrics 的请求计数
	scrape(t, r)
	body := scrape(t, r)
	if strings.Contains(body, `route="/metrics"`) {
		t.Errorf("/metrics should not be self-instrumented:\n%s", body)
	}
}

func TestRedisCollector(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("ping: %v", err)
	}

	m := New()
	m.Registerer().MustRegister(NewRedisCollector(client))
	r := newTestRouter(m)

	body := scrape(t, r)
	for _, want := range []string{
		"redis_pool_total_conns",
		"redis_pool_idle_conns",
		"redis_pool_hits_total",
		"redis_pool_misses_total",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing redis metric %q in body:\n%s", want, body)
		}
	}
}
