package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"backend/internal/auth"
	"backend/pkg/errcode"

	"github.com/gin-gonic/gin"
)

// TestOversizedBodyRejectedWithUnifiedEnvelope 端到端验证 Safe + request.Bind 的组合：
// 超过 2 MB 的请求体必须得到 400 / code 20001 / 「请求体过大」文案，
// 且响应体只有一份 JSON（Safe 不能二次写），业务 handler 完全不被调用。
func TestOversizedBodyRejectedWithUnifiedEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r, ts := newTestServer(t)

	called := false
	ts.auth.loginFn = func(context.Context, *auth.LoginRequest) (*auth.LoginResult, *errcode.Error) {
		called = true
		return defaultLoginResult(), nil
	}

	// 用真实 JSON 结构撑大 body：password 字段本身没有长度上限，超限只能来自 body 尺寸。
	big := bytes.Repeat([]byte("a"), (2<<20)+1024)
	payload := gin.H{"grant_type": "password", "account": "13800138000", "password": string(big)}

	w, body := doJSON(t, r, http.MethodPost, "/api/v1/auth/login", payload)
	assertStatusCode(t, w, body, http.StatusBadRequest, errcode.ErrBadRequest.Code)

	if body.Message != "请求体过大，请检查提交的数据" {
		t.Errorf("message = %q, want %q（不得回显内部错误原文）", body.Message, "请求体过大，请检查提交的数据")
	}
	if bytes.Contains(w.Body.Bytes(), []byte("http:")) {
		t.Errorf("response leaks internal error text: %s", firstN(w.Body.String(), 200))
	}
	if called {
		t.Error("handler must not run for an oversized body")
	}
}

// TestBindErrorDoesNotLeakInternalMessage 非法 JSON 的原文（含字符偏移等实现细节）不回显给客户端，
// 客户端只看到固定的「请求参数错误」文案。
func TestBindErrorDoesNotLeakInternalMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r, ts := newTestServer(t)

	ts.auth.loginFn = func(context.Context, *auth.LoginRequest) (*auth.LoginResult, *errcode.Error) {
		t.Error("handler must not run when the body cannot be decoded")
		return defaultLoginResult(), nil
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"grant_type": oops}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", w.Code, firstN(w.Body.String(), 200))
	}
	var parsed apiResponse
	if err := json.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("response must be a single JSON object, got %q: %v", firstN(w.Body.String(), 200), err)
	}
	if parsed.Code != errcode.ErrBadRequest.Code {
		t.Errorf("code = %d, want %d", parsed.Code, errcode.ErrBadRequest.Code)
	}
	if parsed.Message != "请求参数错误" {
		t.Errorf("message = %q, want 固定文案「请求参数错误」", parsed.Message)
	}
}

func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
