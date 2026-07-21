package authservice

import (
	"context"
	"testing"

	"backend/internal/repo"
	"backend/internal/testutil"
	"backend/pkg/errcode"

	"github.com/google/uuid"
)

// tokenTestSvc 创建带 miniredis 的 AuthService 并预置一个活跃用户。
// 返回 service、预置用户和该用户的 refresh token。
func tokenTestSvc(t *testing.T) (*AuthServiceImpl, *repo.User, string) {
	t.Helper()
	_, rdb := newTestRDB(t)
	users := testutil.NewFakeUserRepo()

	issuer := testTokenIssuer()
	svc, ok := New(users, NewRedisCacheStore(rdb),
		WithTokenIssuer(issuer),
	).(*AuthServiceImpl)
	if !ok {
		t.Fatal("expected *AuthServiceImpl")
	}

	uid := uuid.New()
	phone := "13700000001"
	user := &repo.User{
		ID:       uid,
		Phone:    &phone,
		Nickname: "token-test-user",
		Role:     repo.RoleUser,
		Status:   repo.StatusActive,
	}
	users.Users[uid] = user

	pair, err := issuer.IssueTokenPair(uid.String(), repo.RoleUser, "sess-1")
	if err != nil {
		t.Fatalf("issue token pair: %v", err)
	}

	return svc, user, pair.RefreshToken
}

// TestRefresh_Success 验证合法 refresh token 能换取新 token pair。
func TestRefresh_Success(t *testing.T) {
	svc, user, refreshToken := tokenTestSvc(t)

	pair, e := svc.Refresh(context.Background(), refreshToken)
	if e != nil {
		t.Fatalf("refresh failed: %+v", e)
	}
	if pair == nil || pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatal("expected non-empty token pair")
	}
	// 验证新 access token 可正常解析且携带正确的 userID
	claims, parseErr := svc.tokens.ParseAccessToken(pair.AccessToken)
	if parseErr != nil {
		t.Fatalf("parse new access token: %v", parseErr)
	}
	if claims.UserID != user.ID.String() {
		t.Errorf("new token userID = %q, want %q", claims.UserID, user.ID.String())
	}
}

// TestRefresh_InvalidToken 验证无效 token 返回 ErrInvalidToken。
func TestRefresh_InvalidToken(t *testing.T) {
	svc, _, _ := tokenTestSvc(t)

	_, e := svc.Refresh(context.Background(), "not-a-valid-jwt")
	if e != errcode.ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken, got %+v", e)
	}
}

// TestRefresh_DisabledUser 验证被禁用用户的 refresh 被拒绝。
func TestRefresh_DisabledUser(t *testing.T) {
	svc, user, refreshToken := tokenTestSvc(t)

	// 禁用用户
	user.Status = repo.StatusDisabled

	_, e := svc.Refresh(context.Background(), refreshToken)
	if e != errcode.ErrAccountDisabled {
		t.Fatalf("expected ErrAccountDisabled, got %+v", e)
	}
}

// TestRefresh_ReplayRejected 验证同一 refresh token 二次刷新被 SetNX 拒绝。
func TestRefresh_ReplayRejected(t *testing.T) {
	svc, _, refreshToken := tokenTestSvc(t)

	// 第一次刷新成功
	pair1, e := svc.Refresh(context.Background(), refreshToken)
	if e != nil {
		t.Fatalf("first refresh failed: %+v", e)
	}
	if pair1 == nil {
		t.Fatal("expected token pair")
	}

	// 同一 token 再次刷新应被拒绝（SetNX 原子黑名单）
	_, e2 := svc.Refresh(context.Background(), refreshToken)
	if e2 != errcode.ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken on replay, got %+v", e2)
	}
}

// TestLogout_Success 验证登出后该 refresh token 无法再刷新。
func TestLogout_Success(t *testing.T) {
	svc, _, refreshToken := tokenTestSvc(t)

	if e := svc.Logout(context.Background(), refreshToken); e != nil {
		t.Fatalf("logout failed: %+v", e)
	}

	// 登出后 refresh 应被拒绝
	_, e := svc.Refresh(context.Background(), refreshToken)
	if e != errcode.ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken after logout, got %+v", e)
	}
}

// TestLogout_EmptyToken 验证空 token 登出是无操作。
func TestLogout_EmptyToken(t *testing.T) {
	svc, _, _ := tokenTestSvc(t)

	if e := svc.Logout(context.Background(), ""); e != nil {
		t.Fatalf("logout with empty token should be no-op, got %+v", e)
	}
}

// TestRefresh_UserNotFound 验证用户已被删除后 refresh 被拒绝。
func TestRefresh_UserNotFound(t *testing.T) {
	svc, user, refreshToken := tokenTestSvc(t)

	// 从 repo 中删除用户（模拟用户被删除）
	delete(svc.users.(*testutil.FakeUserRepo).Users, user.ID)

	_, e := svc.Refresh(context.Background(), refreshToken)
	if e != errcode.ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken for deleted user, got %+v", e)
	}
}

// TestRefresh_IssuesNewSessionID 验证刷新后的 token 携带原 sessionID（用于会话延续）。
func TestRefresh_PreservesSessionID(t *testing.T) {
	_, rdb := newTestRDB(t)
	users := testutil.NewFakeUserRepo()
	issuer := testTokenIssuer()

	svc, ok := New(users, NewRedisCacheStore(rdb),
		WithTokenIssuer(issuer),
	).(*AuthServiceImpl)
	if !ok {
		t.Fatal("expected *AuthServiceImpl")
	}

	uid := uuid.New()
	users.Users[uid] = &repo.User{
		ID:     uid,
		Role:   repo.RoleUser,
		Status: repo.StatusActive,
	}

	originalSessionID := "my-session-123"
	pair, err := issuer.IssueTokenPair(uid.String(), repo.RoleUser, originalSessionID)
	if err != nil {
		t.Fatalf("issue token pair: %v", err)
	}

	newPair, e := svc.Refresh(context.Background(), pair.RefreshToken)
	if e != nil {
		t.Fatalf("refresh failed: %+v", e)
	}

	// 解析新 refresh token，验证 sessionID 被保留
	claims, parseErr := issuer.ParseRefreshToken(newPair.RefreshToken)
	if parseErr != nil {
		t.Fatalf("parse new refresh token: %v", parseErr)
	}
	if claims.SessionID != originalSessionID {
		t.Errorf("sessionID = %q, want %q", claims.SessionID, originalSessionID)
	}
}
