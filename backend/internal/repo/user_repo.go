package repo

import (
	"context"

	"github.com/google/uuid"
)

// CreateUserParams 创建用户入参。
type CreateUserParams struct {
	Phone        *string
	Email        *string
	PasswordHash *string
	Nickname     string
	Avatar       string
	Role         string
}

// ListUsersParams 后台用户列表查询入参。
// Keyword 为空时不做模糊匹配；Status 为空时不做状态过滤。
type ListUsersParams struct {
	Keyword string
	Status  string
	Limit   int32
	Offset  int32
}

type UserRepo interface {
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
	GetByPhone(ctx context.Context, phone string) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)

	Create(ctx context.Context, params *CreateUserParams) (*User, error)
	UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) (*User, error)
	// UpdatePasswordIfUnchanged 只在库内 hash 仍等于 expectedHash 时写入 newHash，返回是否更新。
	// 用于登录成功后的 bcrypt 升级（CAS）：并发的改密若已换掉 hash，这里必须放弃，
	// 否则会把旧密码的 hash 写回去，等于撤销用户刚做的改密。
	UpdatePasswordIfUnchanged(ctx context.Context, id uuid.UUID, expectedHash, newHash string) (bool, error)
	UpdateProfile(ctx context.Context, id uuid.UUID, nickname, avatar string) (*User, error)

	// 后台管理
	List(ctx context.Context, params *ListUsersParams) ([]*User, error)
	Count(ctx context.Context, keyword, status string) (int64, error)
	// UpdateRole 修改角色。若目标当前是最后一个 active admin，而 role 又不再是 admin，
	// 返回 ErrLastAdmin 且不做修改（守卫与更新在同一条 SQL 内完成，避免先查后改的竞态）。
	UpdateRole(ctx context.Context, id uuid.UUID, role string) (*User, error)
	// UpdateStatus 修改状态。禁用最后一个 active admin 同样返回 ErrLastAdmin。
	UpdateStatus(ctx context.Context, id uuid.UUID, status string) (*User, error)
}
