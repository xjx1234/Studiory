package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"backend/internal/auth"
	"backend/internal/config"
	"backend/internal/http/middleware"
	"backend/internal/repo"
	adminservice "backend/internal/service/admin"
	todoservice "backend/internal/service/todo"
	userservice "backend/internal/service/user"
	"backend/pkg/errcode"
	"backend/pkg/resp"
	pkgvalidator "backend/pkg/validator"

	"github.com/gin-gonic/gin"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	// 注册自定义校验规则（phone_cn / strong_password 等），否则绑定会 panic。
	pkgvalidator.Init()
	os.Exit(m.Run())
}

// apiResponse 是统一响应体的解码结构。
type apiResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// testServer 持有可在每个用例中覆写的 fake service。
type testServer struct {
	auth  *fakeAuthService
	user  *fakeUserService
	todo  *fakeTodoService
	admin *fakeAdminService
}

// fakeAuthMiddleware 模拟鉴权：读取测试头 X-Test-UserID / X-Test-Role。
// 缺少 userID 时返回 401，模拟未登录。
func fakeAuthMiddleware(c *gin.Context) {
	uid := c.GetHeader("X-Test-UserID")
	if uid == "" {
		resp.Fail(c, errcode.ErrUnauthorized)
		return
	}
	c.Set(middleware.ContextKeyUserID, uid)
	role := c.GetHeader("X-Test-Role")
	if role == "" {
		role = repo.RoleUser
	}
	c.Set(middleware.ContextKeyUserRole, role)
	c.Next()
}

func newTestServer(t *testing.T) (*gin.Engine, *testServer) {
	t.Helper()

	ts := &testServer{
		auth:  &fakeAuthService{},
		user:  &fakeUserService{},
		todo:  &fakeTodoService{},
		admin: &fakeAdminService{},
	}

	deps := &Deps{
		Cfg:                     &config.Config{AppEnv: "test", CORSAllowOrigins: []string{"*"}},
		AuthService:             ts.auth,
		UserService:             ts.user,
		TodoService:             ts.todo,
		AdminService:            ts.admin,
		AuthMiddleware:          fakeAuthMiddleware,
		RateLimitMiddleware:     func(c *gin.Context) { c.Next() },
		UserRateLimitMiddleware: func(c *gin.Context) { c.Next() },
		ReadyChecks: []ReadyCheck{
			{Name: "noop", Check: func(context.Context) error { return nil }},
		},
	}

	r, err := NewRouter(deps)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}
	return r, ts
}

// doJSON 发起一次请求；headers 为可选的额外请求头（key,value 成对）。
func doJSON(t *testing.T, r *gin.Engine, method, path string, body any, headers ...string) (*httptest.ResponseRecorder, apiResponse) {
	t.Helper()

	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var parsed apiResponse
	if w.Body.Len() > 0 {
		if err := json.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
			t.Fatalf("unmarshal response (%s): %v", w.Body.String(), err)
		}
	}
	return w, parsed
}

func assertStatusCode(t *testing.T, w *httptest.ResponseRecorder, body apiResponse, wantHTTP, wantCode int) {
	t.Helper()
	if w.Code != wantHTTP {
		t.Errorf("HTTP status = %d, want %d (body=%s)", w.Code, wantHTTP, w.Body.String())
	}
	if body.Code != wantCode {
		t.Errorf("biz code = %d, want %d (body=%s)", body.Code, wantCode, w.Body.String())
	}
}

const authedUser = "user-1"

func authHeaders(role string) []string {
	return []string{"X-Test-UserID", authedUser, "X-Test-Role", role}
}

// ── 健康检查 ───────────────────────────────────────────────────────────────────

func TestHealth(t *testing.T) {
	r, _ := newTestServer(t)
	w, body := doJSON(t, r, http.MethodGet, "/health", nil)
	assertStatusCode(t, w, body, http.StatusOK, 0)
}

func TestReady(t *testing.T) {
	r, _ := newTestServer(t)
	w, body := doJSON(t, r, http.MethodGet, "/ready", nil)
	assertStatusCode(t, w, body, http.StatusOK, 0)
}

// ── 认证 ───────────────────────────────────────────────────────────────────────

