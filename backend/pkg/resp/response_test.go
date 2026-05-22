package resp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"backend/pkg/errcode"

	"github.com/gin-gonic/gin"
)

func TestOKWritesUnifiedResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	OK(c, gin.H{"foo": "bar"})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body Response
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if body.Code != 0 {
		t.Fatalf("expected code 0, got %d", body.Code)
	}
	if body.Data == nil {
		t.Fatal("expected data not nil")
	}
}

func TestFailWritesErrorResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	Fail(c, errcode.ErrUnauthorized)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}

	var body Response
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if body.Code != errcode.ErrUnauthorized.Code {
		t.Fatalf("expected code %d, got %d", errcode.ErrUnauthorized.Code, body.Code)
	}
	if body.Data != nil {
		t.Fatal("expected data nil on error")
	}
}
