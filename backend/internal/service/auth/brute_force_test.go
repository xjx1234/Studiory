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
	svc, ok := New(repo, NewRedisCacheStore(rdb), WithTokenIssuer(testTokenIssuer())).(*AuthServiceImpl)
	if !ok {
		t.Fatal("expected *AuthServiceImpl")
	}
	return svc
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
		Role:         repo.RoleUser,
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
		switch {
		case e == nil:
			t.Fatalf("attempt %d: expected error, got nil", i+1)
		case e.Code == errcode.ErrAccountLocked.Code:
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

// TestCodeLoginLockedAfterMaxFailures 验证验证码登录连续失败后账号被锁定。
func TestCodeLoginLockedAfterMaxFailures(t *testing.T) {
	_, rdb := newTestRDB(t)
	fakeRepo := testutil.NewFakeUserRepo()
	phone := "13800000004"
	id := uuid.New()
	phoneCopy := phone
	fakeRepo.Users[id] = &repo.User{
		ID:       id,
		Phone:    &phoneCopy,
		Nickname: "code-login-user",
		Role:     repo.RoleUser,
		Status:   repo.StatusActive,
	}

	svc, ok := New(fakeRepo, NewRedisCacheStore(rdb),
		WithTokenIssuer(testTokenIssuer()),
		WithMockCodeFallback(true),
	).(*AuthServiceImpl)
	if !ok {
		t.Fatal("expected *AuthServiceImpl")
	}

	req := &auth.LoginRequest{
		GrantType: auth.GrantTypeSMSCode,
		Phone:     phone,
		Code:      "000000", // 错误验证码
	}

	for i := 0; i < loginMaxFailAttempts; i++ {
		_, e := svc.Login(context.Background(), req)
		switch {
		case e == nil:
			t.Fatalf("attempt %d: expected error, got nil", i+1)
		case e.Code == errcode.ErrAccountLocked.Code:
			t.Fatalf("account locked too early at attempt %d", i+1)
		}
	}

	// 超过阈值后应返回锁定错误
	_, e := svc.Login(context.Background(), req)
	if e == nil || e.Code != errcode.ErrAccountLocked.Code {
		t.Fatalf("expected ErrAccountLocked after %d code failures, got %v", loginMaxFailAttempts, e)
	}
}

// TestCodeLoginClearsCounterOnSuccess 验证验证码登录成功后清除失败计数。
func TestCodeLoginClearsCounterOnSuccess(t *testing.T) {
	mr, rdb := newTestRDB(t)
	fakeRepo := testutil.NewFakeUserRepo()
	phone := "13800000005"
	id := uuid.New()
	phoneCopy := phone
	fakeRepo.Users[id] = &repo.User{
		ID:       id,
		Phone:    &phoneCopy,
		Nickname: "code-login-user",
		Role:     repo.RoleUser,
		Status:   repo.StatusActive,
	}

	svc, ok := New(fakeRepo, NewRedisCacheStore(rdb),
		WithTokenIssuer(testTokenIssuer()),
		WithMockCodeFallback(true),
	).(*AuthServiceImpl)
	if !ok {
		t.Fatal("expected *AuthServiceImpl")
	}

	// 先发送验证码（mock fallback → 固定码 123456）
	if e := svc.SendCode(context.Background(), "sms", phone); e != nil {
		t.Fatalf("SendCode failed: %+v", e)
	}

	// 故意失败 2 次（用错误验证码）
	wrongReq := &auth.LoginRequest{GrantType: auth.GrantTypeSMSCode, Phone: phone, Code: "999999"}
	for i := 0; i < 2; i++ {
		svc.Login(context.Background(), wrongReq) //nolint:errcheck
	}

	// 快进冷却窗口，允许再次发送验证码
	mr.FastForward(codeSendCooldown + time.Second)

	// 再次发送验证码（前一次已被 verifyCode 消费）
	if e := svc.SendCode(context.Background(), "sms", phone); e != nil {
		t.Fatalf("second SendCode failed: %+v", e)
	}

	// 正确验证码登录
	rightReq := &auth.LoginRequest{GrantType: auth.GrantTypeSMSCode, Phone: phone, Code: MockVerificationCode}
	_, e := svc.Login(context.Background(), rightReq)
	if e != nil {
		t.Fatalf("expected success, got %v", e)
	}

	// fail key 应已被删除
	failKey := svc.loginFailKey(loginAccountID(phone, "", ""))
	if mr.Exists(failKey) {
		t.Error("expected fail key to be deleted after successful code login")
	}
}

// TestIsLoginLocked_FailClosed 验证 failOpen=false 时 Redis 故障视为已锁定。
func TestIsLoginLocked_FailClosed(t *testing.T) {
	mr, rdb := newTestRDB(t)
	fakeRepo := testutil.NewFakeUserRepo()

	svc, ok := New(fakeRepo, NewRedisCacheStore(rdb),
		WithTokenIssuer(testTokenIssuer()),
		WithRedisFailOpen(false), // fail-closed
	).(*AuthServiceImpl)
	if !ok {
		t.Fatal("expected *AuthServiceImpl")
	}

	// 关闭 Redis，模拟连接故障
	mr.Close()

	// fail-closed：Redis 故障时应视为已锁定
	if !svc.isLoginLocked(context.Background(), "some-account") {
		t.Error("expected isLoginLocked to return true (fail-closed) when redis is unavailable")
	}
}

// TestIsLoginLocked_FailOpen 验证 failOpen=true（默认）时 Redis 故障视为未锁定。
func TestIsLoginLocked_FailOpen(t *testing.T) {
	mr, rdb := newTestRDB(t)
	fakeRepo := testutil.NewFakeUserRepo()

	svc, ok := New(fakeRepo, NewRedisCacheStore(rdb),
		WithTokenIssuer(testTokenIssuer()),
		WithRedisFailOpen(true), // fail-open (default)
	).(*AuthServiceImpl)
	if !ok {
		t.Fatal("expected *AuthServiceImpl")
	}

	mr.Close()

	if svc.isLoginLocked(context.Background(), "some-account") {
		t.Error("expected isLoginLocked to return false (fail-open) when redis is unavailable")
	}
}
