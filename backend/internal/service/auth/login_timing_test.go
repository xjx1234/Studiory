package authservice

import (
	"context"
	"strings"
	"testing"

	"backend/internal/auth"
	"backend/internal/repo"
	"backend/internal/testutil"
	"backend/pkg/errcode"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// compareSpy 记录 bcrypt 比较接缝收到的 hash，用来确定性地断言「失败分支也付费一次」。
type compareSpy struct {
	hashes []string
}

// installCompareSpy 替换包内 bcryptCompare 接缝，测试结束后恢复。
func installCompareSpy(t *testing.T) *compareSpy {
	t.Helper()
	spy := &compareSpy{}
	orig := bcryptCompare
	bcryptCompare = func(hash, password []byte) error {
		spy.hashes = append(spy.hashes, string(hash))
		return orig(hash, password)
	}
	t.Cleanup(func() { bcryptCompare = orig })
	return spy
}

// TestDummyPasswordHashCostMatchesBcryptCost 盯住 dummy 的成本：它必须与注册写入的
// cost 一致，否则「等时」只是名义上的——比较更便宜的一侧仍然更快。
func TestDummyPasswordHashCostMatchesBcryptCost(t *testing.T) {
	cost, err := bcrypt.Cost(dummyPasswordHash)
	if err != nil {
		t.Fatalf("dummy hash is not a valid bcrypt hash: %v", err)
	}
	if cost != bcryptCost {
		t.Fatalf("dummy hash cost = %d, want bcryptCost = %d（调整 cost 后必须重新生成 dummy）", cost, bcryptCost)
	}
}

// TestLoginUnknownAccountStillPaysOneBcrypt 账号不存在时也要比较一次 dummy：
// 修复前这条路径完全不碰 bcrypt，响应比「密码错误」快一次 cost 10 的计算。
func TestLoginUnknownAccountStillPaysOneBcrypt(t *testing.T) {
	_, rdb := newTestRDB(t)
	svc := bfSvc(t, testutil.NewFakeUserRepo(), rdb)
	spy := installCompareSpy(t)

	_, e := svc.Login(context.Background(), &auth.LoginRequest{
		GrantType: auth.GrantTypePassword,
		Phone:     "13800009999",
		Password:  "whatever",
	})
	if e == nil || e.Code != errcode.ErrInvalidCredentials.Code {
		t.Fatalf("expected ErrInvalidCredentials, got %v", e)
	}
	if len(spy.hashes) != 1 {
		t.Fatalf("bcrypt compares = %d, want exactly 1", len(spy.hashes))
	}
	if spy.hashes[0] != string(dummyPasswordHash) {
		t.Errorf("unknown account must compare against dummy hash, got %q", spy.hashes[0])
	}
}

// TestLoginUserWithoutPasswordPaysDummyCompare 纯 OAuth 账号（PasswordHash 为空）
// 是与「账号不存在」并列的第二个绕过入口，同样必须比较 dummy。
func TestLoginUserWithoutPasswordPaysDummyCompare(t *testing.T) {
	fakeRepo := testutil.NewFakeUserRepo()
	phone := "13800009998"
	id := uuid.New()
	phonePtr := phone
	fakeRepo.Users[id] = &repo.User{
		ID:       id,
		Phone:    &phonePtr,
		Nickname: "oauth-only",
		Role:     repo.RoleUser,
		Status:   repo.StatusActive,
	}

	_, rdb := newTestRDB(t)
	svc := bfSvc(t, fakeRepo, rdb)
	spy := installCompareSpy(t)

	_, e := svc.Login(context.Background(), &auth.LoginRequest{
		GrantType: auth.GrantTypePassword,
		Phone:     phone,
		Password:  "whatever",
	})
	if e == nil || e.Code != errcode.ErrInvalidCredentials.Code {
		t.Fatalf("expected ErrInvalidCredentials, got %v", e)
	}
	if len(spy.hashes) != 1 {
		t.Fatalf("bcrypt compares = %d, want exactly 1", len(spy.hashes))
	}
	if spy.hashes[0] != string(dummyPasswordHash) {
		t.Errorf("passwordless account must compare against dummy hash, got %q", spy.hashes[0])
	}
}

// TestLoginWrongPasswordUsesStoredHash 真实账号仍比较库里的 hash（而不是一律用 dummy），
// 顺带钉住「每条密码登录路径都恰好比较一次」。
func TestLoginWrongPasswordUsesStoredHash(t *testing.T) {
	fakeRepo := testutil.NewFakeUserRepo()
	phone := "13800009997"
	seedUserWithPassword(fakeRepo, phone, "correct")

	_, rdb := newTestRDB(t)
	svc := bfSvc(t, fakeRepo, rdb)
	spy := installCompareSpy(t)

	_, e := svc.Login(context.Background(), &auth.LoginRequest{
		GrantType: auth.GrantTypePassword,
		Phone:     phone,
		Password:  "wrong",
	})
	if e == nil || e.Code != errcode.ErrInvalidCredentials.Code {
		t.Fatalf("expected ErrInvalidCredentials, got %v", e)
	}
	if len(spy.hashes) != 1 {
		t.Fatalf("bcrypt compares = %d, want exactly 1", len(spy.hashes))
	}
	if spy.hashes[0] == string(dummyPasswordHash) {
		t.Fatal("existing account must compare against its stored hash, not dummy")
	}
	if !strings.HasPrefix(spy.hashes[0], "$2") {
		t.Errorf("expected a bcrypt hash, got %q", spy.hashes[0])
	}
}
