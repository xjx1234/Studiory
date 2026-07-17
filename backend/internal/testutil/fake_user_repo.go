// Package testutil 提供测试辅助工具，仅供测试代码使用（不进入生产二进制）。
package testutil

import (
	"context"
	"errors"
	"sort"
	"strings"

	"backend/internal/repo"

	"github.com/google/uuid"
)

// FakeUserRepo 是 repo.UserRepo 的内存实现，供单元测试使用。
// 通过 Users map 预设数据；Create 调用会自动写入并追加到 Created 切片。
type FakeUserRepo struct {
	Users   map[uuid.UUID]*repo.User
	Created []*repo.User

	// 错误注入：非 nil 时对应方法直接返回该错误，用于覆盖 service 层的
	// “非 ErrNotFound 的内部错误” 分支（真实数据库故障等）。
	GetByIDErr        error
	UpdateProfileErr  error
	UpdatePasswordErr error
}

func NewFakeUserRepo() *FakeUserRepo {
	return &FakeUserRepo{Users: make(map[uuid.UUID]*repo.User)}
}

func (r *FakeUserRepo) GetByID(_ context.Context, id uuid.UUID) (*repo.User, error) {
	if r.GetByIDErr != nil {
		return nil, r.GetByIDErr
	}
	if u, ok := r.Users[id]; ok {
		return u, nil
	}
	return nil, repo.ErrNotFound
}

func (r *FakeUserRepo) GetByPhone(_ context.Context, phone string) (*repo.User, error) {
	for _, u := range r.Users {
		if u.Phone != nil && *u.Phone == phone {
			return u, nil
		}
	}
	return nil, repo.ErrNotFound
}

func (r *FakeUserRepo) GetByEmail(_ context.Context, email string) (*repo.User, error) {
	for _, u := range r.Users {
		if u.Email != nil && *u.Email == email {
			return u, nil
		}
	}
	return nil, repo.ErrNotFound
}

func (r *FakeUserRepo) Create(_ context.Context, params *repo.CreateUserParams) (*repo.User, error) {
	if params == nil {
		return nil, errors.New("create user params is nil")
	}
	u := &repo.User{
		ID:           uuid.New(),
		Phone:        params.Phone,
		Email:        params.Email,
		PasswordHash: params.PasswordHash,
		Nickname:     params.Nickname,
		Avatar:       params.Avatar,
		Role:         params.Role,
		Status:       repo.StatusActive,
	}
	r.Users[u.ID] = u
	r.Created = append(r.Created, u)
	return u, nil
}

func (r *FakeUserRepo) UpdatePassword(_ context.Context, id uuid.UUID, hash string) (*repo.User, error) {
	if r.UpdatePasswordErr != nil {
		return nil, r.UpdatePasswordErr
	}
	u, ok := r.Users[id]
	if !ok {
		return nil, repo.ErrNotFound
	}
	u.PasswordHash = &hash
	return u, nil
}

func (r *FakeUserRepo) UpdateProfile(_ context.Context, id uuid.UUID, nickname, avatar string) (*repo.User, error) {
	if r.UpdateProfileErr != nil {
		return nil, r.UpdateProfileErr
	}
	u, ok := r.Users[id]
	if !ok {
		return nil, repo.ErrNotFound
	}
	u.Nickname = nickname
	u.Avatar = avatar
	return u, nil
}

func (r *FakeUserRepo) matches(u *repo.User, keyword, status string) bool {
	if status != "" && u.Status != status {
		return false
	}
	if keyword == "" {
		return true
	}
	kw := strings.ToLower(keyword)
	if u.Phone != nil && strings.Contains(strings.ToLower(*u.Phone), kw) {
		return true
	}
	if u.Email != nil && strings.Contains(strings.ToLower(*u.Email), kw) {
		return true
	}
	return strings.Contains(strings.ToLower(u.Nickname), kw)
}

func (r *FakeUserRepo) List(_ context.Context, params *repo.ListUsersParams) ([]*repo.User, error) {
	if params == nil {
		return nil, errors.New("list users params is nil")
	}
	var filtered []*repo.User
	for _, u := range r.Users {
		if r.matches(u, params.Keyword, params.Status) {
			filtered = append(filtered, u)
		}
	}
	// 与 SQL 一致：按创建时间倒序（同刻退化用 ID 保证稳定排序）
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].CreatedAt.Equal(filtered[j].CreatedAt) {
			return filtered[i].ID.String() > filtered[j].ID.String()
		}
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})

	offset := int(params.Offset)
	if offset >= len(filtered) {
		return []*repo.User{}, nil
	}
	end := offset + int(params.Limit)
	if params.Limit <= 0 || end > len(filtered) {
		end = len(filtered)
	}
	return filtered[offset:end], nil
}

func (r *FakeUserRepo) Count(_ context.Context, keyword, status string) (int64, error) {
	var n int64
	for _, u := range r.Users {
		if r.matches(u, keyword, status) {
			n++
		}
	}
	return n, nil
}

func (r *FakeUserRepo) UpdateRole(_ context.Context, id uuid.UUID, role string) (*repo.User, error) {
	u, ok := r.Users[id]
	if !ok {
		return nil, repo.ErrNotFound
	}
	u.Role = role
	return u, nil
}

func (r *FakeUserRepo) UpdateStatus(_ context.Context, id uuid.UUID, status string) (*repo.User, error) {
	u, ok := r.Users[id]
	if !ok {
		return nil, repo.ErrNotFound
	}
	u.Status = status
	return u, nil
}
