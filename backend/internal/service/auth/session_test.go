package authservice

import (
	"context"
	"testing"
	"time"

	"backend/internal/auth"
	"backend/internal/repo"
	"backend/internal/session"
	"backend/internal/testutil"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

func authWithSessions(t *testing.T, multiDevice bool) (Service, *session.Store, *testutil.FakeUserRepo) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	issuer := testTokenIssuer()
	sess := session.NewStore(rdb, "test", multiDevice, time.Hour)
	users := testutil.NewFakeUserRepo()

	svc := New(users, NewRedisCacheStore(rdb),
		WithTokenIssuer(issuer),
		WithSessionStore(sess),
	)
	return svc, sess, users
}

func strPtr(s string) *string { return &s }

func seedPasswordUser(t *testing.T, users *testutil.FakeUserRepo, phone, password string) uuid.UUID {
	t.Helper()
	hashBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	hash := string(hashBytes)
	uid := uuid.New()
	users.Users[uid] = &repo.User{
		ID: uid, Phone: strPtr(phone), PasswordHash: &hash,
		Nickname: "u", Role: repo.RoleUser, Status: repo.StatusActive,
	}
	return uid
}

func TestLogin_SingleDeviceKicksPreviousSession(t *testing.T) {
	svc, sess, users := authWithSessions(t, false)
	ctx := context.Background()

	phone := "13800138001"
	uid := seedPasswordUser(t, users, phone, "password")

	login1, e1 := svc.Login(ctx, &auth.LoginRequest{
		GrantType: auth.GrantTypePassword,
		Phone:     phone,
		Password:  "password",
	})
	if e1 != nil {
		t.Fatalf("first login: %+v", e1)
	}

	claims1, err := testTokenIssuer().ParseAccessToken(login1.Tokens.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if claims1.SessionID == "" {
		t.Fatal("expected session id in token")
	}

	login2, e2 := svc.Login(ctx, &auth.LoginRequest{
		GrantType: auth.GrantTypePassword,
		Phone:     phone,
		Password:  "password",
	})
	if e2 != nil {
		t.Fatalf("second login: %+v", e2)
	}

	claims2, _ := testTokenIssuer().ParseAccessToken(login2.Tokens.AccessToken)
	if claims1.SessionID == claims2.SessionID {
		t.Fatal("expected different session ids")
	}

	if sess.Validate(ctx, uid.String(), claims1.SessionID) {
		t.Fatal("first session should be kicked in single-device mode")
	}
	if !sess.Validate(ctx, uid.String(), claims2.SessionID) {
		t.Fatal("second session should be active")
	}
}

func TestLogin_MultiDeviceKeepsBothSessions(t *testing.T) {
	svc, sess, users := authWithSessions(t, true)
	ctx := context.Background()

	phone := "13800138002"
	uid := seedPasswordUser(t, users, phone, "password")

	login1, e1 := svc.Login(ctx, &auth.LoginRequest{
		GrantType: auth.GrantTypePassword, Phone: phone, Password: "password",
	})
	if e1 != nil {
		t.Fatalf("first login: %+v", e1)
	}
	login2, e2 := svc.Login(ctx, &auth.LoginRequest{
		GrantType: auth.GrantTypePassword, Phone: phone, Password: "password",
	})
	if e2 != nil {
		t.Fatalf("second login: %+v", e2)
	}

	c1, _ := testTokenIssuer().ParseAccessToken(login1.Tokens.AccessToken)
	c2, _ := testTokenIssuer().ParseAccessToken(login2.Tokens.AccessToken)

	if !sess.Validate(ctx, uid.String(), c1.SessionID) || !sess.Validate(ctx, uid.String(), c2.SessionID) {
		t.Fatal("both sessions should remain valid in multi-device mode")
	}
}

func TestLogout_OnlyRevokesCurrentSession_MultiDevice(t *testing.T) {
	svc, sess, users := authWithSessions(t, true)
	ctx := context.Background()

	phone := "13800138003"
	uid := seedPasswordUser(t, users, phone, "password")

	login1, e1 := svc.Login(ctx, &auth.LoginRequest{
		GrantType: auth.GrantTypePassword, Phone: phone, Password: "password",
	})
	if e1 != nil {
		t.Fatalf("first login: %+v", e1)
	}
	login2, e2 := svc.Login(ctx, &auth.LoginRequest{
		GrantType: auth.GrantTypePassword, Phone: phone, Password: "password",
	})
	if e2 != nil {
		t.Fatalf("second login: %+v", e2)
	}

	c1, _ := testTokenIssuer().ParseAccessToken(login1.Tokens.AccessToken)
	c2, _ := testTokenIssuer().ParseAccessToken(login2.Tokens.AccessToken)

	if e := svc.Logout(ctx, login1.Tokens.RefreshToken); e != nil {
		t.Fatalf("logout: %+v", e)
	}

	if sess.Validate(ctx, uid.String(), c1.SessionID) {
		t.Fatal("logged out session should be invalid")
	}
	if !sess.Validate(ctx, uid.String(), c2.SessionID) {
		t.Fatal("other device session should still be valid")
	}
}
