package userservice

import (
	"context"
	"testing"

	"backend/internal/repo"

	"github.com/google/uuid"
)

type fakeUserRepo struct {
	user *repo.User
}

func (r *fakeUserRepo) GetByID(_ context.Context, id uuid.UUID) (*repo.User, error) {
	if r.user != nil && r.user.ID == id {
		return r.user, nil
	}
	return nil, repo.ErrNotFound
}

func (r *fakeUserRepo) GetByPhone(context.Context, string) (*repo.User, error) {
	return nil, repo.ErrNotFound
}

func (r *fakeUserRepo) GetByEmail(context.Context, string) (*repo.User, error) {
	return nil, repo.ErrNotFound
}

func (r *fakeUserRepo) Create(context.Context, *repo.CreateUserParams) (*repo.User, error) {
	return nil, repo.ErrNotFound
}

func (r *fakeUserRepo) UpdatePassword(context.Context, uuid.UUID, string) (*repo.User, error) {
	return nil, repo.ErrNotFound
}

func (r *fakeUserRepo) UpdateProfile(_ context.Context, id uuid.UUID, nickname, avatar string) (*repo.User, error) {
	if r.user == nil || r.user.ID != id {
		return nil, repo.ErrNotFound
	}
	r.user.Nickname = nickname
	r.user.Avatar = avatar
	return r.user, nil
}

func TestUpdateProfile(t *testing.T) {
	id := uuid.New()
	repoUser := &repo.User{
		ID:       id,
		Nickname: "旧昵称",
		Avatar:   "",
		Role:     "student",
	}
	svc := New(&fakeUserRepo{user: repoUser})

	profile, e := svc.UpdateProfile(context.Background(), id.String(), &UpdateProfileInput{
		Nickname: "新昵称",
		Avatar:   "https://example.com/a.png",
	})
	if e != nil {
		t.Fatalf("update failed: %+v", e)
	}
	if profile.Nickname != "新昵称" || profile.Avatar != "https://example.com/a.png" {
		t.Fatalf("unexpected profile: %+v", profile)
	}
}
