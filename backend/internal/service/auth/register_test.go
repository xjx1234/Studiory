package authservice

import (
	"context"
	"testing"

	"backend/internal/repo"
	"backend/internal/testutil"
	"backend/pkg/errcode"

	"github.com/redis/go-redis/v9"
)

func registerSvc(t *testing.T, rdb redis.UniversalClient) *AuthServiceImpl {
	t.Helper()
	svc, ok := New(testutil.NewFakeUserRepo(), NewRedisCacheStore(rdb),
		WithTokenIssuer(testTokenIssuer()),
		WithMockCodeFallback(true),
	).(*AuthServiceImpl)
	if !ok {
		t.Fatal("expected *AuthServiceImpl")
	}
	return svc
}

// TestRegisterWithPassword_Success 验证密码注册成功并返回 Token。
func TestRegisterWithPassword_Success(t *testing.T) {
	_, rdb := newTestRDB(t)
	svc := registerSvc(t, rdb)
	fakeRepo := svc.users.(*testutil.FakeUserRepo)

	result, e := svc.Register(context.Background(), &RegisterInput{
		GrantType: "password",
		Phone:     "13900000001",
		Password:  "P@ssw0rd!",
		Nickname:  "测试用户A",
	})
	if e != nil {
		t.Fatalf("register failed: %+v", e)
	}
	if result == nil || result.Tokens == nil || result.User == nil {
		t.Fatal("expected login result with tokens and user")
	}
	if result.User.Nickname != "测试用户A" {
		t.Errorf("nickname = %q, want %q", result.User.Nickname, "测试用户A")
	}
	if len(fakeRepo.Created) != 1 {
		t.Fatalf("expected 1 created user, got %d", len(fakeRepo.Created))
	}
	if fakeRepo.Created[0].PasswordHash == nil || *fakeRepo.Created[0].PasswordHash == "" {
		t.Error("expected password hash to be set")
	}
}

// TestRegisterWithPassword_EmptyPassword 验证空密码返回 ErrBadRequest。
func TestRegisterWithPassword_EmptyPassword(t *testing.T) {
	_, rdb := newTestRDB(t)
	svc := registerSvc(t, rdb)

	_, e := svc.Register(context.Background(), &RegisterInput{
		GrantType: "password",
		Phone:     "13900000002",
		Password:  "",
	})
	if e != errcode.ErrBadRequest {
		t.Fatalf("expected ErrBadRequest, got %+v", e)
	}
}

// TestRegisterWithPassword_AlreadyExists 验证重复注册返回 ErrAlreadyExists。
func TestRegisterWithPassword_AlreadyExists(t *testing.T) {
	_, rdb := newTestRDB(t)
	svc := registerSvc(t, rdb)
	fakeRepo := svc.users.(*testutil.FakeUserRepo)

	// 预置一个已存在用户
	seedUserWithPassword(fakeRepo, "13900000003", "existing")

	_, e := svc.Register(context.Background(), &RegisterInput{
		GrantType: "password",
		Phone:     "13900000003",
		Password:  "AnotherP@ss1",
	})
	if e != errcode.ErrAlreadyExists {
		t.Fatalf("expected ErrAlreadyExists, got %+v", e)
	}
}

// TestRegisterWithCode_Success 验证验证码注册成功。
func TestRegisterWithCode_Success(t *testing.T) {
	_, rdb := newTestRDB(t)
	svc := registerSvc(t, rdb)

	// 先发送验证码（mock fallback → 固定码 123456）
	if e := svc.SendCode(context.Background(), "email", "newuser@example.com"); e != nil {
		t.Fatalf("SendCode failed: %+v", e)
	}

	result, e := svc.Register(context.Background(), &RegisterInput{
		GrantType: "code",
		CodeType:  "email",
		Email:     "newuser@example.com",
		Code:      MockVerificationCode,
	})
	if e != nil {
		t.Fatalf("register with code failed: %+v", e)
	}
	if result == nil || result.Tokens == nil {
		t.Fatal("expected login result with tokens")
	}
}

// TestRegisterWithCode_InvalidCode 验证错误验证码被拒绝。
func TestRegisterWithCode_InvalidCode(t *testing.T) {
	_, rdb := newTestRDB(t)
	svc := registerSvc(t, rdb)

	if e := svc.SendCode(context.Background(), "sms", "13900000004"); e != nil {
		t.Fatalf("SendCode failed: %+v", e)
	}

	_, e := svc.Register(context.Background(), &RegisterInput{
		GrantType: "code",
		CodeType:  "sms",
		Phone:     "13900000004",
		Code:      "999999",
	})
	if e != errcode.ErrInvalidCode {
		t.Fatalf("expected ErrInvalidCode, got %+v", e)
	}
}

