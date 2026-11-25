package authservice

import (
	"context"
	"testing"
	"time"

	"backend/internal/auth"
	"backend/internal/repo"
	"backend/internal/testutil"
	"backend/pkg/errcode"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// 标识归一化：同一个手机号/邮箱的不同书写形式必须落到同一个账号。
// 不成立时，换一种大小写就能绕过验证码冷却与日配额（直接烧短信费）、
// 注册出重复账号，或把防暴力破解的失败计数拆进多个桶里。

func TestSendCode_EmailCaseVariantsShareKeys(t *testing.T) {
	_, rdb := newTestRDB(t)
	svc, snd := quotaSvc(t, rdb, WithCodeQuota(5, 0))
	ctx := context.Background()

	if e := svc.SendCode(ctx, "email", "Alice@Example.com", "1.1.1.1"); e != nil {
		t.Fatalf("first SendCode: %v", e)
	}
	if e := svc.SendCode(ctx, "email", "ALICE@example.COM", "1.1.1.1"); e != errcode.ErrTooManyRequests {
		t.Fatalf("case-variant SendCode error = %+v, want ErrTooManyRequests (cooldown)", e)
	}
	if snd.calls != 1 {
		t.Errorf("sender calls = %d, want 1 (变体必须共用冷却，不能再次下发)", snd.calls)
	}

	if n, _ := rdb.Exists(ctx, svc.codeRedisKey("email", "alice@example.com")).Result(); n != 1 {
		t.Errorf("code not stored under canonical key, exists=%d", n)
	}
	if got, _ := rdb.Get(ctx, svc.codeQuotaKey("email", "  ALICE@Example.com ")).Int(); got != 1 {
		t.Errorf("quota counter under canonical key = %d, want 1", got)
	}
}

func TestSendCode_PhoneFormatVariantsShareKeys(t *testing.T) {
	_, rdb := newTestRDB(t)
	svc, snd := quotaSvc(t, rdb, WithCodeQuota(5, 0))
	ctx := context.Background()

	if e := svc.SendCode(ctx, "sms", "13800138000", "1.1.1.1"); e != nil {
		t.Fatalf("first SendCode: %v", e)
	}
	if e := svc.SendCode(ctx, "sms", "+86 138-0013-8000", "1.1.1.1"); e != errcode.ErrTooManyRequests {
		t.Fatalf("format-variant SendCode error = %+v, want ErrTooManyRequests (cooldown)", e)
	}
	if snd.calls != 1 {
		t.Errorf("sender calls = %d, want 1", snd.calls)
	}
	if got, _ := rdb.Get(ctx, svc.codeQuotaKey("sms", "13800138000")).Int(); got != 1 {
		t.Errorf("quota counter = %d, want 1", got)
	}
}

func TestRegister_EmailCaseVariantsDoNotDuplicate(t *testing.T) {
	mr, rdb := newTestRDB(t)
	svc := registerSvc(t, rdb)
	fakeRepo := svc.users.(*testutil.FakeUserRepo)
	ctx := context.Background()

	if e := svc.SendCode(ctx, "email", "Alice@Example.com", "1.1.1.1"); e != nil {
		t.Fatalf("SendCode: %v", e)
	}
	if _, e := svc.Register(ctx, &RegisterInput{
		GrantType: "code", CodeType: "email", Email: "Alice@Example.com", Code: MockVerificationCode,
	}); e != nil {
		t.Fatalf("first register: %+v", e)
	}
	if len(fakeRepo.Created) != 1 {
		t.Fatalf("created users = %d, want 1", len(fakeRepo.Created))
	}
	stored := fakeRepo.Created[0].Email
	if stored == nil || *stored != "alice@example.com" {
		t.Fatalf("stored email = %v, want canonical %q", stored, "alice@example.com")
	}

	// 冷却过后重新收码，再用另一种大小写注册：必须撞到「已存在」，而不是第二个账号。
	mr.FastForward(61 * time.Second) // 越过冷却窗口，仍在 24h 配额窗口内
	if e := svc.SendCode(ctx, "email", "ALICE@example.com", "1.1.1.1"); e != nil {
		t.Fatalf("second SendCode: %v", e)
	}
	if _, e := svc.Register(ctx, &RegisterInput{
		GrantType: "code", CodeType: "email", Email: "ALICE@example.com", Code: MockVerificationCode,
	}); e != errcode.ErrAlreadyExists {
		t.Fatalf("re-register with case variant error = %+v, want ErrAlreadyExists", e)
	}
	if len(fakeRepo.Created) != 1 {
		t.Errorf("created users = %d, want 1 (大小写变体不得注册出第二个账号)", len(fakeRepo.Created))
	}
}

func TestLogin_AccountCaseVariantsResolveSameUser(t *testing.T) {
	_, rdb := newTestRDB(t)
	fakeRepo := testutil.NewFakeUserRepo()
	hash, err := bcrypt.GenerateFromPassword([]byte("P@ssw0rd!"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	hashStr := string(hash)
	email := "alice@example.com"
	id := uuid.New()
	fakeRepo.Users[id] = &repo.User{ID: id, Email: &email, PasswordHash: &hashStr, Nickname: "u", Role: repo.RoleUser}
	svc := bfSvc(t, fakeRepo, rdb)

	res, e := svc.loginWithPassword(context.Background(), &auth.LoginRequest{
		GrantType: auth.GrantTypePassword,
		Account:   "  Alice@EXAMPLE.com ",
		Password:  "P@ssw0rd!",
	})
	if e != nil {
		t.Fatalf("login with case/format variant account: %+v", e)
	}
	if res.User.ID != id.String() {
		t.Errorf("logged in user = %s, want %s", res.User.ID, id)
	}
}

func TestLoginFailuresAcrossAccountVariantsShareLockBucket(t *testing.T) {
	_, rdb := newTestRDB(t)
	fakeRepo := testutil.NewFakeUserRepo()
	hashStr := "P@ssw0rd!"
	hash, err := bcrypt.GenerateFromPassword([]byte(hashStr), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	stored := string(hash)
	email := "alice@example.com"
	id := uuid.New()
	fakeRepo.Users[id] = &repo.User{ID: id, Email: &email, PasswordHash: &stored, Nickname: "u", Role: repo.RoleUser}
	svc := bfSvc(t, fakeRepo, rdb)
	ctx := context.Background()

	// 失败计数故意分散在不同书写形式上：归一化后它们同属一个桶，第 5 次即触发锁定。
	for _, account := range []string{
		"alice@example.com", "Alice@Example.com", "ALICE@example.com",
		"alice@EXAMPLE.com", "Alice@example.COM",
	} {
		if _, e := svc.loginWithPassword(ctx, &auth.LoginRequest{
			GrantType: auth.GrantTypePassword, Account: account, Password: "wrong",
		}); e != errcode.ErrInvalidCredentials {
			t.Fatalf("attempt for %q error = %+v, want ErrInvalidCredentials", account, e)
		}
	}

	// 用正确密码 + 另一种写法也必须仍是锁定态，否则换写法即可继续爆破。
	if _, e := svc.loginWithPassword(ctx, &auth.LoginRequest{
		GrantType: auth.GrantTypePassword, Account: "Alice@Example.com", Password: hashStr,
	}); e != errcode.ErrAccountLocked {
		t.Fatalf("after threshold error = %+v, want ErrAccountLocked", e)
	}
}
