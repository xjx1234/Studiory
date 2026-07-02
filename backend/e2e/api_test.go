//go:build integration

package e2e_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"backend/internal/repo"
)

func TestHealthAndReady(t *testing.T) {
	w, body := doJSON(t, http.MethodGet, "/health", nil)
	if w.Code != http.StatusOK || body.Code != 0 {
		t.Fatalf("health: http=%d code=%d", w.Code, body.Code)
	}

	w, body = doJSON(t, http.MethodGet, "/ready", nil)
	if w.Code != http.StatusOK || body.Code != 0 {
		t.Fatalf("ready: http=%d code=%d body=%s", w.Code, body.Code, w.Body.String())
	}
}

func TestAuthRegisterLoginProfile(t *testing.T) {
	phone := uniquePhone()
	login := registerAndLogin(t, phone, "Str0ng!Pass")

	w, body := doJSON(t, http.MethodGet, "/api/v1/user/profile", nil, bearer(login.Tokens.AccessToken)...)
	if w.Code != http.StatusOK || body.Code != 0 {
		t.Fatalf("profile: http=%d code=%d body=%s", w.Code, body.Code, w.Body.String())
	}

	var profile struct {
		ID    string `json:"id"`
		Phone string `json:"phone"`
		Role  string `json:"role"`
	}
	if err := json.Unmarshal(body.Data, &profile); err != nil {
		t.Fatalf("decode profile: %v", err)
	}
	if profile.ID != login.User.ID || profile.Role != repo.RoleUser {
		t.Fatalf("profile mismatch: %+v login=%+v", profile, login.User)
	}
	if profile.Phone != phone {
		t.Errorf("phone = %q, want %q", profile.Phone, phone)
	}
}

func TestTodoLifecycle(t *testing.T) {
	login := registerAndLogin(t, uniquePhone(), "Str0ng!Pass")
	token := login.Tokens.AccessToken

	w, body := doJSON(t, http.MethodPost, "/api/v1/user/todos", map[string]any{
		"title": "e2e todo",
	}, bearer(token)...)
	if w.Code != http.StatusOK || body.Code != 0 {
		t.Fatalf("create todo: http=%d code=%d body=%s", w.Code, body.Code, w.Body.String())
	}

	var created struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		Done  bool   `json:"done"`
	}
	if err := json.Unmarshal(body.Data, &created); err != nil {
		t.Fatalf("decode todo: %v", err)
	}
	if created.Title != "e2e todo" || created.Done {
		t.Fatalf("unexpected todo: %+v", created)
	}

	w, body = doJSON(t, http.MethodGet, "/api/v1/user/todos?page=1&page_size=20", nil, bearer(token)...)
	if w.Code != http.StatusOK || body.Code != 0 {
		t.Fatalf("list todos: http=%d code=%d", w.Code, body.Code)
	}

	var list struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(body.Data, &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if list.Total < 1 {
		t.Fatalf("expected at least 1 todo, got %+v", list)
	}

	w, body = doJSON(t, http.MethodPatch, "/api/v1/user/todos/"+created.ID, map[string]any{
		"title": "updated",
		"done":  true,
	}, bearer(token)...)
	if w.Code != http.StatusOK || body.Code != 0 {
		t.Fatalf("update todo: http=%d code=%d", w.Code, body.Code)
	}

	w, body = doJSON(t, http.MethodDelete, "/api/v1/user/todos/"+created.ID, nil, bearer(token)...)
	if w.Code != http.StatusOK || body.Code != 0 {
		t.Fatalf("delete todo: http=%d code=%d", w.Code, body.Code)
	}
}