func TestLogin_Success(t *testing.T) {
	r, _ := newTestServer(t)
	w, body := doJSON(t, r, http.MethodPost, "/api/v1/auth/login", gin.H{
		"grant_type": "password",
		"account":    "13800138000",
		"password":   "secret",
	})
	assertStatusCode(t, w, body, http.StatusOK, 0)

	var result auth.LoginResult
	if err := json.Unmarshal(body.Data, &result); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if result.Tokens == nil || result.Tokens.AccessToken == "" {
		t.Errorf("expected access token in response, got %+v", result.Tokens)
	}
}

func TestLogin_ValidationError(t *testing.T) {
	r, _ := newTestServer(t)
	// 缺少必填的 grant_type → 参数校验失败 400 / 20002
	w, body := doJSON(t, r, http.MethodPost, "/api/v1/auth/login", gin.H{
		"account":  "13800138000",
		"password": "secret",
	})
	assertStatusCode(t, w, body, http.StatusBadRequest, errcode.ErrValidation.Code)
}

func TestLogin_InvalidCredentials(t *testing.T) {
	r, ts := newTestServer(t)
	ts.auth.loginFn = func(context.Context, *auth.LoginRequest) (*auth.LoginResult, *errcode.Error) {
		return nil, errcode.ErrInvalidCredentials
	}
	w, body := doJSON(t, r, http.MethodPost, "/api/v1/auth/login", gin.H{
		"grant_type": "password",
		"account":    "13800138000",
		"password":   "wrong",
	})
	assertStatusCode(t, w, body, http.StatusUnauthorized, errcode.ErrInvalidCredentials.Code)
}

func TestLogin_AccountLocked(t *testing.T) {
	r, ts := newTestServer(t)
	ts.auth.loginFn = func(context.Context, *auth.LoginRequest) (*auth.LoginResult, *errcode.Error) {
		return nil, errcode.ErrAccountLocked
	}
	w, body := doJSON(t, r, http.MethodPost, "/api/v1/auth/login", gin.H{
		"grant_type": "password",
		"account":    "13800138000",
		"password":   "wrong",
	})
	assertStatusCode(t, w, body, http.StatusTooManyRequests, errcode.ErrAccountLocked.Code)
}

func TestRegister_Success(t *testing.T) {
	r, _ := newTestServer(t)
	w, body := doJSON(t, r, http.MethodPost, "/api/v1/auth/register", gin.H{
		"grant_type": "password",
		"phone":      "13800138000",
		"password":   "Str0ng!Pass",
	})
	assertStatusCode(t, w, body, http.StatusOK, 0)
}

// ── 用户资料（受保护）─────────────────────────────────────────────────────────

func TestGetProfile_Unauthorized(t *testing.T) {
	r, _ := newTestServer(t)
	// 不带 X-Test-UserID → fake 鉴权返回 401
	w, body := doJSON(t, r, http.MethodGet, "/api/v1/user/profile", nil)
	assertStatusCode(t, w, body, http.StatusUnauthorized, errcode.ErrUnauthorized.Code)
}

func TestGetProfile_Success(t *testing.T) {
	r, _ := newTestServer(t)
	w, body := doJSON(t, r, http.MethodGet, "/api/v1/user/profile", nil, authHeaders(repo.RoleUser)...)
	assertStatusCode(t, w, body, http.StatusOK, 0)

	var profile userservice.ProfileResult
	if err := json.Unmarshal(body.Data, &profile); err != nil {
		t.Fatalf("decode profile: %v", err)
	}
	if profile.ID != authedUser {
		t.Errorf("profile.ID = %q, want %q", profile.ID, authedUser)
	}
}

func TestUpdateProfile_EmptyBody(t *testing.T) {
	r, _ := newTestServer(t)
	// nickname 和 avatar 都为空 → handler 返回 ErrValidation
	w, body := doJSON(t, r, http.MethodPatch, "/api/v1/user/profile", gin.H{}, authHeaders(repo.RoleUser)...)
	assertStatusCode(t, w, body, http.StatusBadRequest, errcode.ErrValidation.Code)
}

