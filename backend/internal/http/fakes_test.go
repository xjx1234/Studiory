package http

import (
	"context"

	"backend/internal/auth"
	authservice "backend/internal/service/auth"
	todoservice "backend/internal/service/todo"
	userservice "backend/internal/service/user"
	"backend/pkg/errcode"
	"backend/pkg/pagination"
)

// 以下 fake service 实现各自的 Service 接口，供 handler 集成测试使用。
// 每个方法用可选的函数字段覆写行为；未设置时返回一个合理的默认成功值。

// ── fakeAuthService ───────────────────────────────────────────────────────────

type fakeAuthService struct {
	loginFn    func(ctx context.Context, req *auth.LoginRequest) (*auth.LoginResult, *errcode.Error)
	registerFn func(ctx context.Context, in *authservice.RegisterInput) (*auth.LoginResult, *errcode.Error)
	sendCodeFn func(ctx context.Context, codeType, target string) *errcode.Error
	refreshFn  func(ctx context.Context, refreshToken string) (*auth.TokenPair, *errcode.Error)
	logoutFn   func(ctx context.Context, refreshToken string) *errcode.Error
}

var _ authservice.Service = (*fakeAuthService)(nil)

func defaultLoginResult() *auth.LoginResult {
	return &auth.LoginResult{
		Tokens: &auth.TokenPair{AccessToken: "access-token", RefreshToken: "refresh-token", ExpiresIn: 3600},
		User:   &auth.UserInfo{ID: "user-1", Nickname: "tester", Role: "user"},
	}
}

func (f *fakeAuthService) Login(ctx context.Context, req *auth.LoginRequest) (*auth.LoginResult, *errcode.Error) {
	if f.loginFn != nil {
		return f.loginFn(ctx, req)
	}
	return defaultLoginResult(), nil
}

func (f *fakeAuthService) Register(ctx context.Context, in *authservice.RegisterInput) (*auth.LoginResult, *errcode.Error) {
	if f.registerFn != nil {
		return f.registerFn(ctx, in)
	}
	return defaultLoginResult(), nil
}

func (f *fakeAuthService) SendCode(ctx context.Context, codeType, target string) *errcode.Error {
	if f.sendCodeFn != nil {
		return f.sendCodeFn(ctx, codeType, target)
	}
	return nil
}

func (f *fakeAuthService) Refresh(ctx context.Context, refreshToken string) (*auth.TokenPair, *errcode.Error) {
	if f.refreshFn != nil {
		return f.refreshFn(ctx, refreshToken)
	}
	return &auth.TokenPair{AccessToken: "access-token-2", RefreshToken: "refresh-token-2", ExpiresIn: 3600}, nil
}

func (f *fakeAuthService) Logout(ctx context.Context, refreshToken string) *errcode.Error {
	if f.logoutFn != nil {
		return f.logoutFn(ctx, refreshToken)
	}
	return nil
}

// ── fakeUserService ───────────────────────────────────────────────────────────

type fakeUserService struct {
	getProfileFn     func(ctx context.Context, userID string) (*userservice.ProfileResult, *errcode.Error)
	updateProfileFn  func(ctx context.Context, userID string, in *userservice.UpdateProfileInput) (*userservice.ProfileResult, *errcode.Error)
	changePasswordFn func(ctx context.Context, userID string, in *userservice.ChangePasswordInput) *errcode.Error
}

var _ userservice.Service = (*fakeUserService)(nil)

func defaultProfile(userID string) *userservice.ProfileResult {
	return &userservice.ProfileResult{ID: userID, Nickname: "tester", Role: "user"}
}

func (f *fakeUserService) GetProfile(ctx context.Context, userID string) (*userservice.ProfileResult, *errcode.Error) {
	if f.getProfileFn != nil {
		return f.getProfileFn(ctx, userID)
	}
	return defaultProfile(userID), nil
}

func (f *fakeUserService) UpdateProfile(ctx context.Context, userID string, in *userservice.UpdateProfileInput) (*userservice.ProfileResult, *errcode.Error) {
	if f.updateProfileFn != nil {
		return f.updateProfileFn(ctx, userID, in)
	}
	p := defaultProfile(userID)
	if in.Nickname != "" {
		p.Nickname = in.Nickname
	}
	if in.Avatar != "" {
		p.Avatar = in.Avatar
	}
	return p, nil
}

func (f *fakeUserService) ChangePassword(ctx context.Context, userID string, in *userservice.ChangePasswordInput) *errcode.Error {
	if f.changePasswordFn != nil {
		return f.changePasswordFn(ctx, userID, in)
	}
	return nil
}

// ── fakeTodoService ───────────────────────────────────────────────────────────

type fakeTodoService struct {
	listFn   func(ctx context.Context, userID string, page pagination.Query) (pagination.List[todoservice.Item], *errcode.Error)
	getFn    func(ctx context.Context, userID, todoID string) (*todoservice.Item, *errcode.Error)
	createFn func(ctx context.Context, userID string, in *todoservice.CreateInput) (*todoservice.Item, *errcode.Error)
	updateFn func(ctx context.Context, userID, todoID string, in *todoservice.UpdateInput) (*todoservice.Item, *errcode.Error)
	deleteFn func(ctx context.Context, userID, todoID string) *errcode.Error
}

var _ todoservice.Service = (*fakeTodoService)(nil)

func (f *fakeTodoService) List(ctx context.Context, userID string, page pagination.Query) (pagination.List[todoservice.Item], *errcode.Error) {
	if f.listFn != nil {
		return f.listFn(ctx, userID, page)
	}
	return pagination.List[todoservice.Item]{Items: []todoservice.Item{}, Total: 0}, nil
}

func (f *fakeTodoService) Get(ctx context.Context, userID, todoID string) (*todoservice.Item, *errcode.Error) {
	if f.getFn != nil {
		return f.getFn(ctx, userID, todoID)
	}
	return &todoservice.Item{ID: todoID, Title: "demo"}, nil
}

func (f *fakeTodoService) Create(ctx context.Context, userID string, in *todoservice.CreateInput) (*todoservice.Item, *errcode.Error) {
	if f.createFn != nil {
		return f.createFn(ctx, userID, in)
	}
	return &todoservice.Item{ID: "todo-1", Title: in.Title}, nil
}

func (f *fakeTodoService) Update(ctx context.Context, userID, todoID string, in *todoservice.UpdateInput) (*todoservice.Item, *errcode.Error) {
	if f.updateFn != nil {
		return f.updateFn(ctx, userID, todoID, in)
	}
	return &todoservice.Item{ID: todoID, Title: in.Title, Done: in.Done}, nil
}

func (f *fakeTodoService) Delete(ctx context.Context, userID, todoID string) *errcode.Error {
	if f.deleteFn != nil {
		return f.deleteFn(ctx, userID, todoID)
	}
	return nil
}
