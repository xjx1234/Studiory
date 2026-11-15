package http

import (
	"net/http"
	"strings"
	"testing"
)

// bcrypt 对超过 72 字节的密码返回 ErrPasswordTooLong。校验层必须在校验阶段
// 就拒绝，否则 service 会把它当作内部错误返回 500。
//
// 用 "a1" 重复构造：既满足「≥8 位、含字母和数字」，又能精确控制字节数
// （纯 ASCII，1 字符 = 1 字节）。
var (
	maxLenPassword  = strings.Repeat("a1", 36)       // 72 字节，恰好是 bcrypt 上限
	tooLongPassword = strings.Repeat("a1", 36) + "x" // 73 字节，刚越界
)

func TestRegister_PasswordLengthBoundary(t *testing.T) {
	cases := []struct {
		name     string
		password string
		wantHTTP int
		wantCode int
	}{
		{name: "72 bytes is accepted", password: maxLenPassword, wantHTTP: http.StatusOK, wantCode: 0},
		{name: "73 bytes is rejected with 400", password: tooLongPassword, wantHTTP: http.StatusBadRequest, wantCode: 20002},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := newTestServer(t)
			body := map[string]any{
				"grant_type": "password",
				"phone":      "13800138000",
				"code":       "123456",
				"password":   tc.password,
			}
			w, resp := doJSON(t, r, http.MethodPost, "/api/v1/auth/register", body)
			assertStatusCode(t, w, resp, tc.wantHTTP, tc.wantCode)
		})
	}
}

func TestChangePassword_PasswordLengthBoundary(t *testing.T) {
	cases := []struct {
		name     string
		password string
		wantHTTP int
		wantCode int
	}{
		{name: "72 bytes is accepted", password: maxLenPassword, wantHTTP: http.StatusOK, wantCode: 0},
		{name: "73 bytes is rejected with 400", password: tooLongPassword, wantHTTP: http.StatusBadRequest, wantCode: 20002},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := newTestServer(t)
			body := map[string]any{
				"old_password": "OldPass123",
				"new_password": tc.password,
			}
			w, resp := doJSON(t, r, http.MethodPatch, "/api/v1/user/password", body, authHeaders("user")...)
			assertStatusCode(t, w, resp, tc.wantHTTP, tc.wantCode)
		})
	}
}
