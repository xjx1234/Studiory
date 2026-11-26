package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"backend/pkg/errcode"
	"backend/pkg/resp"

	"github.com/gin-gonic/gin"
)

// TestTimeout_MapsDeadlineTo503 验证两件事：
//  1. 请求 ctx 会在超时后取消，阻塞在 ctx 上的 handler 能及时退出；
//  2. handler 把底层错误当作 ErrInternal 返回时，resp.Fail 会统一改写为 503。
func TestTimeout_MapsDeadlineTo503(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(RequestID(), Timeout(20*time.Millisecond))
	r.GET("/slow", func(c *gin.Context) {
		<-c.Request.Context().Done()
		resp.Fail(c, errcode.ErrInternal)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/slow", nil))

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 after request timeout, got %d body=%s", w.Code, w.Body.String())
	}
}

// TestTimeout_NoopWhenDisabled 单测/未配置场景下 d<=0 不应改变请求行为。
func TestTimeout_NoopWhenDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(Timeout(0))
	r.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ping", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// seenBudget 读取 handler 看到的 ctx 剩余预算；ok=false 表示压根没有 deadline。
func seenBudget(t *testing.T, c *gin.Context) time.Duration {
	t.Helper()
	d, ok := c.Request.Context().Deadline()
	if !ok {
		t.Error("handler 看到的 ctx 没有 deadline，中间件未生效")
		return 0
	}
	return time.Until(d)
}

// TestTimeout_RouteBudgetExtends 验证按路由模板放宽预算：
// 全局 20ms 之外的慢端点应拿到 500ms，能睡过 60ms 仍返回 200；未覆盖的路由不受影响。
func TestTimeout_RouteBudgetExtends(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const (
		global   = 20 * time.Millisecond
		override = 500 * time.Millisecond
	)

	var slowBudget, plainBudget time.Duration
	r := gin.New()
	r.Use(Timeout(global, RouteBudget{Path: "/slow", Budget: override}))
	r.GET("/slow", func(c *gin.Context) {
		slowBudget = seenBudget(t, c)
		time.Sleep(3 * global) // 只有预算真被放宽才跨得过全局值
		c.Status(http.StatusOK)
	})
	r.GET("/plain", func(c *gin.Context) {
		plainBudget = seenBudget(t, c)
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/slow", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("覆盖路由应在放宽后的预算内完成，got %d body=%s", w.Code, w.Body.String())
	}
	if slowBudget < override-time.Second/2 {
		t.Errorf("覆盖路由预算 = %s，期望接近 %s", slowBudget, override)
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/plain", nil))
	if plainBudget <= 0 || plainBudget >= global {
		t.Errorf("未覆盖路由预算 = %s，期望接近 %s（放宽外溢或预算丢失）", plainBudget, global)
	}
}

// TestTimeout_RouteBudgetZeroIsIgnored Budget<=0 的覆盖条目沿用全局预算，
// 避免「配了个 0」变成该路由完全不受超时保护。
func TestTimeout_RouteBudgetZeroIsIgnored(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(Timeout(20*time.Millisecond, RouteBudget{Path: "/slow", Budget: 0}))
	r.GET("/slow", func(c *gin.Context) {
		if got := seenBudget(t, c); got >= 20*time.Millisecond {
			t.Errorf("被忽略的零预算覆盖不应改变行为，实际预算 = %s", got)
		}
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/slow", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("got %d body=%s", w.Code, w.Body.String())
	}
}

// TestTimeout_NestedLongerBudgetCannotExtend 把这条教训钉成测试：
// 在更短的外层 deadline 之内再挂一层更长的 Timeout，生效的仍是外层值。
// 也就是说「按路由放宽」必须发生在本中间件内部（建 ctx 之前），嵌套实现是无效的。
func TestTimeout_NestedLongerBudgetCannotExtend(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const (
		outer = 20 * time.Millisecond
		inner = time.Second
	)

	var got time.Duration
	r := gin.New()
	r.Use(Timeout(outer))
	r.GET("/nested", Timeout(inner), func(c *gin.Context) {
		got = seenBudget(t, c)
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/nested", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("got %d body=%s", w.Code, w.Body.String())
	}
	if got > outer+outer/2 {
		t.Errorf("嵌套更长的预算居然生效了：%s > %s", got, outer)
	}
}
