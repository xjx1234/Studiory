package request

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"backend/pkg/errcode"
	pkgvalidator "backend/pkg/validator"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
	pkgvalidator.Init()
}

type apiResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type testBody struct {
	Name string `json:"name" binding:"required"`
}

func newContext(method, path string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	var reader *bytes.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	return c, w
}

func decodeResponse(t *testing.T, w *httptest.ResponseRecorder) apiResponse {
	t.Helper()
	var resp apiResponse
	if w.Body.Len() > 0 {
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response body %q: %v", w.Body.String(), err)
		}
	}
	return resp
}

func TestBind_Success(t *testing.T) {
	c, w := newContext(http.MethodPost, "/", []byte(`{"name":"alice"}`))

	var body testBody
	if ok := Bind(c, &body); !ok {
		t.Fatalf("expected Bind to succeed, got response: %s", w.Body.String())
	}
	if body.Name != "alice" {
		t.Errorf("body.Name = %q, want %q", body.Name, "alice")
	}
	if w.Body.Len() != 0 {
		t.Errorf("expected no response body written on success, got: %s", w.Body.String())
	}
}

func TestBind_ValidationFailureWritesFieldErrors(t *testing.T) {
	c, w := newContext(http.MethodPost, "/", []byte(`{"name":""}`))

	var body testBody
	if ok := Bind(c, &body); ok {
		t.Fatal("expected Bind to fail for empty required field")
	}
	if w.Code != http.StatusBadRequest {
		t.Errorf("HTTP status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	resp := decodeResponse(t, w)
	if resp.Code != errcode.ErrValidation.Code {
		t.Errorf("resp.Code = %d, want %d", resp.Code, errcode.ErrValidation.Code)
	}

	var data struct {
		Fields map[string]string `json:"fields"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode data.fields: %v", err)
	}
	if _, ok := data.Fields["name"]; !ok {
		t.Errorf("expected field error for %q, got %v", "name", data.Fields)
	}
}

func TestBind_MalformedJSONReturnsBadRequest(t *testing.T) {
	c, w := newContext(http.MethodPost, "/", []byte(`{not-json`))

	var body testBody
	if ok := Bind(c, &body); ok {
		t.Fatal("expected Bind to fail for malformed JSON")
	}
	if w.Code != errcode.ErrBadRequest.HTTPStatus {
		t.Errorf("HTTP status = %d, want %d", w.Code, errcode.ErrBadRequest.HTTPStatus)
	}

	resp := decodeResponse(t, w)
	if resp.Code != errcode.ErrBadRequest.Code {
		t.Errorf("resp.Code = %d, want %d", resp.Code, errcode.ErrBadRequest.Code)
	}
	// 客户端只拿到固定文案：绑定错误的原文（含 JSON 偏移、类型名等实现细节）不回显。
	if resp.Message != "请求参数错误" {
		t.Errorf("resp.Message = %q, want %q（应为 i18n 固定文案，不回显原始错误）", resp.Message, "请求参数错误")
	}
	if strings.Contains(resp.Message, "invalid character") {
		t.Errorf("resp.Message leaks internal JSON error: %q", resp.Message)
	}
	// 原文必须仍然进 c.Errors，否则线上无法排障。
	if len(c.Errors) != 1 {
		t.Fatalf("len(c.Errors) = %d, want 1（原始错误应记入 c.Errors 供访问日志）", len(c.Errors))
	}
	if !strings.Contains(c.Errors[0].Err.Error(), "invalid character") {
		t.Errorf("c.Errors[0] = %v, want the original JSON syntax error", c.Errors[0].Err)
	}
}

// TestBind_OversizedBodyReturnsBodyTooLarge 请求体超过 Safe 中间件的 MaxBytesReader 限制时，
// 必须给出可本地化的「请求体过大」文案（而不是 err.Error() 原文，也不是第二次写响应）。
func TestBind_OversizedBodyReturnsBodyTooLarge(t *testing.T) {
	c, w := newContext(http.MethodPost, "/", []byte(`{"name":"`+strings.Repeat("a", 4096)+`"}`))
	c.Request.Body = http.MaxBytesReader(w, c.Request.Body, 64)

	var body testBody
	if ok := Bind(c, &body); ok {
		t.Fatal("expected Bind to fail for oversized body")
	}
	if w.Code != errcode.ErrBadRequest.HTTPStatus {
		t.Errorf("HTTP status = %d, want %d", w.Code, errcode.ErrBadRequest.HTTPStatus)
	}

	resp := decodeResponse(t, w)
	if resp.Code != errcode.ErrBadRequest.Code {
		t.Errorf("resp.Code = %d, want %d", resp.Code, errcode.ErrBadRequest.Code)
	}
	if resp.Message != "请求体过大，请检查提交的数据" {
		t.Errorf("resp.Message = %q, want %q", resp.Message, "请求体过大，请检查提交的数据")
	}
	if len(c.Errors) != 1 {
		t.Fatalf("len(c.Errors) = %d, want 1", len(c.Errors))
	}
	var maxBytesErr *http.MaxBytesError
	if !errors.As(c.Errors[0].Err, &maxBytesErr) {
		t.Errorf("c.Errors[0] = %v, want *http.MaxBytesError", c.Errors[0].Err)
	}
}

type testQuery struct {
	Page int `form:"page" binding:"required,min=1"`
}

func TestBindQuery_Success(t *testing.T) {
	c, w := newContext(http.MethodGet, "/?page=2", nil)

	var q testQuery
	if ok := BindQuery(c, &q); !ok {
		t.Fatalf("expected BindQuery to succeed, got response: %s", w.Body.String())
	}
	if q.Page != 2 {
		t.Errorf("q.Page = %d, want 2", q.Page)
	}
}

func TestBindQuery_ValidationFailure(t *testing.T) {
	c, w := newContext(http.MethodGet, "/?page=0", nil)

	var q testQuery
	if ok := BindQuery(c, &q); ok {
		t.Fatal("expected BindQuery to fail for page=0 (min=1)")
	}
	resp := decodeResponse(t, w)
	if resp.Code != errcode.ErrValidation.Code {
		t.Errorf("resp.Code = %d, want %d", resp.Code, errcode.ErrValidation.Code)
	}
}

type testURI struct {
	ID string `uri:"id" binding:"required,uuid"`
}

func TestBindURI_Success(t *testing.T) {
	c, w := newContext(http.MethodGet, "/x/11111111-1111-1111-1111-111111111111", nil)
	c.Params = gin.Params{{Key: "id", Value: "11111111-1111-1111-1111-111111111111"}}

	var u testURI
	if ok := BindURI(c, &u); !ok {
		t.Fatalf("expected BindURI to succeed, got response: %s", w.Body.String())
	}
	if u.ID != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("u.ID = %q", u.ID)
	}
}

func TestBindURI_ValidationFailure(t *testing.T) {
	c, w := newContext(http.MethodGet, "/x/not-a-uuid", nil)
	c.Params = gin.Params{{Key: "id", Value: "not-a-uuid"}}

	var u testURI
	if ok := BindURI(c, &u); ok {
		t.Fatal("expected BindURI to fail for invalid uuid")
	}
	resp := decodeResponse(t, w)
	if resp.Code != errcode.ErrValidation.Code {
		t.Errorf("resp.Code = %d, want %d", resp.Code, errcode.ErrValidation.Code)
	}
}

func TestDetectLang(t *testing.T) {
	cases := []struct {
		name       string
		query      string
		acceptLang string
		want       string
	}{
		{"query param wins", "?lang=en", "zh-CN,zh;q=0.9", "en"},
		{"falls back to Accept-Language header", "", "en-US,en;q=0.8", "en-US"},
		{"defaults to zh when nothing set", "", "", "zh"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newContext(http.MethodGet, "/"+tc.query, nil)
			if tc.acceptLang != "" {
				c.Request.Header.Set("Accept-Language", tc.acceptLang)
			}
			if got := detectLang(c); got != tc.want {
				t.Errorf("detectLang() = %q, want %q", got, tc.want)
			}
		})
	}
}
