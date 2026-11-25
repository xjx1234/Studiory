package session

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func testStore(t *testing.T, multiDevice bool) (*Store, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return NewStore(rdb, "test", multiDevice, time.Hour, true), mr
}

func TestStore_MultiDevice_TwoSessions(t *testing.T) {
	store, _ := testStore(t, true)
	ctx := context.Background()
	uid := "user-1"
	sid1, sid2 := NewSessionID(), NewSessionID()

	if err := store.Register(ctx, uid, sid1); err != nil {
		t.Fatal(err)
	}
	if err := store.Register(ctx, uid, sid2); err != nil {
		t.Fatal(err)
	}

	if !store.Validate(ctx, uid, sid1) || !store.Validate(ctx, uid, sid2) {
		t.Fatal("both sessions should be valid in multi-device mode")
	}
}

func TestStore_SingleDevice_NewLoginKicksOld(t *testing.T) {
	store, _ := testStore(t, false)
	ctx := context.Background()
	uid := "user-1"
	sid1, sid2 := NewSessionID(), NewSessionID()

	if err := store.Register(ctx, uid, sid1); err != nil {
		t.Fatal(err)
	}
	if err := store.Register(ctx, uid, sid2); err != nil {
		t.Fatal(err)
	}

	if store.Validate(ctx, uid, sid1) {
		t.Fatal("old session should be invalid after new login in single-device mode")
	}
	if !store.Validate(ctx, uid, sid2) {
		t.Fatal("new session should be valid")
	}
}

func TestStore_RevokeSingleSession_MultiDevice(t *testing.T) {
	store, _ := testStore(t, true)
	ctx := context.Background()
	uid := "user-1"
	sid1, sid2 := NewSessionID(), NewSessionID()

	_ = store.Register(ctx, uid, sid1)
	_ = store.Register(ctx, uid, sid2)

	if err := store.Revoke(ctx, uid, sid1); err != nil {
		t.Fatal(err)
	}

	if store.Validate(ctx, uid, sid1) {
		t.Fatal("revoked session should be invalid")
	}
	if !store.Validate(ctx, uid, sid2) {
		t.Fatal("other device session should remain valid")
	}
}

func TestStore_RevokeAll(t *testing.T) {
	store, _ := testStore(t, true)
	ctx := context.Background()
	uid := "user-1"
	sid1, sid2 := NewSessionID(), NewSessionID()

	_ = store.Register(ctx, uid, sid1)
	_ = store.Register(ctx, uid, sid2)

	if err := store.RevokeAll(ctx, uid); err != nil {
		t.Fatal(err)
	}
	if store.Validate(ctx, uid, sid1) || store.Validate(ctx, uid, sid2) {
		t.Fatal("all sessions should be revoked")
	}
}

// TestStore_UserIDFormsShareOneKeySpace 验证会话/吊销键不受 UUID 书写形式影响：
// 登录时用 canonical 形式注册，管理端用大写/花括号/urn 形式吊销，也必须命中同一把键。
// 修复前 RevokeAll 会对错误形式的键做 SMembers（返回空集），禁用账号表面成功、旧 token 照样能用。
func TestStore_UserIDFormsShareOneKeySpace(t *testing.T) {
	uid := uuid.New()
	canonical := uid.String()
	forms := []string{
		canonical,
		strings.ToUpper(canonical),
		"{" + canonical + "}",
		"urn:uuid:" + canonical,
	}

	for _, multiDevice := range []bool{true, false} {
		mode := "single-device"
		if multiDevice {
			mode = "multi-device"
		}
		t.Run(mode, func(t *testing.T) {
			for _, form := range forms {
				t.Run(form, func(t *testing.T) {
					store, _ := testStore(t, multiDevice)
					ctx := context.Background()
					sid := NewSessionID()

					if err := store.Register(ctx, canonical, sid); err != nil {
						t.Fatalf("register: %v", err)
					}
					if !store.Validate(ctx, form, sid) {
						t.Fatalf("Validate(%q) = false, want true：读取方形同不一致", form)
					}
					if err := store.RevokeAll(ctx, form); err != nil {
						t.Fatalf("revoke all: %v", err)
					}
					if store.Validate(ctx, canonical, sid) {
						t.Errorf("RevokeAll(%q) 没有吊销用 canonical 形式注册的会话", form)
					}
				})
			}
		})
	}
}

// TestStore_SessionExpiresAbsolutely 固定「绝对过期」语义：
// 同一用户后续登录会顺带续期 sessions:uid 集合，但绝不能复活已到期的旧 sid。
// 修复前 Validate 先看集合成员，集合被续期后旧 sid 会被误判为有效。
func TestStore_SessionExpiresAbsolutely(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := NewStore(rdb, "test", true, time.Hour, false)
	ctx := context.Background()
	uid := "user-1"
	sid1, sid2 := NewSessionID(), NewSessionID()

	if err := store.Register(ctx, uid, sid1); err != nil {
		t.Fatal(err)
	}

	mr.FastForward(40 * time.Minute)
	// 第二次登录会把 sessions:uid 集合的 TTL 续到「此刻 + 1h」。
	if err := store.Register(ctx, uid, sid2); err != nil {
		t.Fatal(err)
	}

	mr.FastForward(30 * time.Minute) // sid1 已存续 70 分钟，sid2 仅 30 分钟

	if store.Validate(ctx, uid, sid1) {
		t.Error("sid1 已超过绝对 TTL，集合续期不得让它复活")
	}
	if !store.Validate(ctx, uid, sid2) {
		t.Error("sid2 仍在 TTL 内，应判定有效")
	}
	if store.Validate(ctx, "user-2", sid2) {
		t.Error("session 键的 userID 不匹配时必须拒绝")
	}
}

func TestValidate_FailOpenOnRedisError(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := NewStore(rdb, "test", true, time.Hour, true) // fail-open
	ctx := context.Background()

	// 先注册一个 session
	sid := NewSessionID()
	if err := store.Register(ctx, "user-1", sid); err != nil {
		t.Fatal(err)
	}

	// 关闭 Redis 模拟故障
	mr.Close()

	// fail-open：Redis 故障时应放行
	if !store.Validate(ctx, "user-1", sid) {
		t.Error("expected Validate to return true (fail-open) when redis is unavailable")
	}
}

func TestValidate_FailClosedOnRedisError(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := NewStore(rdb, "test", true, time.Hour, false) // fail-closed
	ctx := context.Background()

	// 先注册一个 session
	sid := NewSessionID()
	if err := store.Register(ctx, "user-1", sid); err != nil {
		t.Fatal(err)
	}

	// 关闭 Redis 模拟故障
	mr.Close()

	// fail-closed：Redis 故障时应拒绝
	if store.Validate(ctx, "user-1", sid) {
		t.Error("expected Validate to return false (fail-closed) when redis is unavailable")
	}
}
