package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"backend/pkg/errcode"

	"github.com/gin-gonic/gin"
)

func newSafeRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Safe())
	r.POST("/echo", func(c *gin.Context) {
		var body struct {
			Data string `json:"data"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			_ = c.Error(err)
			return
		}
		c.Status(http.StatusOK)
	})
	return r
}

func TestSafe_AllowsSmallBody(t *testing.T) {
	r := newSafeRouter()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader(`{"data":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
}

func TestSafe_RejectsOversizedBody(t *testing.T) {
	r := newSafeRouter()

	huge := bytes.Repeat([]byte("a"), (2<<20)+1024) // 超过 defaultMaxBodyBytes（2MB）
	payload := `{"data":"` + string(huge) + `"}`

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != errcode.ErrBadRequest.HTTPStatus {
		t.Fatalf("status = %d, want %d", w.Code, errcode.ErrBadRequest.HTTPStatus)
	}

	var resp struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, w.Body.String())
	}
	if resp.Code != errcode.ErrBadRequest.Code {
		t.Errorf("resp.Code = %d, want %d", resp.Code, errcode.ErrBadRequest.Code)
	}
}

func TestSafe_IgnoresUnrelatedErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Safe())
	r.GET("/boom", func(c *gin.Context) {
		_ = c.Error(errUnrelated{})
		c.Status(http.StatusTeapot)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	r.ServeHTTP(w, req)

	// Safe() 只处理 MaxBytesError，其他错误应该原样放行，不覆盖已经写入的状态码。
	if w.Code != http.StatusTeapot {
		t.Fatalf("status = %d, want %d (Safe should not swallow unrelated errors)", w.Code, http.StatusTeapot)
	}
}

type errUnrelated struct{}

func (errUnrelated) Error() string { return "unrelated error" }
