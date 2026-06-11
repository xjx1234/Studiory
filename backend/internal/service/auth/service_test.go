package authservice

import (
	"context"
	"errors"
	"testing"
	"time"

	"backend/internal/auth"
	"backend/internal/repo"
	"backend/internal/testutil"

	"github.com/google/uuid"
)

func testTokenIssuer() *auth.TokenIssuer {
	return auth.NewTokenIssuer("test-secret-key-for-unit-tests", time.Hour, time.Hour)
}

// fakeOAuthRepo 是 OAuth repo 的内存实现，仅用于 auth service 测试。
type fakeOAuthRepo struct {
	byKey map[string]*repo.User
}

func newFakeOAuthRepo() *fakeOAuthRepo {
	return &fakeOAuthRepo{byKey: make(map[string]*repo.User)}
}

func oauthKey(provider, openID string) string {
	return provider + ":" + openID
}

func (r *fakeOAuthRepo) GetOAuth(_ context.Context, provider, openID string) (*repo.UserOAuth, error) {
	return nil, repo.ErrNotFound
}

func (r *fakeOAuthRepo) CreateOAuth(_ context.Context, userID uuid.UUID, provider, openID string) (*repo.UserOAuth, error) {
	key := oauthKey(provider, openID)
	if _, exists := r.byKey[key]; exists {
		return nil, errors.New("oauth binding already exists")
	}
	return &repo.UserOAuth{
		ID:       uuid.New(),
		UserID:   userID,
		Provider: provider,
		OpenID:   openID,
	}, nil
}

func (r *fakeOAuthRepo) GetUserByOAuth(_ context.Context, provider, openID string) (*repo.User, error) {
	if u, ok := r.byKey[oauthKey(provider, openID)]; ok {
		return u, nil
	}
	return nil, repo.ErrNotFound
}

func (r *fakeOAuthRepo) bind(user *repo.User, provider, openID string) {
	r.byKey[oauthKey(provider, openID)] = user
}

func TestLoginWithOAuthCreatesUserOnFirstLogin(t *testing.T) {
	users := testutil.NewFakeUserRepo()
	oauthRepo := newFakeOAuthRepo()

	svc := New(users, nil,
		WithTokenIssuer(testTokenIssuer()),
		WithOAuthRepo(oauthRepo),
		WithOAuthDevMode(true),
	)

	result, e := svc.Login(context.Background(), &auth.LoginRequest{
		GrantType: auth.GrantTypeOAuth,
		Provider:  "wechat",
		OpenID:    "wx_openid_001",
	})
	if e != nil {
		t.Fatalf("login failed: %+v", e)
	}
	if result == nil || result.Tokens == nil || result.User == nil {
		t.Fatal("expected login result with tokens and user")
	}
	if len(users.Created) != 1 {
		t.Fatalf("expected 1 created user, got %d", len(users.Created))
	}

	// 绑定后再次登录应直接返回同一用户
	oauthRepo.bind(users.Created[0], "wechat", "wx_openid_001")

	result2, e2 := svc.Login(context.Background(), &auth.LoginRequest{
		GrantType: auth.GrantTypeOAuth,
		Provider:  "wechat",
		OpenID:    "wx_openid_001",
	})
	if e2 != nil {
		t.Fatalf("second login failed: %+v", e2)
	}
	if result2.User.ID != result.User.ID {
		t.Fatalf("expected same user id, got %s vs %s", result2.User.ID, result.User.ID)
	}
}

func TestLoginWithOAuthRejectsUnknownProvider(t *testing.T) {
	svc := New(testutil.NewFakeUserRepo(), nil,
		WithTokenIssuer(testTokenIssuer()),
		WithOAuthRepo(newFakeOAuthRepo()),
		WithOAuthDevMode(true),
	)

	_, e := svc.Login(context.Background(), &auth.LoginRequest{
		GrantType: auth.GrantTypeOAuth,
		Provider:  "unknown",
		OpenID:    "id_001",
	})
	if e == nil {
		t.Fatal("expected unsupported grant error")
	}
}

func TestLoginWithOAuthRequiresDevMode(t *testing.T) {
	svc := New(testutil.NewFakeUserRepo(), nil,
		WithTokenIssuer(testTokenIssuer()),
		WithOAuthRepo(newFakeOAuthRepo()),
		WithOAuthDevMode(false),
	)

	_, e := svc.Login(context.Background(), &auth.LoginRequest{
		GrantType: auth.GrantTypeOAuth,
		Provider:  "wechat",
		OpenID:    "wx_openid_001",
	})
	if e == nil {
		t.Fatal("expected unsupported grant when dev mode disabled")
	}
}
