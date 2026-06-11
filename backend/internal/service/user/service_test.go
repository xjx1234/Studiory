package userservice

import (
	"context"
	"testing"

	"backend/internal/repo"
	"backend/internal/testutil"

	"github.com/google/uuid"
)

func TestUpdateProfile(t *testing.T) {
	id := uuid.New()
	fakeRepo := testutil.NewFakeUserRepo()
	fakeRepo.Users[id] = &repo.User{
		ID:       id,
		Nickname: "旧昵称",
		Avatar:   "",
		Role:     "student",
	}

	svc := New(fakeRepo)

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
