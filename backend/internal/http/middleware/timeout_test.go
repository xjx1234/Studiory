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
