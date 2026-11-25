package adminservice

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"backend/internal/repo"
	"backend/internal/session"
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
	if item.SessionsRevoked == nil || !*item.SessionsRevoked {
		t.Fatalf("role change should report sessions_revoked=true, got %v", item.SessionsRevoked)
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
	if item.SessionsRevoked == nil || !*item.SessionsRevoked {
		t.Fatalf("disable should report sessions_revoked=true, got %v", item.SessionsRevoked)
	}
}

// TestSetStatus_ReportsRevocationFailure 吊销写失败时数据库更新已生效，接口仍返回成功，
// 但必须把 sessions_revoked=false 告诉调用方，避免误以为旧 token 已即时失效。
func TestSetStatus_ReportsRevocationFailure(t *testing.T) {
	r := testutil.NewFakeUserRepo()
	target := seedUser(r, repo.RoleUser, repo.StatusActive, "13800000001", time.Now())

	mr, rdb := newRDB(t)
	mr.Close() // 让后续 Redis 写入失败
	svc := New(r, WithRevokeSupport(rdb, "test", time.Hour))

	item, e := svc.SetStatus(context.Background(), uuid.New().String(), target.ID.String(), repo.StatusDisabled)
	if e != nil {
		t.Fatalf("disable should succeed even when revocation fails: %+v", e)
	}
	if item.Status != repo.StatusDisabled {
		t.Fatalf("status = %s, want disabled", item.Status)
	}
	if item.SessionsRevoked == nil || *item.SessionsRevoked {
		t.Fatalf("expected sessions_revoked=false, got %v", item.SessionsRevoked)
	}
}

