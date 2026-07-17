package userservice

import (
	"context"
	"errors"
	"testing"
	"time"

	"backend/internal/repo"
	"backend/internal/session"
	"backend/internal/testutil"
	"backend/pkg/errcode"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

func TestWithLoggerOption(t *testing.T) {
	svc := New(testutil.NewFakeUserRepo(), WithLogger(zap.NewNop()))
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestGetProfile_IncludesEmail(t *testing.T) {
	id := uuid.New()
	email := "a@example.com"
	fakeRepo := testutil.NewFakeUserRepo()
	fakeRepo.Users[id] = &repo.User{ID: id, Email: &email, Nickname: "n", Role: repo.RoleUser}
	svc := New(fakeRepo)

	profile, e := svc.GetProfile(context.Background(), id.String())
	if e != nil {
		t.Fatalf("GetProfile: %+v", e)
	}
	if profile.Email != email {
		t.Fatalf("Email = %q, want %q", profile.Email, email)
	}
}

func TestUpdateProfile(t *testing.T) {
	id := uuid.New()
	fakeRepo := testutil.NewFakeUserRepo()
	fakeRepo.Users[id] = &repo.User{
		ID:       id,
		Nickname: "旧昵称",
		Avatar:   "",
		Role:     repo.RoleUser,
	}

	svc := New(fakeRepo)

	profile, e := svc.UpdateProfile(context.Background(), id.String(), &UpdateProfileInput{
		Nickname: "新昵称",
		Avatar:   "https://example.com/a.png",
	})
	if e != nil {
		t.Fatalf("update failed: %+v", e)
	}
	if profile.Nickname != "新昵称" || profile.Avatar != "https://example.com/a.png" {
		t.Fatalf("unexpected profile: %+v", profile)
	}
}

func TestUpdateProfile_EmptyFieldsKeepOldValues(t *testing.T) {
	id := uuid.New()
	fakeRepo := testutil.NewFakeUserRepo()
	fakeRepo.Users[id] = &repo.User{ID: id, Nickname: "旧昵称", Avatar: "old.png", Role: repo.RoleUser}

	svc := New(fakeRepo)

	profile, e := svc.UpdateProfile(context.Background(), id.String(), &UpdateProfileInput{})
	if e != nil {
		t.Fatalf("update failed: %+v", e)
	}
	if profile.Nickname != "旧昵称" || profile.Avatar != "old.png" {
		t.Fatalf("expected old values preserved, got: %+v", profile)
	}
}

func TestUpdateProfile_InvalidUserID(t *testing.T) {
	svc := New(testutil.NewFakeUserRepo())

	_, e := svc.UpdateProfile(context.Background(), "not-a-uuid", &UpdateProfileInput{Nickname: "x"})
	if e == nil || e.Code != errcode.ErrBadRequest.Code {
		t.Fatalf("expected ErrBadRequest, got %+v", e)
	}
}

func TestUpdateProfile_NilInput(t *testing.T) {
	id := uuid.New()
	fakeRepo := testutil.NewFakeUserRepo()
	fakeRepo.Users[id] = &repo.User{ID: id}
	svc := New(fakeRepo)

	_, e := svc.UpdateProfile(context.Background(), id.String(), nil)
	if e == nil || e.Code != errcode.ErrBadRequest.Code {
		t.Fatalf("expected ErrBadRequest, got %+v", e)
	}
}

func TestUpdateProfile_UserNotFound(t *testing.T) {
	svc := New(testutil.NewFakeUserRepo())

	_, e := svc.UpdateProfile(context.Background(), uuid.New().String(), &UpdateProfileInput{Nickname: "x"})
	if e == nil || e.Code != errcode.ErrNotFound.Code {
		t.Fatalf("expected ErrNotFound, got %+v", e)
	}
}

func TestUpdateProfile_LookupInternalError(t *testing.T) {
	fakeRepo := testutil.NewFakeUserRepo()
	fakeRepo.GetByIDErr = errors.New("db down")
	svc := New(fakeRepo)

	_, e := svc.UpdateProfile(context.Background(), uuid.New().String(), &UpdateProfileInput{Nickname: "x"})
	if e == nil || e.Code != errcode.ErrInternal.Code {
		t.Fatalf("expected ErrInternal, got %+v", e)
	}
}

func TestUpdateProfile_UpdateInternalError(t *testing.T) {
	id := uuid.New()
	fakeRepo := testutil.NewFakeUserRepo()
	fakeRepo.Users[id] = &repo.User{ID: id}
	fakeRepo.UpdateProfileErr = errors.New("db down")
	svc := New(fakeRepo)

	_, e := svc.UpdateProfile(context.Background(), id.String(), &UpdateProfileInput{Nickname: "x"})
	if e == nil || e.Code != errcode.ErrInternal.Code {
		t.Fatalf("expected ErrInternal, got %+v", e)
	}
}

func TestUpdateProfile_UpdateReturnsNotFound(t *testing.T) {
	id := uuid.New()
	fakeRepo := testutil.NewFakeUserRepo()
	fakeRepo.Users[id] = &repo.User{ID: id}
	fakeRepo.UpdateProfileErr = repo.ErrNotFound
	svc := New(fakeRepo)

	_, e := svc.UpdateProfile(context.Background(), id.String(), &UpdateProfileInput{Nickname: "x"})
	if e == nil || e.Code != errcode.ErrNotFound.Code {
		t.Fatalf("expected ErrNotFound, got %+v", e)
	}
}

func TestGetProfile_Success(t *testing.T) {
	id := uuid.New()
	phone := "13800138000"
	fakeRepo := testutil.NewFakeUserRepo()
	fakeRepo.Users[id] = &repo.User{ID: id, Nickname: "小明", Phone: &phone, Role: repo.RoleUser}
	svc := New(fakeRepo)

	profile, e := svc.GetProfile(context.Background(), id.String())
	if e != nil {
		t.Fatalf("GetProfile: %+v", e)
	}
	if profile.Nickname != "小明" || profile.Phone != phone {
		t.Fatalf("unexpected profile: %+v", profile)
	}
}

func TestGetProfile_InvalidUserID(t *testing.T) {
	svc := New(testutil.NewFakeUserRepo())

	_, e := svc.GetProfile(context.Background(), "not-a-uuid")
	if e == nil || e.Code != errcode.ErrBadRequest.Code {
		t.Fatalf("expected ErrBadRequest, got %+v", e)
	}
}

func TestGetProfile_NotFound(t *testing.T) {
	svc := New(testutil.NewFakeUserRepo())

	_, e := svc.GetProfile(context.Background(), uuid.New().String())
	if e == nil || e.Code != errcode.ErrNotFound.Code {
		t.Fatalf("expected ErrNotFound, got %+v", e)
	}
}

func TestGetProfile_InternalError(t *testing.T) {
	fakeRepo := testutil.NewFakeUserRepo()
	fakeRepo.GetByIDErr = errors.New("db down")
	svc := New(fakeRepo)

	_, e := svc.GetProfile(context.Background(), uuid.New().String())
	if e == nil || e.Code != errcode.ErrInternal.Code {
		t.Fatalf("expected ErrInternal, got %+v", e)
	}
}

func hashPassword(t *testing.T, pw string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	return string(h)
}

func TestChangePassword_Success(t *testing.T) {
	id := uuid.New()
	hash := hashPassword(t, "OldPass123")
	fakeRepo := testutil.NewFakeUserRepo()
	fakeRepo.Users[id] = &repo.User{ID: id, PasswordHash: &hash}
	svc := New(fakeRepo)

	e := svc.ChangePassword(context.Background(), id.String(), &ChangePasswordInput{
		OldPassword: "OldPass123",
		NewPassword: "NewPass456",
	})
	if e != nil {
		t.Fatalf("ChangePassword: %+v", e)
	}

	updated := fakeRepo.Users[id]
	if err := bcrypt.CompareHashAndPassword([]byte(*updated.PasswordHash), []byte("NewPass456")); err != nil {
		t.Fatalf("new password hash mismatch: %v", err)
	}
}

func TestChangePassword_NilOrEmptyInput(t *testing.T) {
	id := uuid.New()
	svc := New(testutil.NewFakeUserRepo())

	cases := []*ChangePasswordInput{
		nil,
		{},
		{OldPassword: "only-old"},
		{NewPassword: "only-new"},
	}
	for _, in := range cases {
		e := svc.ChangePassword(context.Background(), id.String(), in)
		if e == nil || e.Code != errcode.ErrBadRequest.Code {
			t.Errorf("input=%+v: expected ErrBadRequest, got %+v", in, e)
		}
	}
}

func TestChangePassword_InvalidUserID(t *testing.T) {
	svc := New(testutil.NewFakeUserRepo())

	e := svc.ChangePassword(context.Background(), "not-a-uuid", &ChangePasswordInput{OldPassword: "a", NewPassword: "b"})
	if e == nil || e.Code != errcode.ErrBadRequest.Code {
		t.Fatalf("expected ErrBadRequest, got %+v", e)
	}
}

func TestChangePassword_UserNotFound(t *testing.T) {
	svc := New(testutil.NewFakeUserRepo())

	e := svc.ChangePassword(context.Background(), uuid.New().String(), &ChangePasswordInput{OldPassword: "a", NewPassword: "b"})
	if e == nil || e.Code != errcode.ErrNotFound.Code {
		t.Fatalf("expected ErrNotFound, got %+v", e)
	}
}

func TestChangePassword_LookupInternalError(t *testing.T) {
	fakeRepo := testutil.NewFakeUserRepo()
	fakeRepo.GetByIDErr = errors.New("db down")
	svc := New(fakeRepo)

	e := svc.ChangePassword(context.Background(), uuid.New().String(), &ChangePasswordInput{OldPassword: "a", NewPassword: "b"})
	if e == nil || e.Code != errcode.ErrInternal.Code {
		t.Fatalf("expected ErrInternal, got %+v", e)
	}
}

func TestChangePassword_NoPasswordHashSet(t *testing.T) {
	// OAuth-only 用户，没有设置过密码。
	id := uuid.New()
	fakeRepo := testutil.NewFakeUserRepo()
	fakeRepo.Users[id] = &repo.User{ID: id}
	svc := New(fakeRepo)

	e := svc.ChangePassword(context.Background(), id.String(), &ChangePasswordInput{OldPassword: "a", NewPassword: "b"})
	if e == nil || e.Code != errcode.ErrInvalidCredentials.Code {
		t.Fatalf("expected ErrInvalidCredentials, got %+v", e)
	}
}

func TestChangePassword_EmptyPasswordHashSet(t *testing.T) {
	id := uuid.New()
	empty := ""
	fakeRepo := testutil.NewFakeUserRepo()
	fakeRepo.Users[id] = &repo.User{ID: id, PasswordHash: &empty}
	svc := New(fakeRepo)

	e := svc.ChangePassword(context.Background(), id.String(), &ChangePasswordInput{OldPassword: "a", NewPassword: "b"})
	if e == nil || e.Code != errcode.ErrInvalidCredentials.Code {
		t.Fatalf("expected ErrInvalidCredentials, got %+v", e)
	}
}

func TestChangePassword_WrongOldPassword(t *testing.T) {
	id := uuid.New()
	hash := hashPassword(t, "CorrectOld1")
	fakeRepo := testutil.NewFakeUserRepo()
	fakeRepo.Users[id] = &repo.User{ID: id, PasswordHash: &hash}
	svc := New(fakeRepo)

	e := svc.ChangePassword(context.Background(), id.String(), &ChangePasswordInput{
		OldPassword: "WrongOld1",
		NewPassword: "NewPass456",
	})
	if e == nil || e.Code != errcode.ErrWrongPassword.Code {
		t.Fatalf("expected ErrWrongPassword, got %+v", e)
	}
}

func TestChangePassword_SamePassword(t *testing.T) {
	id := uuid.New()
	hash := hashPassword(t, "SamePass123")
	fakeRepo := testutil.NewFakeUserRepo()
	fakeRepo.Users[id] = &repo.User{ID: id, PasswordHash: &hash}
	svc := New(fakeRepo)

	e := svc.ChangePassword(context.Background(), id.String(), &ChangePasswordInput{
		OldPassword: "SamePass123",
		NewPassword: "SamePass123",
	})
	if e == nil || e.Code != errcode.ErrSamePassword.Code {
		t.Fatalf("expected ErrSamePassword, got %+v", e)
	}
}

func TestChangePassword_UpdateInternalError(t *testing.T) {
	id := uuid.New()
	hash := hashPassword(t, "OldPass123")
	fakeRepo := testutil.NewFakeUserRepo()
	fakeRepo.Users[id] = &repo.User{ID: id, PasswordHash: &hash}
	fakeRepo.UpdatePasswordErr = errors.New("db down")
	svc := New(fakeRepo)

	e := svc.ChangePassword(context.Background(), id.String(), &ChangePasswordInput{
		OldPassword: "OldPass123",
		NewPassword: "NewPass456",
	})
	if e == nil || e.Code != errcode.ErrInternal.Code {
		t.Fatalf("expected ErrInternal, got %+v", e)
	}
}

func TestChangePassword_RevokesSessionsAndAccessTokens(t *testing.T) {
	id := uuid.New()
	hash := hashPassword(t, "OldPass123")
	fakeRepo := testutil.NewFakeUserRepo()
	fakeRepo.Users[id] = &repo.User{ID: id, PasswordHash: &hash}

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	sessions := session.NewStore(rdb, "test", true, time.Hour)
	if err := sessions.Register(context.Background(), id.String(), "session-1"); err != nil {
		t.Fatalf("Register: %v", err)
	}

	svc := New(fakeRepo, WithSessionStore(sessions), WithRevokeSupport(rdb, "test", time.Hour))

	e := svc.ChangePassword(context.Background(), id.String(), &ChangePasswordInput{
		OldPassword: "OldPass123",
		NewPassword: "NewPass456",
	})
	if e != nil {
		t.Fatalf("ChangePassword: %+v", e)
	}

	if sessions.Validate(context.Background(), id.String(), "session-1") {
		t.Error("expected session to be revoked after password change")
	}

	revokeKey := "test:revoke:uid:" + id.String()
	if exists, _ := rdb.Exists(context.Background(), revokeKey).Result(); exists == 0 {
		t.Error("expected access-token revoke marker to be set after password change")
	}
}

func TestChangePassword_RevokeErrorsDoNotFailRequest(t *testing.T) {
	// sessions/rdb 均已关闭连接：Revoke 操作会报错，但改密本身应当仍然成功（fail-open，只记日志）。
	id := uuid.New()
	hash := hashPassword(t, "OldPass123")
	fakeRepo := testutil.NewFakeUserRepo()
	fakeRepo.Users[id] = &repo.User{ID: id, PasswordHash: &hash}

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	sessions := session.NewStore(rdb, "test", true, time.Hour)
	mr.Close()

	svc := New(fakeRepo, WithSessionStore(sessions), WithRevokeSupport(rdb, "test", time.Hour))

	e := svc.ChangePassword(context.Background(), id.String(), &ChangePasswordInput{
		OldPassword: "OldPass123",
		NewPassword: "NewPass456",
	})
	if e != nil {
		t.Fatalf("expected ChangePassword to succeed despite revoke errors, got %+v", e)
	}
}
