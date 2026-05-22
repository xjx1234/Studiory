package authservice

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"backend/internal/auth"
	"backend/internal/repo"

	"github.com/google/uuid"
)

func TestMain(m *testing.M) {
	auth.InitToken("test-secret-key-for-unit-tests", time.Hour, time.Hour)
	os.Exit(m.Run())
}

type fakeUserRepo struct {
	users   map[uuid.UUID]*repo.User
	created []*repo.User
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{users: make(map[uuid.UUID]*repo.User)}
}

func (r *fakeUserRepo) GetByID(_ context.Context, id uuid.UUID) (*repo.User, error) {
	if u, ok := r.users[id]; ok {
		return u, nil
	}
	return nil, repo.ErrNotFound
}

func (r *fakeUserRepo) GetByPhone(context.Context, string) (*repo.User, error) {
	return nil, repo.ErrNotFound
}

func (r *fakeUserRepo) GetByEmail(context.Context, string) (*repo.User, error) {
	return nil, repo.ErrNotFound
}

func (r *fakeUserRepo) Create(_ context.Context, _, _, _ *string, nickname, avatar, role string) (*repo.User, error) {
	u := &repo.User{
		ID:       uuid.New(),
		Nickname: nickname,
		Avatar:   avatar,
		Role:     role,
	}
	r.users[u.ID] = u
	r.created = append(r.created, u)
	return u, nil
}

func (r *fakeUserRepo) UpdatePassword(context.Context, uuid.UUID, string) (*repo.User, error) {
	return nil, repo.ErrNotFound
}

func (r *fakeUserRepo) UpdateProfile(context.Context, uuid.UUID, string, string) (*repo.User, error) {
	return nil, repo.ErrNotFound
}

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
	users := newFakeUserRepo()
	oauthRepo := newFakeOAuthRepo()

	svc := New(users, nil,
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
	if len(users.created) != 1 {
		t.Fatalf("expected 1 created user, got %d", len(users.created))
	}

	// 绑定后再次登录应直接返回同一用户
	oauthRepo.bind(users.created[0], "wechat", "wx_openid_001")

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
	svc := New(newFakeUserRepo(), nil,
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
	svc := New(newFakeUserRepo(), nil,
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
