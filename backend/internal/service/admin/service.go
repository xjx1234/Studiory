// Package adminservice 提供后台用户管理能力（列表 / 详情 / 改角色 / 启用禁用）。
// 作为脚手架示例，演示如何基于 RBAC 在 admin 路由下编写管理类业务。
package adminservice

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"backend/internal/repo"
	baseservice "backend/internal/service"
	"backend/internal/session"
	"backend/pkg/errcode"
	"backend/pkg/pagination"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// 写入 admin_audit_logs 的 action 取值；按 action 过滤即可还原某类管理操作的历史。
const (
	AuditActionUpdateRole = "user.role.update"
	AuditActionSetStatus  = "user.status.update"
)

// UserItem 后台用户视图（对外输出结构）。
type UserItem struct {
	ID        string `json:"id"`
	Phone     string `json:"phone,omitempty"`
	Email     string `json:"email,omitempty"`
	Nickname  string `json:"nickname"`
	Avatar    string `json:"avatar"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	// SessionsRevoked 仅在「改角色 / 禁用」的响应中出现（omitempty）：
	// true=旧会话与 access token 已吊销；false=更新已生效但吊销未完成（Redis 异常）。
	SessionsRevoked *bool `json:"sessions_revoked,omitempty"`
}

// ListInput 用户列表查询入参。
type ListInput struct {
	Keyword string
	Status  string
}

// Service 定义后台用户管理业务入口。
type Service interface {
	ListUsers(ctx context.Context, in ListInput, page pagination.Query) (pagination.List[UserItem], *errcode.Error)
	GetUser(ctx context.Context, userID string) (*UserItem, *errcode.Error)
	UpdateRole(ctx context.Context, actingUserID, targetUserID, role string) (*UserItem, *errcode.Error)
	SetStatus(ctx context.Context, actingUserID, targetUserID, status string) (*UserItem, *errcode.Error)
}

type adminServiceImpl struct {
	baseservice.LogSupport

	users     repo.UserRepo
	audit     repo.AdminAuditRepo
	sessions  *session.Store
	rdb       redis.UniversalClient
	keyPrefix string
	accessTTL time.Duration
}

type Option func(*adminServiceImpl)

// New 创建 AdminService。
func New(users repo.UserRepo, opts ...Option) Service {
	s := &adminServiceImpl{users: users}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func WithLogger(logger *zap.Logger) Option {
	return func(s *adminServiceImpl) {
		s.SetLogger(logger)
	}
}

// WithSessionStore 注入会话存储，禁用用户时吊销全部 session。
func WithSessionStore(store *session.Store) Option {
	return func(s *adminServiceImpl) {
		s.sessions = store
	}
}

// WithAuditRepo 注入审计仓库，改角色/改状态后写入 admin_audit_logs。
// 未注入时不写审计（单测与最小化装配场景）。
func WithAuditRepo(audit repo.AdminAuditRepo) Option {
	return func(s *adminServiceImpl) {
		s.audit = audit
	}
}

// WithRevokeSupport 注入 Redis，禁用用户后立即吊销其旧 access token。
func WithRevokeSupport(rdb redis.UniversalClient, keyPrefix string, accessTTL time.Duration) Option {
	return func(s *adminServiceImpl) {
		s.rdb = rdb
		s.keyPrefix = keyPrefix
		s.accessTTL = accessTTL
	}
}

func (s *adminServiceImpl) ListUsers(ctx context.Context, in ListInput, page pagination.Query) (pagination.List[UserItem], *errcode.Error) {
	var empty pagination.List[UserItem]

	if in.Status != "" && !isValidStatus(in.Status) {
		return empty, errcode.ErrBadRequest
	}

	total, err := s.users.Count(ctx, in.Keyword, in.Status)
	if err != nil {
		s.LogInternal("ListUsers count", err)
		return empty, errcode.ErrInternal
	}

	rows, err := s.users.List(ctx, &repo.ListUsersParams{
		Keyword: in.Keyword,
		Status:  in.Status,
		Limit:   page.LimitInt32(),
		Offset:  page.OffsetInt32(),
	})
	if err != nil {
		s.LogInternal("ListUsers list", err)
		return empty, errcode.ErrInternal
	}

	items := make([]UserItem, 0, len(rows))
	for _, u := range rows {
		items = append(items, toUserItem(u))
	}
	return pagination.NewList(items, page, total), nil
}

func (s *adminServiceImpl) GetUser(ctx context.Context, userID string) (*UserItem, *errcode.Error) {
	id, e := baseservice.ParseUUID(userID)
	if e != nil {
		return nil, e
	}

	user, err := s.users.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, errcode.ErrNotFound
		}
		s.LogInternal("GetUser lookup", err, baseservice.UserIDField(userID))
		return nil, errcode.ErrInternal
	}

	item := toUserItem(user)
	return &item, nil
}

func (s *adminServiceImpl) UpdateRole(ctx context.Context, actingUserID, targetUserID, role string) (*UserItem, *errcode.Error) {
	if !isValidRole(role) {
		return nil, errcode.ErrBadRequest
	}
	id, e := baseservice.ParseUUID(targetUserID)
	if e != nil {
		return nil, e
	}
	// 不允许管理员修改自己的角色，避免自我降权后失去管理入口。
	// 使用 EqualFold 防止 UUID 大小写差异绕过保护。
	if strings.EqualFold(actingUserID, targetUserID) {
		return nil, errcode.ErrCannotModifySelf
	}

	before, err := s.users.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, errcode.ErrNotFound
		}
		s.LogInternal("UpdateRole lookup", err,
			baseservice.UserIDField(targetUserID),
			baseservice.ActorUserIDField(actingUserID),
		)
		return nil, errcode.ErrInternal
	}
	// 先快照旧值再更新：审计的 before 不能依赖更新后仍指向同一行的对象。
	previousRole := before.Role

	updated, err := s.users.UpdateRole(ctx, id, role)
	if err != nil {
		switch {
		case errors.Is(err, repo.ErrNotFound):
			return nil, errcode.ErrNotFound
		case errors.Is(err, repo.ErrLastAdmin):
			// 守卫在 SQL 内完成：会让系统失去最后一个可用管理员的降权被拒绝。
			return nil, errcode.ErrLastAdmin
		}
		s.LogInternal("UpdateRole update", err,
			baseservice.UserIDField(targetUserID),
			baseservice.ActorUserIDField(actingUserID),
		)
		return nil, errcode.ErrInternal
	}

	s.recordAudit(ctx, actingUserID, id, AuditActionUpdateRole,
		map[string]string{"role": previousRole},
		map[string]string{"role": updated.Role},
	)

	// 角色变更后立即吊销该用户所有旧 access token，使旧角色即时失效。
	// 与 SetStatus 禁用行为保持一致：权限变更必须立即生效，
	// 否则被降权用户的旧 token 在剩余 TTL 内仍携带旧 role。
	// 吊销失败也要在响应里说明，避免调用方误以为权限已即时收回。
	revoked := s.revokeAccessTokens(ctx, targetUserID)

	item := toUserItem(updated)
	item.SessionsRevoked = &revoked
	return &item, nil
}

func (s *adminServiceImpl) SetStatus(ctx context.Context, actingUserID, targetUserID, status string) (*UserItem, *errcode.Error) {
	if !isValidStatus(status) {
		return nil, errcode.ErrBadRequest
	}
	id, e := baseservice.ParseUUID(targetUserID)
	if e != nil {
		return nil, e
	}
	// 不允许管理员禁用自己，避免把自己锁在系统外。
	// 使用 EqualFold 防止 UUID 大小写差异绕过保护。
	if strings.EqualFold(actingUserID, targetUserID) && status == repo.StatusDisabled {
		return nil, errcode.ErrCannotModifySelf
	}

	before, err := s.users.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, errcode.ErrNotFound
		}
		s.LogInternal("SetStatus lookup", err,
			baseservice.UserIDField(targetUserID),
			baseservice.ActorUserIDField(actingUserID),
		)
		return nil, errcode.ErrInternal
	}
	previousStatus := before.Status

	updated, err := s.users.UpdateStatus(ctx, id, status)
	if err != nil {
		switch {
		case errors.Is(err, repo.ErrNotFound):
			return nil, errcode.ErrNotFound
		case errors.Is(err, repo.ErrLastAdmin):
			return nil, errcode.ErrLastAdmin
		}
		s.LogInternal("SetStatus update", err,
			baseservice.UserIDField(targetUserID),
			baseservice.ActorUserIDField(actingUserID),
		)
		return nil, errcode.ErrInternal
	}

	s.recordAudit(ctx, actingUserID, id, AuditActionSetStatus,
		map[string]string{"status": previousStatus},
		map[string]string{"status": updated.Status},
	)

	// 禁用后立即吊销该用户所有旧 access token，使其当前会话即时失效。
	item := toUserItem(updated)
	if status == repo.StatusDisabled {
		revoked := s.revokeAccessTokens(ctx, targetUserID)
		item.SessionsRevoked = &revoked
	}
	return &item, nil
}

// revokeAccessTokens 吊销指定用户的会话与 access token，返回是否全部成功。
// 任一存储写失败都会把结果标记为 false，由调用方在响应中告知「吊销未完成」。
func (s *adminServiceImpl) revokeAccessTokens(ctx context.Context, userID string) bool {
	// 吊销键必须和中间件读取的那把完全一致：路径参数允许大写/`{}`/`urn:uuid:` 形式，
	// 而读取方用的是 JWT 里的 canonical uid。不归一化等于写了一把没人读的键——
	// 接口返回 200，旧 token 却在剩余 TTL 内继续可用。
	userID = baseservice.CanonicalUserID(userID)
	ok := true

	if s.sessions != nil {
		if err := s.sessions.RevokeAll(ctx, userID); err != nil {
			ok = false
			s.LogInternal("revoke all sessions", err, baseservice.UserIDField(userID))
		}
	}
	if s.rdb == nil {
		return ok
	}
	revokeKey := fmt.Sprintf("%s:revoke:uid:%s", s.keyPrefix, userID)
	if err := s.rdb.Set(ctx, revokeKey, time.Now().Unix(), s.accessTTL).Err(); err != nil {
		ok = false
		s.LogInternal("revoke access tokens", err, baseservice.UserIDField(userID))
	}
	return ok
}

// recordAudit 写入一条后台管理审计日志（操作者、目标、动作、前后值）。
//
// 审计写失败不回滚已经生效的角色/状态变更：同库写入失败通常已由业务语句暴露，
// 回滚也救不回丢失的日志；这里保留内部错误日志便于按日志补录。若合规要求更高，
// 可改为与业务变更同事务（fail-closed），届时需要 repo 层提供事务句柄。
func (s *adminServiceImpl) recordAudit(ctx context.Context, actingUserID string, targetID uuid.UUID, action string, before, after map[string]string) {
	if s.audit == nil {
		return
	}

	actorID, err := uuid.Parse(baseservice.CanonicalUserID(actingUserID))
	if err != nil {
		s.LogInternal("write audit log: invalid actor id", err,
			baseservice.ActorUserIDField(actingUserID),
		)
		return
	}

	entry := &repo.AdminAuditLog{
		OperatorID: &actorID,
		TargetID:   &targetID,
		Action:     action,
		Before:     before,
		After:      after,
	}
	if err := s.audit.Create(ctx, entry); err != nil {
		s.LogInternal("write audit log", err,
			baseservice.UserIDField(targetID.String()),
			baseservice.ActorUserIDField(actingUserID),
		)
	}
}

func isValidRole(role string) bool {
	return role == repo.RoleAdmin || role == repo.RoleUser
}

func isValidStatus(status string) bool {
	return status == repo.StatusActive || status == repo.StatusDisabled
}

func toUserItem(u *repo.User) UserItem {
	item := UserItem{
		ID:        u.ID.String(),
		Nickname:  u.Nickname,
		Avatar:    u.Avatar,
		Role:      u.Role,
		Status:    u.Status,
		CreatedAt: u.CreatedAt.Format(time.RFC3339),
		UpdatedAt: u.UpdatedAt.Format(time.RFC3339),
	}
	if u.Phone != nil {
		item.Phone = *u.Phone
	}
	if u.Email != nil {
		item.Email = *u.Email
	}
	return item
}