func TestRefreshAndLogout(t *testing.T) {
	login := registerAndLogin(t, uniquePhone(), "Str0ng!Pass")

	w, body := doJSON(t, http.MethodPost, "/api/v1/auth/refresh", map[string]any{
		"refresh_token": login.Tokens.RefreshToken,
	})
	if w.Code != http.StatusOK || body.Code != 0 {
		t.Fatalf("refresh: http=%d code=%d body=%s", w.Code, body.Code, w.Body.String())
	}

	var refreshed tokenPair
	if err := json.Unmarshal(body.Data, &refreshed); err != nil {
		t.Fatalf("decode refresh: %v", err)
	}
	if refreshed.AccessToken == "" || refreshed.RefreshToken == "" {
		t.Fatal("expected new token pair")
	}

	w, body = doJSON(t, http.MethodPost, "/api/v1/auth/logout", map[string]any{
		"refresh_token": refreshed.RefreshToken,
	}, bearer(refreshed.AccessToken)...)
	if w.Code != http.StatusOK || body.Code != 0 {
		t.Fatalf("logout: http=%d code=%d body=%s", w.Code, body.Code, w.Body.String())
	}

	// 旧 refresh token 不可再次刷新
	w, body = doJSON(t, http.MethodPost, "/api/v1/auth/refresh", map[string]any{
		"refresh_token": refreshed.RefreshToken,
	})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("refresh after logout: http=%d want 401", w.Code)
	}
}

func TestAdminUsersFlow(t *testing.T) {
	const adminPass = "Admin!Pass1"
	adminPhone := uniquePhone()
	createAdminUser(t, adminPhone, adminPass)

	w, body := doJSON(t, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"grant_type": "password",
		"phone":      adminPhone,
		"password":   adminPass,
	})
	if w.Code != http.StatusOK || body.Code != 0 {
		t.Fatalf("admin login: http=%d code=%d body=%s", w.Code, body.Code, w.Body.String())
	}
	var adminLogin loginData
	if err := json.Unmarshal(body.Data, &adminLogin); err != nil {
		t.Fatalf("decode admin login: %v", err)
	}
	if adminLogin.User.Role != repo.RoleAdmin {
		t.Fatalf("role = %q, want admin", adminLogin.User.Role)
	}
	adminToken := adminLogin.Tokens.AccessToken

	w, body = doJSON(t, http.MethodGet, "/api/v1/admin/ping", nil, bearer(adminToken)...)
	if w.Code != http.StatusOK || body.Code != 0 {
		t.Fatalf("admin ping: http=%d code=%d", w.Code, body.Code)
	}

	// 普通用户不能访问 admin
	userLogin := registerAndLogin(t, uniquePhone(), "Str0ng!Pass")
	w, body = doJSON(t, http.MethodGet, "/api/v1/admin/ping", nil, bearer(userLogin.Tokens.AccessToken)...)
	if w.Code != http.StatusForbidden || body.Code != 10007 {
		t.Fatalf("user admin ping: http=%d code=%d want 403/10007", w.Code, body.Code)
	}

	w, body = doJSON(t, http.MethodGet, "/api/v1/admin/users?page=1&page_size=10", nil, bearer(adminToken)...)
	if w.Code != http.StatusOK || body.Code != 0 {
		t.Fatalf("admin list users: http=%d code=%d body=%s", w.Code, body.Code, w.Body.String())
	}

	var list struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(body.Data, &list); err != nil {
		t.Fatalf("decode admin list: %v", err)
	}
	if list.Total < 2 {
		t.Fatalf("expected at least admin + user, total=%d", list.Total)
	}

	w, body = doJSON(t, http.MethodGet, "/api/v1/admin/users/"+userLogin.User.ID, nil, bearer(adminToken)...)
	if w.Code != http.StatusOK || body.Code != 0 {
		t.Fatalf("admin get user: http=%d code=%d body=%s", w.Code, body.Code, w.Body.String())
	}
}

func TestOAuthDevModeLogin(t *testing.T) {
	openID := "wx_e2e_" + uniquePhone()
	w, body := doJSON(t, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"grant_type": "oauth",
		"provider":   "wechat",
		"open_id":    openID,
	})
	if w.Code != http.StatusOK || body.Code != 0 {
		t.Fatalf("oauth login: http=%d code=%d body=%s", w.Code, body.Code, w.Body.String())
	}

	var data loginData
	if err := json.Unmarshal(body.Data, &data); err != nil {
		t.Fatalf("decode oauth login: %v", err)
	}
	if data.Tokens.AccessToken == "" {
		t.Fatal("expected tokens from oauth login")
	}

	w, body = doJSON(t, http.MethodGet, "/api/v1/user/profile", nil, bearer(data.Tokens.AccessToken)...)
	if w.Code != http.StatusOK || body.Code != 0 {
		t.Fatalf("oauth profile: http=%d code=%d", w.Code, body.Code)
	}
}