// TestRegisterWithCode_LocksAfterTooManyFailures 验证验证码注册猜错达到阈值后锁定。
func TestRegisterWithCode_LocksAfterTooManyFailures(t *testing.T) {
	_, rdb := newTestRDB(t)
	svc := registerSvc(t, rdb)
	phone := "13900000010"

	if e := svc.SendCode(context.Background(), "sms", phone); e != nil {
		t.Fatalf("SendCode failed: %+v", e)
	}

	wrong := &RegisterInput{
		GrantType: "code",
		CodeType:  "sms",
		Phone:     phone,
		Code:      "999999",
	}
	for i := 0; i < loginMaxFailAttempts; i++ {
		_, e := svc.Register(context.Background(), wrong)
		switch {
		case e == nil:
			t.Fatalf("attempt %d: expected error, got nil", i+1)
		case e.Code == errcode.ErrAccountLocked.Code:
			t.Fatalf("account locked too early at attempt %d", i+1)
		case e.Code != errcode.ErrInvalidCode.Code:
			t.Fatalf("attempt %d: expected ErrInvalidCode, got %+v", i+1, e)
		}
	}

	_, e := svc.Register(context.Background(), wrong)
	if e == nil || e.Code != errcode.ErrAccountLocked.Code {
		t.Fatalf("expected ErrAccountLocked after %d failures, got %+v", loginMaxFailAttempts, e)
	}

	// 即使验证码正确，锁定期间也不应放行。
	_, e = svc.Register(context.Background(), &RegisterInput{
		GrantType: "code",
		CodeType:  "sms",
		Phone:     phone,
		Code:      MockVerificationCode,
	})
	if e == nil || e.Code != errcode.ErrAccountLocked.Code {
		t.Fatalf("expected ErrAccountLocked for correct code while locked, got %+v", e)
	}
}

// TestRegisterWithCode_ClearsCounterOnSuccess 验证注册成功后清除失败计数。
func TestRegisterWithCode_ClearsCounterOnSuccess(t *testing.T) {
	mr, rdb := newTestRDB(t)
	svc := registerSvc(t, rdb)
	phone := "13900000011"

	if e := svc.SendCode(context.Background(), "sms", phone); e != nil {
		t.Fatalf("SendCode failed: %+v", e)
	}

	wrong := &RegisterInput{GrantType: "code", CodeType: "sms", Phone: phone, Code: "999999"}
	for i := 0; i < 2; i++ {
		_, _ = svc.Register(context.Background(), wrong)
	}

	_, e := svc.Register(context.Background(), &RegisterInput{
		GrantType: "code",
		CodeType:  "sms",
		Phone:     phone,
		Code:      MockVerificationCode,
	})
	if e != nil {
		t.Fatalf("expected successful register, got %+v", e)
	}

	failKey := svc.loginFailKey(loginAccountID(phone, "", ""))
	if mr.Exists(failKey) {
		t.Error("expected fail key to be deleted after successful code register")
	}
}

// TestRegisterWithCode_MissingFields 验证缺少 code 或 codeType 返回 ErrBadRequest。
func TestRegisterWithCode_MissingFields(t *testing.T) {
	_, rdb := newTestRDB(t)
	svc := registerSvc(t, rdb)

	tests := []struct {
		name  string
		input RegisterInput
	}{
		{"empty code", RegisterInput{GrantType: "code", CodeType: "sms", Phone: "13900000005", Code: ""}},
		{"empty codeType", RegisterInput{GrantType: "code", CodeType: "", Phone: "13900000005", Code: "123456"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, e := svc.Register(context.Background(), &tt.input)
			if e != errcode.ErrBadRequest {
				t.Fatalf("expected ErrBadRequest, got %+v", e)
			}
		})
	}
}

// TestRegister_UnsupportedGrantType 验证不支持的注册类型返回 ErrUnsupportedGrant。
func TestRegister_UnsupportedGrantType(t *testing.T) {
	_, rdb := newTestRDB(t)
	svc := registerSvc(t, rdb)

	_, e := svc.Register(context.Background(), &RegisterInput{
		GrantType: "unknown",
	})
	if e != errcode.ErrUnsupportedGrant {
		t.Fatalf("expected ErrUnsupportedGrant, got %+v", e)
	}
}

// TestDefaultNickname 验证默认昵称生成逻辑。
func TestDefaultNickname(t *testing.T) {
	svc := &AuthServiceImpl{}

	tests := []struct {
		name     string
		input    RegisterInput
		expected string
	}{
		{"explicit nickname", RegisterInput{Nickname: "自定义"}, "自定义"},
		{"phone suffix", RegisterInput{Phone: "13900001234"}, "用户1234"},
		{"email prefix", RegisterInput{Email: "alice@example.com"}, "用户_alice"},
		{"fallback", RegisterInput{}, "新用户"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.defaultNickname(&tt.input)
			if got != tt.expected {
				t.Errorf("defaultNickname() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// TestRegisterWithPassword_DisabledUser 验证注册时用户被禁用不会影响创建（新用户默认 active）。
func TestRegisterWithPassword_CreatesActiveUser(t *testing.T) {
	_, rdb := newTestRDB(t)
	svc := registerSvc(t, rdb)
	fakeRepo := svc.users.(*testutil.FakeUserRepo)

	result, e := svc.Register(context.Background(), &RegisterInput{
		GrantType: "password",
		Email:     "active@example.com",
		Password:  "P@ssw0rd!",
	})
	if e != nil {
		t.Fatalf("register failed: %+v", e)
	}
	if result.User.Role != repo.RoleUser {
		t.Errorf("role = %q, want %q", result.User.Role, repo.RoleUser)
	}
	if len(fakeRepo.Created) != 1 {
		t.Fatalf("expected 1 created user")
	}
	if fakeRepo.Created[0].Status != repo.StatusActive {
		t.Errorf("status = %q, want %q", fakeRepo.Created[0].Status, repo.StatusActive)
	}
}
