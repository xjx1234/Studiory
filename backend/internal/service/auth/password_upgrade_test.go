package authservice

import (
	"context"
	"errors"
	"testing"

	"backend/internal/auth"
	"backend/internal/repo"
	"backend/internal/testutil"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func seedUserWithCost(t *testing.T, fakeRepo *testutil.FakeUserRepo, phone, password string, cost int) *repo.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		t.Fatalf("generate bcrypt hash at cost %d: %v", cost, err)
	}
	hashStr := string(hash)
	phonePtr := phone
	id := uuid.New()
	u := &repo.User{
		ID:           id,
		Phone:        &phonePtr,
		PasswordHash: &hashStr,
		Nickname:     "升级用例",
		Role:         repo.RoleUser,
		Status:       repo.StatusActive,
	}
	fakeRepo.Users[id] = u
	return u
}

func passwordLoginReq(phone, password string) *auth.LoginRequest {
	return &auth.LoginRequest{GrantType: auth.GrantTypePassword, Phone: phone, Password: password}
}

// TestLoginUpgradesLowCostHash 库内 hash 的 cost 低于目标时，登录成功后应就地升级，
// 且升级后的 hash 仍能用同一密码验证。
func TestLoginUpgradesLowCostHash(t *testing.T) {
	fakeRepo := testutil.NewFakeUserRepo()
	user := seedUserWithCost(t, fakeRepo, "13800008888", "correct", bcrypt.MinCost)

	_, rdb := newTestRDB(t)
	svc := bfSvc(t, fakeRepo, rdb)

	if _, e := svc.Login(context.Background(), passwordLoginReq("13800008888", "correct")); e != nil {
		t.Fatalf("login failed: %+v", e)
	}

	got := *fakeRepo.Users[user.ID].PasswordHash
	cost, err := bcrypt.Cost([]byte(got))
	if err != nil {
		t.Fatalf("upgraded hash is not valid bcrypt: %v", err)
	}
	if cost != bcryptCost {
		t.Fatalf("upgraded cost = %d, want %d", cost, bcryptCost)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(got), []byte("correct")); err != nil {
		t.Fatalf("upgraded hash must still verify the same password: %v", err)
	}
	if fakeRepo.PasswordCASUpdates != 1 {
		t.Fatalf("password CAS updates = %d, want 1", fakeRepo.PasswordCASUpdates)
	}
}

// TestLoginSkipsUpgradeWhenCostIsCurrent cost 已达标时不能做无谓写入（否则每次登录都写库）。
func TestLoginSkipsUpgradeWhenCostIsCurrent(t *testing.T) {
	fakeRepo := testutil.NewFakeUserRepo()
	seedUserWithCost(t, fakeRepo, "13800008889", "correct", bcryptCost)

	_, rdb := newTestRDB(t)
	svc := bfSvc(t, fakeRepo, rdb)

	if _, e := svc.Login(context.Background(), passwordLoginReq("13800008889", "correct")); e != nil {
		t.Fatalf("login failed: %+v", e)
	}
	if fakeRepo.PasswordCASUpdates != 0 {
		t.Fatalf("password CAS updates = %d, want 0 when cost already meets target", fakeRepo.PasswordCASUpdates)
	}
}

// TestLoginSurvivesUpgradeFailure 升级写库失败只记日志：不能让一次成功登录变成 500，
// 也不能破坏库里的旧 hash。
func TestLoginSurvivesUpgradeFailure(t *testing.T) {
	fakeRepo := testutil.NewFakeUserRepo()
	user := seedUserWithCost(t, fakeRepo, "13800008890", "correct", bcrypt.MinCost)
	before := *user.PasswordHash

	_, rdb := newTestRDB(t)
	svc := bfSvc(t, fakeRepo, rdb)
	fakeRepo.UpdatePasswordErr = errors.New("db down")

	if _, e := svc.Login(context.Background(), passwordLoginReq("13800008890", "correct")); e != nil {
		t.Fatalf("login must succeed even when the upgrade write fails: %+v", e)
	}
	if got := *fakeRepo.Users[user.ID].PasswordHash; got != before {
		t.Fatal("failed upgrade must not change the stored hash")
	}
}

// TestUpgradePasswordHashSkipsWhenHashChanged 模拟并发改密：CAS 的 expected 已过期时必须放弃，
// 否则会把旧密码的 hash 写回去，等于撤销用户刚做的改密。
func TestUpgradePasswordHashSkipsWhenHashChanged(t *testing.T) {
	fakeRepo := testutil.NewFakeUserRepo()
	user := seedUserWithCost(t, fakeRepo, "13800008891", "correct", bcrypt.MinCost)

	_, rdb := newTestRDB(t)
	svc := bfSvc(t, fakeRepo, rdb)

	// 登录请求读到的是旧 hash；在升级写库之前，另一条改密路径已经写入新 hash。
	stale := &repo.User{ID: user.ID, PasswordHash: user.PasswordHash}
	newer, err := bcrypt.GenerateFromPassword([]byte("changed-by-user"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("generate concurrent hash: %v", err)
	}
	newerStr := string(newer)
	fakeRepo.Users[user.ID].PasswordHash = &newerStr

	svc.upgradePasswordHash(context.Background(), stale, "correct")

	if got := *fakeRepo.Users[user.ID].PasswordHash; got != newerStr {
		t.Fatal("upgrade must not overwrite a hash that was changed concurrently")
	}
	if fakeRepo.PasswordCASUpdates != 0 {
		t.Fatalf("password CAS updates = %d, want 0 on CAS mismatch", fakeRepo.PasswordCASUpdates)
	}
}
