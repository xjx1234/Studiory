package authservice

import (
	"context"
	"testing"
	"time"

	"backend/internal/auth"
	"backend/internal/repo"
	"backend/internal/testutil"
	"backend/pkg/errcode"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

func newTestRDB(t *testing.T) (*miniredis.Miniredis, redis.UniversalClient) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return mr, rdb
}

func bfSvc(t *testing.T, repo *testutil.FakeUserRepo, rdb redis.UniversalClient) *AuthServiceImpl {
	t.Helper()
	return New(repo, rdb, WithTokenIssuer(testTokenIssuer()))
}

func seedUserWithPassword(fakeRepo *testutil.FakeUserRepo, phone, password string) {
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	hashStr := string(hash)
	phonePtr := phone
	id := uuid.New()
	fakeRepo.Users[id] = &repo.User{
		ID:           id,
		Phone:        &phonePtr,
		PasswordHash: &hashStr,
		Nickname:     "测试用户",
		Role:         "student",
	}
}

// TestLoginLockedAfterMaxFailures 验证连续失败后账号被锁定。
func TestLoginLockedAfterMaxFailures(t *testing.T) {
	_, rdb := newTestRDB(t)
	svc := bfSvc(t, testutil.NewFakeUserRepo(), rdb)

	req := &auth.LoginRequest{
		GrantType: auth.GrantTypePassword,
		Phone:     "13800000001",
		Password:  "wrong",
	}

	for i := 0; i < loginMaxFailAttempts; i++ {
		_, e := svc.Login(context.Background(), req)
		if e == nil {
			t.Fatalf("attempt %d: expected error, got nil", i+1)
		}
		if e.Code == errcode.ErrAccountLocked.Code {
			t.Fatalf("account locked too early at attempt %d", i+1)
		}
	}

	// 超过阈值后应返回锁定错误
	_, e := svc.Login(context.Background(), req)
	if e == nil || e.Code != errcode.ErrAccountLocked.Code {
		t.Fatalf("expected ErrAccountLocked after %d failures, got %v", loginMaxFailAttempts, e)
	}
}

// TestLoginClearsCounterOnSuccess 验证成功登录后失败计数被清除。
func TestLoginClearsCounterOnSuccess(t *testing.T) {
	mr, rdb := newTestRDB(t)
	fakeRepo := testutil.NewFakeUserRepo()
	phone := "13800000002"
	seedUserWithPassword(fakeRepo, phone, "correct")

	svc := bfSvc(t, fakeRepo, rdb)

	wrongReq := &auth.LoginRequest{GrantType: auth.GrantTypePassword, Phone: phone, Password: "wrong"}
	rightReq := &auth.LoginRequest{GrantType: auth.GrantTypePassword, Phone: phone, Password: "correct"}

	// 先失败 3 次（未达阈值）
	for i := 0; i < 3; i++ {
		svc.Login(context.Background(), wrongReq) //nolint:errcheck
	}

	// 成功登录一次
	_, e := svc.Login(context.Background(), rightReq)
	if e != nil {
		t.Fatalf("expected success, got %v", e)
	}

	// fail key 应已被删除
	failKey := svc.loginFailKey(loginAccountID(phone, "", ""))
	if mr.Exists(failKey) {
		t.Error("expected fail key to be deleted after successful login")
	}
}

// TestLoginLockExpires 验证锁定到期后可以重新登录。
func TestLoginLockExpires(t *testing.T) {
	mr, rdb := newTestRDB(t)
	fakeRepo := testutil.NewFakeUserRepo()
	phone := "13800000003"
	seedUserWithPassword(fakeRepo, phone, "correct")

	svc := bfSvc(t, fakeRepo, rdb)

	wrongReq := &auth.LoginRequest{GrantType: auth.GrantTypePassword, Phone: phone, Password: "wrong"}
	rightReq := &auth.LoginRequest{GrantType: auth.GrantTypePassword, Phone: phone, Password: "correct"}

	// 触发锁定（失败 loginMaxFailAttempts+1 次）
	for i := 0; i <= loginMaxFailAttempts; i++ {
		svc.Login(context.Background(), wrongReq) //nolint:errcheck
	}
	_, e := svc.Login(context.Background(), wrongReq)
	if e == nil || e.Code != errcode.ErrAccountLocked.Code {
		t.Fatalf("expected ErrAccountLocked, got %v", e)
	}

	// 快进时间，使锁定 key 过期
	mr.FastForward(loginLockDuration + time.Second)

	// 锁定到期后正确密码可以登录成功
	_, e = svc.Login(context.Background(), rightReq)
	if e != nil {
		t.Fatalf("expected success after lock expired, got %v", e)
	}
}