func TestChangePassword_WeakNewPassword(t *testing.T) {
	r, _ := newTestServer(t)
	w, body := doJSON(t, r, http.MethodPatch, "/api/v1/user/password", gin.H{
		"old_password": "whatever",
		"new_password": "123",
	}, authHeaders(repo.RoleUser)...)
	assertStatusCode(t, w, body, http.StatusBadRequest, errcode.ErrValidation.Code)
}

func TestChangePassword_WrongOldPassword(t *testing.T) {
	r, ts := newTestServer(t)
	ts.user.changePasswordFn = func(context.Context, string, *userservice.ChangePasswordInput) *errcode.Error {
		return errcode.ErrWrongPassword
	}
	w, body := doJSON(t, r, http.MethodPatch, "/api/v1/user/password", gin.H{
		"old_password": "wrong",
		"new_password": "Str0ng!Pass",
	}, authHeaders(repo.RoleUser)...)
	assertStatusCode(t, w, body, http.StatusUnauthorized, errcode.ErrWrongPassword.Code)
}

// ── Todo（受保护）─────────────────────────────────────────────────────────────

func TestCreateTodo_Success(t *testing.T) {
	r, _ := newTestServer(t)
	w, body := doJSON(t, r, http.MethodPost, "/api/v1/user/todos", gin.H{
		"title": "buy milk",
	}, authHeaders(repo.RoleUser)...)
	assertStatusCode(t, w, body, http.StatusOK, 0)
}

func TestCreateTodo_ValidationError(t *testing.T) {
	r, _ := newTestServer(t)
	// title 为空 → 校验失败
	w, body := doJSON(t, r, http.MethodPost, "/api/v1/user/todos", gin.H{
		"title": "",
	}, authHeaders(repo.RoleUser)...)
	assertStatusCode(t, w, body, http.StatusBadRequest, errcode.ErrValidation.Code)
}

func TestGetTodo_NotFound(t *testing.T) {
	r, ts := newTestServer(t)
	ts.todo.getFn = func(context.Context, string, string) (*todoservice.Item, *errcode.Error) {
		return nil, errcode.ErrNotFound
	}
	w, body := doJSON(t, r, http.MethodGet,
		"/api/v1/user/todos/11111111-1111-1111-1111-111111111111", nil, authHeaders(repo.RoleUser)...)
	assertStatusCode(t, w, body, http.StatusNotFound, errcode.ErrNotFound.Code)
}

func TestGetTodo_InvalidUUID(t *testing.T) {
	r, _ := newTestServer(t)
	// 非法 uuid → uri 绑定校验失败 400
	w, body := doJSON(t, r, http.MethodGet, "/api/v1/user/todos/not-a-uuid", nil, authHeaders(repo.RoleUser)...)
	assertStatusCode(t, w, body, http.StatusBadRequest, errcode.ErrValidation.Code)
}

func TestDeleteTodo_Success(t *testing.T) {
	r, _ := newTestServer(t)
	w, body := doJSON(t, r, http.MethodDelete,
		"/api/v1/user/todos/11111111-1111-1111-1111-111111111111", nil, authHeaders(repo.RoleUser)...)
	assertStatusCode(t, w, body, http.StatusOK, 0)
}

// ── 管理员（受保护 + RequireRole）─────────────────────────────────────────────

func TestAdminPing_AsAdmin(t *testing.T) {
	r, _ := newTestServer(t)
	w, body := doJSON(t, r, http.MethodGet, "/api/v1/admin/ping", nil, authHeaders(repo.RoleAdmin)...)
	assertStatusCode(t, w, body, http.StatusOK, 0)
}

func TestAdminPing_AsUserForbidden(t *testing.T) {
	r, _ := newTestServer(t)
	// 普通用户角色 → RequireRole 拦截 403
	w, body := doJSON(t, r, http.MethodGet, "/api/v1/admin/ping", nil, authHeaders(repo.RoleUser)...)
	assertStatusCode(t, w, body, http.StatusForbidden, errcode.ErrForbidden.Code)
}

func TestAdminPing_Unauthorized(t *testing.T) {
	r, _ := newTestServer(t)
	w, body := doJSON(t, r, http.MethodGet, "/api/v1/admin/ping", nil)
	assertStatusCode(t, w, body, http.StatusUnauthorized, errcode.ErrUnauthorized.Code)
}

