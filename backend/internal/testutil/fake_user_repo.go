// Package testutil 提供测试辅助工具，仅供测试代码使用（不进入生产二进制）。
package testutil

import (
	"context"
	"errors"

	"backend/internal/repo"

	"github.com/google/uuid"
)

// FakeUserRepo 是 repo.UserRepo 的内存实现，供单元测试使用。
// 通过 Users map 预设数据；Create 调用会自动写入并追加到 Created 切片。
type FakeUserRepo struct {
	Users   map[uuid.UUID]*repo.User
	Created []*repo.User
}

func NewFakeUserRepo() *FakeUserRepo {
	return &FakeUserRepo{Users: make(map[uuid.UUID]*repo.User)}
}

func (r *FakeUserRepo) GetByID(_ context.Context, id uuid.UUID) (*repo.User, error) {
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
	}
	r.Users[u.ID] = u
	r.Created = append(r.Created, u)
	return u, nil
}

func (r *FakeUserRepo) UpdatePassword(_ context.Context, id uuid.UUID, hash string) (*repo.User, error) {
	u, ok := r.Users[id]
	if !ok {
		return nil, repo.ErrNotFound
	}
	u.PasswordHash = &hash
	return u, nil
}

func (r *FakeUserRepo) UpdateProfile(_ context.Context, id uuid.UUID, nickname, avatar string) (*repo.User, error) {
	u, ok := r.Users[id]
	if !ok {
		return nil, repo.ErrNotFound
	}
	u.Nickname = nickname
	u.Avatar = avatar
	return u, nil
}
