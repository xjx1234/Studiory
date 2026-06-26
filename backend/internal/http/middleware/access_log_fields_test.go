package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-contrib/requestid"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap/zapcore"
)

func TestAccessLogFields_RequestIDAndUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/user/profile", nil)
	c.Request.Header.Set("X-Request-Id", "req-abc")

	requestid.New()(c)
	c.Set(ContextKeyUserID, "user-123")

	fields := AccessLogFields(c)
	got := fieldMap(fields)

	if got["request_id"] != "req-abc" {
		t.Fatalf("request_id: got %q", got["request_id"])
	}
	if got["user_id"] != "user-123" {
		t.Fatalf("user_id: got %q", got["user_id"])
	}
}

func TestAccessLogFields_OmitsEmptyUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/health", nil)
	requestid.New()(c)

	fields := AccessLogFields(c)
	got := fieldMap(fields)

	if got["request_id"] == "" {
		t.Fatal("expected request_id")
	}
	if _, ok := got["user_id"]; ok {
		t.Fatal("user_id should be omitted for unauthenticated requests")
	}
}

func fieldMap(fields []zapcore.Field) map[string]string {
	out := make(map[string]string, len(fields))
	enc := zapcore.NewMapObjectEncoder()
	for _, f := range fields {
		f.AddTo(enc)
	}
	for k, v := range enc.Fields {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}