// ── 管理员用户管理 ─────────────────────────────────────────────────────────────

func TestAdminListUsers_AsAdmin(t *testing.T) {
	r, _ := newTestServer(t)
	w, body := doJSON(t, r, http.MethodGet, "/api/v1/admin/users?page=1&page_size=10", nil, authHeaders(repo.RoleAdmin)...)
	assertStatusCode(t, w, body, http.StatusOK, 0)
}

func TestAdminListUsers_AsUserForbidden(t *testing.T) {
	r, _ := newTestServer(t)
	w, body := doJSON(t, r, http.MethodGet, "/api/v1/admin/users", nil, authHeaders(repo.RoleUser)...)
	assertStatusCode(t, w, body, http.StatusForbidden, errcode.ErrForbidden.Code)
}

func TestAdminGetUser_NotFound(t *testing.T) {
	r, ts := newTestServer(t)
	ts.admin.getFn = func(context.Context, string) (*adminservice.UserItem, *errcode.Error) {
		return nil, errcode.ErrNotFound
	}
	w, body := doJSON(t, r, http.MethodGet,
		"/api/v1/admin/users/11111111-1111-1111-1111-111111111111", nil, authHeaders(repo.RoleAdmin)...)
	assertStatusCode(t, w, body, http.StatusNotFound, errcode.ErrNotFound.Code)
}

func TestAdminUpdateRole_Success(t *testing.T) {
	r, _ := newTestServer(t)
	w, body := doJSON(t, r, http.MethodPatch,
		"/api/v1/admin/users/11111111-1111-1111-1111-111111111111/role",
		gin.H{"role": "admin"}, authHeaders(repo.RoleAdmin)...)
	assertStatusCode(t, w, body, http.StatusOK, 0)
}

func TestAdminUpdateRole_InvalidRole(t *testing.T) {
	r, _ := newTestServer(t)
	w, body := doJSON(t, r, http.MethodPatch,
		"/api/v1/admin/users/11111111-1111-1111-1111-111111111111/role",
		gin.H{"role": "superuser"}, authHeaders(repo.RoleAdmin)...)
	assertStatusCode(t, w, body, http.StatusBadRequest, errcode.ErrValidation.Code)
}

func TestAdminSetStatus_Success(t *testing.T) {
	r, _ := newTestServer(t)
	w, body := doJSON(t, r, http.MethodPatch,
		"/api/v1/admin/users/11111111-1111-1111-1111-111111111111/status",
		gin.H{"status": "disabled"}, authHeaders(repo.RoleAdmin)...)
	assertStatusCode(t, w, body, http.StatusOK, 0)
}

func TestAdminSetStatus_InvalidStatus(t *testing.T) {
	r, _ := newTestServer(t)
	w, body := doJSON(t, r, http.MethodPatch,
		"/api/v1/admin/users/11111111-1111-1111-1111-111111111111/status",
		gin.H{"status": "frozen"}, authHeaders(repo.RoleAdmin)...)
	assertStatusCode(t, w, body, http.StatusBadRequest, errcode.ErrValidation.Code)
}

func TestAdminSetStatus_CannotDisableSelf(t *testing.T) {
	r, ts := newTestServer(t)
	ts.admin.setStatusFn = func(_ context.Context, acting, target, status string) (*adminservice.UserItem, *errcode.Error) {
		if acting == target && status == repo.StatusDisabled {
			return nil, errcode.ErrCannotModifySelf
		}
		return &adminservice.UserItem{ID: target, Status: status}, nil
	}
	// authedUser = "user-1"，路径里的 id 也用 user-1 触发自我禁用拦截
	w, body := doJSON(t, r, http.MethodPatch,
		"/api/v1/admin/users/11111111-1111-1111-1111-111111111111/status",
		gin.H{"status": "disabled"}, "X-Test-UserID", "11111111-1111-1111-1111-111111111111", "X-Test-Role", repo.RoleAdmin)
	assertStatusCode(t, w, body, http.StatusBadRequest, errcode.ErrCannotModifySelf.Code)
}
