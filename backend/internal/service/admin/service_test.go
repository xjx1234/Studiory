package adminservice

import (
	"context"
	"testing"
	"time"

	"backend/internal/repo"
	"backend/internal/testutil"
	"backend/pkg/errcode"
	"backend/pkg/pagination"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func strPtr(s string) *string { return &s }

func seedUser(r *testutil.FakeUserRepo, role, status string, phone string, created time.Time) *repo.User {
	id := uuid.New()
	u := &repo.User{
		ID:        id,
		Phone:     strPtr(phone),
		Nickname:  "u-" + phone,
		Role:      role,
		Status:    status,
		CreatedAt: created,
		UpdatedAt: created,
	}
	r.Users[id] = u
	return u
}

func newRDB(t *testing.T) (*miniredis.Miniredis, redis.UniversalClient) {
	t.Helper()
	mr := miniredis.RunT(t)
	return mr, redis.NewClient(&redis.Options{Addr: mr.Addr()})
}

func page(p, size int) pagination.Query { return pagination.Query{Page: p, PageSize: size} }

func TestListUsers_PaginationAndOrder(t *testing.T) {
	r := testutil.NewFakeUserRepo()
	base := time.Now()
	seedUser(r, repo.RoleUser, repo.StatusActive, "13800000001", base.Add(1*time.Second))
	seedUser(r, repo.RoleUser, repo.StatusActive, "13800000002", base.Add(2*time.Second))
	seedUser(r, repo.RoleAdmin, repo.StatusActive, "13800000003", base.Add(3*time.Second))

	svc := New(r)

	list, e := svc.ListUsers(context.Background(), ListInput{}, page(1, 2))
	if e != nil {
		t.Fatalf("list failed: %+v", e)
	}
	if list.Total != 3 {
		t.Fatalf("total = %d, want 3", list.Total)
	}
	if len(list.Items) != 2 {
		t.Fatalf("items = %d, want 2 (page size)", len(list.Items))
	}
	// 创建时间倒序：最新的 13800000003 在最前
	if list.Items[0].Phone != "13800000003" {
		t.Fatalf("first item phone = %s, want newest 13800000003", list.Items[0].Phone)
	}
}

func TestListUsers_StatusFilter(t *testing.T) {
	r := testutil.NewFakeUserRepo()
	now := time.Now()
	seedUser(r, repo.RoleUser, repo.StatusActive, "13800000001", now)
	seedUser(r, repo.RoleUser, repo.StatusDisabled, "13800000002", now)

	svc := New(r)

	list, e := svc.ListUsers(context.Background(), ListInput{Status: repo.StatusDisabled}, page(1, 20))
	if e != nil {
		t.Fatalf("list failed: %+v", e)
	}
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].Status != repo.StatusDisabled {
		t.Fatalf("unexpected filtered result: %+v", list)
	}
}

func TestListUsers_InvalidStatus(t *testing.T) {
	svc := New(testutil.NewFakeUserRepo())
	_, e := svc.ListUsers(context.Background(), ListInput{Status: "bogus"}, page(1, 20))
	if e == nil || e.Code != errcode.ErrBadRequest.Code {
		t.Fatalf("expected ErrBadRequest, got %+v", e)
	}
}

func TestGetUser_NotFound(t *testing.T) {
	svc := New(testutil.NewFakeUserRepo())
	_, e := svc.GetUser(context.Background(), uuid.New().String())
	if e == nil || e.Code != errcode.ErrNotFound.Code {
		t.Fatalf("expected ErrNotFound, got %+v", e)
	}
}

func TestUpdateRole_Success(t *testing.T) {
	r := testutil.NewFakeUserRepo()
	target := seedUser(r, repo.RoleUser, repo.StatusActive, "13800000001", time.Now())
	svc := New(r)

	item, e := svc.UpdateRole(context.Background(), uuid.New().String(), target.ID.String(), repo.RoleAdmin)
	if e != nil {
		t.Fatalf("update role failed: %+v", e)
	}
	if item.Role != repo.RoleAdmin {
		t.Fatalf("role = %s, want admin", item.Role)
	}
}

func TestUpdateRole_InvalidRole(t *testing.T) {
	svc := New(testutil.NewFakeUserRepo())
	_, e := svc.UpdateRole(context.Background(), uuid.New().String(), uuid.New().String(), "superuser")
	if e == nil || e.Code != errcode.ErrBadRequest.Code {
		t.Fatalf("expected ErrBadRequest, got %+v", e)
	}
}

func TestUpdateRole_CannotModifySelf(t *testing.T) {
	r := testutil.NewFakeUserRepo()
	self := seedUser(r, repo.RoleAdmin, repo.StatusActive, "13800000001", time.Now())
	svc := New(r)

	_, e := svc.UpdateRole(context.Background(), self.ID.String(), self.ID.String(), repo.RoleUser)
	if e == nil || e.Code != errcode.ErrCannotModifySelf.Code {
		t.Fatalf("expected ErrCannotModifySelf, got %+v", e)
	}
}

func TestSetStatus_DisableRevokesTokens(t *testing.T) {
	r := testutil.NewFakeUserRepo()
	target := seedUser(r, repo.RoleUser, repo.StatusActive, "13800000001", time.Now())

	mr, rdb := newRDB(t)
	svc := New(r, WithRevokeSupport(rdb, "test", time.Hour))

	item, e := svc.SetStatus(context.Background(), uuid.New().String(), target.ID.String(), repo.StatusDisabled)
	if e != nil {
		t.Fatalf("set status failed: %+v", e)
	}
	if item.Status != repo.StatusDisabled {
		t.Fatalf("status = %s, want disabled", item.Status)
	}

	revokeKey := "test:revoke:uid:" + target.ID.String()
	if !mr.Exists(revokeKey) {
		t.Fatalf("expected revoke key %s to be set after disabling", revokeKey)
	}
}

func TestSetStatus_CannotDisableSelf(t *testing.T) {
	r := testutil.NewFakeUserRepo()
	self := seedUser(r, repo.RoleAdmin, repo.StatusActive, "13800000001", time.Now())
	svc := New(r)

	_, e := svc.SetStatus(context.Background(), self.ID.String(), self.ID.String(), repo.StatusDisabled)
	if e == nil || e.Code != errcode.ErrCannotModifySelf.Code {
		t.Fatalf("expected ErrCannotModifySelf, got %+v", e)
	}
}

func TestSetStatus_SelfReactivateAllowed(t *testing.T) {
	r := testutil.NewFakeUserRepo()
	self := seedUser(r, repo.RoleAdmin, repo.StatusActive, "13800000001", time.Now())
	svc := New(r)

	// 允许对自己设置 active（仅禁用自己被拦截）
	if _, e := svc.SetStatus(context.Background(), self.ID.String(), self.ID.String(), repo.StatusActive); e != nil {
		t.Fatalf("self re-activate should be allowed, got %+v", e)
	}
}