// TestSetStatus_NonCanonicalUserIDStillRevokes 验证管理员用非 canonical 形式（大写）的 id
// 禁用用户时，吊销键依然写在 canonical UUID 上——中间件读的是 JWT 里的 canonical uid，
// 写错形式等于「200 OK 但一个 token 都没吊销」。
func TestSetStatus_NonCanonicalUserIDStillRevokes(t *testing.T) {
	r := testutil.NewFakeUserRepo()
	target := seedUser(r, repo.RoleUser, repo.StatusActive, "13800000001", time.Now())

	mr, rdb := newRDB(t)
	sessions := session.NewStore(rdb, "test", true, time.Hour, true)
	svc := New(r,
		WithRevokeSupport(rdb, "test", time.Hour),
		WithSessionStore(sessions),
	)

	ctx := context.Background()
	sid := session.NewSessionID()
	if err := sessions.Register(ctx, target.ID.String(), sid); err != nil {
		t.Fatalf("register session: %v", err)
	}

	canonical := target.ID.String()
	upper := strings.ToUpper(canonical)
	if upper == canonical {
		t.Fatal("test precondition broken: uppercase form equals canonical")
	}

	if _, e := svc.SetStatus(ctx, uuid.New().String(), upper, repo.StatusDisabled); e != nil {
		t.Fatalf("set status with uppercase id failed: %+v", e)
	}

	if !mr.Exists("test:revoke:uid:" + canonical) {
		t.Errorf("expected canonical revoke key test:revoke:uid:%s to be set", canonical)
	}
	if mr.Exists("test:revoke:uid:" + upper) {
		t.Error("revoke key written under the non-canonical form — nobody reads it back")
	}
	if sessions.Validate(ctx, canonical, sid) {
		t.Error("session registered under canonical id survived RevokeAll(non-canonical id)")
	}

	// 改角色同样要吊销（旧 token 携带旧 role）。
	other := seedUser(r, repo.RoleUser, repo.StatusActive, "13800000002", time.Now())
	mr.FlushAll()
	sid2 := session.NewSessionID()
	if err := sessions.Register(ctx, other.ID.String(), sid2); err != nil {
		t.Fatalf("register session: %v", err)
	}
	if _, e := svc.UpdateRole(ctx, uuid.New().String(), "{"+other.ID.String()+"}", repo.RoleAdmin); e != nil {
		t.Fatalf("update role with braced id failed: %+v", e)
	}
	if !mr.Exists("test:revoke:uid:" + other.ID.String()) {
		t.Errorf("role change did not revoke canonical key for %s", other.ID.String())
	}
	if sessions.Validate(ctx, other.ID.String(), sid2) {
		t.Error("role change left the old session alive")
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

// TestUpdateRole_RejectsDemotingLastAdmin 降权最后一个 active admin 必须被拒绝。
// 守卫与 UPDATE 在同一条 SQL 内完成，不存在“先查后改”的竞态窗口。
func TestUpdateRole_RejectsDemotingLastAdmin(t *testing.T) {
	r := testutil.NewFakeUserRepo()
	admin := seedUser(r, repo.RoleAdmin, repo.StatusActive, "13800000001", time.Now())
	svc := New(r)

	_, e := svc.UpdateRole(context.Background(), uuid.New().String(), admin.ID.String(), repo.RoleUser)
	if e == nil || e.Code != errcode.ErrLastAdmin.Code {
		t.Fatalf("expected ErrLastAdmin, got %+v", e)
	}
	if admin.Role != repo.RoleAdmin {
		t.Fatal("role was changed despite the guard")
	}
}

// TestUpdateRole_AllowsDemotingWhenAnotherAdminExists 仍有其他可用管理员时允许降权。
func TestUpdateRole_AllowsDemotingWhenAnotherAdminExists(t *testing.T) {
	r := testutil.NewFakeUserRepo()
	operator := seedUser(r, repo.RoleAdmin, repo.StatusActive, "13800000001", time.Now())
	target := seedUser(r, repo.RoleAdmin, repo.StatusActive, "13800000002", time.Now())
	svc := New(r)

	item, e := svc.UpdateRole(context.Background(), operator.ID.String(), target.ID.String(), repo.RoleUser)
	if e != nil {
		t.Fatalf("demote should be allowed when another admin remains: %+v", e)
	}
	if item.Role != repo.RoleUser {
		t.Fatalf("role = %s, want user", item.Role)
	}
}

// TestSetStatus_RejectsDisablingLastAdmin 禁用最后一个 active admin 必须被拒绝。
func TestSetStatus_RejectsDisablingLastAdmin(t *testing.T) {
	r := testutil.NewFakeUserRepo()
	admin := seedUser(r, repo.RoleAdmin, repo.StatusActive, "13800000001", time.Now())
	svc := New(r)

	_, e := svc.SetStatus(context.Background(), uuid.New().String(), admin.ID.String(), repo.StatusDisabled)
	if e == nil || e.Code != errcode.ErrLastAdmin.Code {
		t.Fatalf("expected ErrLastAdmin, got %+v", e)
	}
	if admin.Status != repo.StatusActive {
		t.Fatal("status was changed despite the guard")
	}
}

// TestUpdateRole_WritesAuditLog 审计必须记录操作者、目标、动作与前后值。
func TestUpdateRole_WritesAuditLog(t *testing.T) {
	r := testutil.NewFakeUserRepo()
	operator := seedUser(r, repo.RoleAdmin, repo.StatusActive, "13800000001", time.Now())
	target := seedUser(r, repo.RoleUser, repo.StatusActive, "13800000002", time.Now())
	audit := &testutil.FakeAuditRepo{}
	svc := New(r, WithAuditRepo(audit))

	if _, e := svc.UpdateRole(context.Background(), operator.ID.String(), target.ID.String(), repo.RoleAdmin); e != nil {
		t.Fatalf("update role failed: %+v", e)
	}
	if len(audit.Logs) != 1 {
		t.Fatalf("audit logs = %d, want 1", len(audit.Logs))
	}
	entry := audit.Logs[0]
	if entry.Action != AuditActionUpdateRole {
		t.Errorf("action = %q, want %q", entry.Action, AuditActionUpdateRole)
	}
	if entry.OperatorID == nil || *entry.OperatorID != operator.ID {
		t.Errorf("operator = %v, want %s", entry.OperatorID, operator.ID)
	}
	if entry.TargetID == nil || *entry.TargetID != target.ID {
		t.Errorf("target = %v, want %s", entry.TargetID, target.ID)
	}
	if entry.Before["role"] != repo.RoleUser || entry.After["role"] != repo.RoleAdmin {
		t.Errorf("before/after = %v/%v, want role user->admin", entry.Before, entry.After)
	}
}

// TestSetStatus_WritesAuditLog 禁用操作同样留痕，before/after 记录状态变化。
func TestSetStatus_WritesAuditLog(t *testing.T) {
	r := testutil.NewFakeUserRepo()
	operator := seedUser(r, repo.RoleAdmin, repo.StatusActive, "13800000001", time.Now())
	target := seedUser(r, repo.RoleUser, repo.StatusActive, "13800000002", time.Now())
	audit := &testutil.FakeAuditRepo{}
	svc := New(r, WithAuditRepo(audit))

	if _, e := svc.SetStatus(context.Background(), operator.ID.String(), target.ID.String(), repo.StatusDisabled); e != nil {
		t.Fatalf("set status failed: %+v", e)
	}
	if len(audit.Logs) != 1 {
		t.Fatalf("audit logs = %d, want 1", len(audit.Logs))
	}
	entry := audit.Logs[0]
	if entry.Action != AuditActionSetStatus {
		t.Errorf("action = %q, want %q", entry.Action, AuditActionSetStatus)
	}
	if entry.Before["status"] != repo.StatusActive || entry.After["status"] != repo.StatusDisabled {
		t.Errorf("before/after = %v/%v, want status active->disabled", entry.Before, entry.After)
	}
}

// TestSetStatus_AuditFailureDoesNotFailOperation 审计写失败不回滚业务变更：
// 业务语句已经生效，这里仍返回成功，失败细节进内部日志。
func TestSetStatus_AuditFailureDoesNotFailOperation(t *testing.T) {
	r := testutil.NewFakeUserRepo()
	operator := seedUser(r, repo.RoleAdmin, repo.StatusActive, "13800000001", time.Now())
	target := seedUser(r, repo.RoleUser, repo.StatusActive, "13800000002", time.Now())
	audit := &testutil.FakeAuditRepo{Err: errors.New("audit db is down")}
	svc := New(r, WithAuditRepo(audit))

	if _, e := svc.SetStatus(context.Background(), operator.ID.String(), target.ID.String(), repo.StatusDisabled); e != nil {
		t.Fatalf("business change should still succeed: %+v", e)
	}
	if target.Status != repo.StatusDisabled {
		t.Fatal("status change was rolled back by audit failure")
	}
}
