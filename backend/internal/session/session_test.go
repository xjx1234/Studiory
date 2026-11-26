package session

import (
	"context"
	"strconv"
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

// TestStore_CheckAccess_MergesRevokeAndSessionInOneRoundTrip 固定 P2-11：
// 吊销检查与 session 校验必须合并为一次 MGET，而不是两次独立查询。
func TestStore_CheckAccess_MergesRevokeAndSessionInOneRoundTrip(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := NewStore(rdb, "test", true, time.Hour, true)
	ctx := context.Background()

	sid := NewSessionID()
	if err := store.Register(ctx, "user-1", sid); err != nil {
		t.Fatal(err)
	}

	counter := &commandCounter{}
	rdb.AddHook(counter)

	now := time.Now()
	valid, err := store.CheckAccess(ctx, "user-1", sid, &now)
	if err != nil || !valid {
		t.Fatalf("CheckAccess() = (%v, %v), want (true, nil)", valid, err)
	}
	if counter.total != 1 || counter.last != "mget" {
		t.Fatalf("redis commands = %d (last %q), want exactly 1 mget", counter.total, counter.last)
	}
}

// TestStore_CheckAccess_Scenarios 覆盖吊销、签发时间、session 失效与 userID 形式。
func TestStore_CheckAccess_Scenarios(t *testing.T) {
	uid := uuid.New()
	canonical := uid.String()
	issuedAt := time.Now()

	newStore := func(t *testing.T) (*Store, *miniredis.Miniredis) {
		t.Helper()
		mr := miniredis.RunT(t)
		rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		return NewStore(rdb, "test", true, time.Hour, true), mr
	}

	t.Run("revoke timestamp after issue", func(t *testing.T) {
		store, mr := newStore(t)
		sid := NewSessionID()
		if err := store.Register(context.Background(), canonical, sid); err != nil {
			t.Fatal(err)
		}
		if err := mr.Set("test:revoke:uid:"+canonical, strconv.FormatInt(issuedAt.Add(time.Minute).Unix(), 10)); err != nil {
			t.Fatal(err)
		}
		if valid, _ := store.CheckAccess(context.Background(), canonical, sid, &issuedAt); valid {
			t.Error("token issued before revoke must be rejected")
		}
	})

	t.Run("token issued after revoke", func(t *testing.T) {
		store, mr := newStore(t)
		sid := NewSessionID()
		if err := store.Register(context.Background(), canonical, sid); err != nil {
			t.Fatal(err)
		}
		if err := mr.Set("test:revoke:uid:"+canonical, strconv.FormatInt(issuedAt.Add(-time.Minute).Unix(), 10)); err != nil {
			t.Fatal(err)
		}
		if valid, _ := store.CheckAccess(context.Background(), canonical, sid, &issuedAt); !valid {
			t.Error("token issued after revoke should stay valid")
		}
	})

	t.Run("claim uid forms hit canonical revoke key", func(t *testing.T) {
		store, mr := newStore(t)
		sid := NewSessionID()
		if err := store.Register(context.Background(), canonical, sid); err != nil {
			t.Fatal(err)
		}
		if err := mr.Set("test:revoke:uid:"+canonical, strconv.FormatInt(issuedAt.Unix(), 10)); err != nil {
			t.Fatal(err)
		}
		for _, form := range []string{canonical, strings.ToUpper(canonical), "{" + canonical + "}"} {
			if valid, _ := store.CheckAccess(context.Background(), form, sid, &issuedAt); valid {
				t.Errorf("claim uid %q 没有命中 canonical 吊销键", form)
			}
		}
	})

	t.Run("missing session", func(t *testing.T) {
		store, _ := newStore(t)
		if valid, _ := store.CheckAccess(context.Background(), canonical, NewSessionID(), &issuedAt); valid {
			t.Error("unknown session must be rejected")
		}
	})

	t.Run("session bound to another user", func(t *testing.T) {
		store, _ := newStore(t)
		sid := NewSessionID()
		if err := store.Register(context.Background(), canonical, sid); err != nil {
			t.Fatal(err)
		}
		if valid, _ := store.CheckAccess(context.Background(), uuid.New().String(), sid, &issuedAt); valid {
			t.Error("session of another user must be rejected")
		}
	})

	t.Run("redis unavailable returns error", func(t *testing.T) {
		store, mr := newStore(t)
		mr.Close()
		if _, err := store.CheckAccess(context.Background(), canonical, NewSessionID(), &issuedAt); err == nil {
			t.Error("expected error when redis is unavailable so caller can apply failOpen")
		}
	})
}

// commandCounter 统计经 go-redis 发出的命令数，用来断言往返次数。
type commandCounter struct {
	total int
	last  string
}

func (h *commandCounter) DialHook(next redis.DialHook) redis.DialHook { return next }

func (h *commandCounter) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		h.total++
		h.last = cmd.Name()
		return next(ctx, cmd)
	}
}

func (h *commandCounter) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
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
