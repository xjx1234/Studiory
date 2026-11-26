package middleware

import (
	"net/http/httptest"
	"testing"

	"backend/pkg/reqid"

	"github.com/gin-gonic/gin"
)

func newRequestIDRouter(capture *string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestID())
	r.GET("/", func(c *gin.Context) {
		*capture = reqid.FromContext(c.Request.Context())
		c.Status(200)
	})
	return r
}

// TestRequestID_InjectsClientHeaderIntoContext 客户端自带 id 时，ctx 与响应头都用它，
// 这样网关/前端生成的 id 可以贯穿到 service 日志。
func TestRequestID_InjectsClientHeaderIntoContext(t *testing.T) {
	var got string
	r := newRequestIDRouter(&got)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-Id", "client-id")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got != "client-id" {
		t.Errorf("request ctx id = %q, want client-id", got)
	}
	if header := w.Header().Get("X-Request-Id"); header != "client-id" {
		t.Errorf("response X-Request-Id = %q, want client-id", header)
	}
}

// TestRequestID_GeneratesAndPropagatesWhenMissing 无入参时自动生成，
// 且 ctx 与响应头是同一个 id。
func TestRequestID_GeneratesAndPropagatesWhenMissing(t *testing.T) {
	var got string
	r := newRequestIDRouter(&got)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))

	if got == "" {
		t.Fatal("expected generated request id in ctx")
	}
	if header := w.Header().Get("X-Request-Id"); header != got {
		t.Errorf("response X-Request-Id = %q, want %q (same as ctx)", header, got)
	}
}
