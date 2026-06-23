package session

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func testStore(t *testing.T, multiDevice bool) (*Store, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return NewStore(rdb, "test", multiDevice, time.Hour), mr
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
