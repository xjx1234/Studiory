//go:build integration

// Package mysql 集成测试：用 testcontainers 起一个临时 MySQL，灌入真实迁移后
// 验证与 PG 侧等价的关键行为。重点锁死三个移植回归点：
//  1. GetUserByOAuth JOIN 列名歧义（Error 1052）；
//  2. DATETIME 时区漂移（必须与 UTC 对齐，不允许偏移服务器本地时区）；
//  3. users.role / users.status 的 CHECK 约束必须生效。
//
// 运行方式（需要本机有 Docker）：
//
//	go test -tags=integration ./internal/repo/mysql/...
//
// 不带 -tags=integration 时本文件不参与编译，普通单测不受影响。
package mysql

import (
	"context"
	"testing"
	"time"

	"backend/internal/config"
	"backend/internal/repo"
	"backend/internal/store"

	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
)

func newTestStore(t *testing.T) repo.Storage {
	t.Helper()
	ctx := context.Background()

	container, err := tcmysql.Run(ctx, "mysql:8.4",
		tcmysql.WithDatabase("app_test"),
		tcmysql.WithUsername("test"),
		tcmysql.WithPassword("test"),
		tcmysql.WithScripts("../../migrations/mysql/000001_init_schema.up.sql"),
	)
	if err != nil {
		t.Fatalf("start mysql: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	// 与生产同款路径：容器裸 DSN → NormalizeMySQLDSN（parseTime/loc/time_zone）→ store.NewMySQL
	rawDSN, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("mysql dsn: %v", err)
	}
	dsn, err := config.NormalizeMySQLDSN(rawDSN)
	if err != nil {
		t.Fatalf("normalize dsn: %v", err)
	}
	db, err := store.NewMySQL(ctx, dsn, store.MySQLOptions{MaxOpenConns: 4}, nil)
	if err != nil {
		t.Fatalf("connect mysql: %v", err)
	}
	st := NewStore(db)
	t.Cleanup(st.Close)
	return st
}

func createTestUser(t *testing.T, s repo.Storage, phone string) *repo.User {
	t.Helper()
	u, err := s.Users().Create(context.Background(), &repo.CreateUserParams{
		Phone:    &phone,
		Nickname: "it-user",
		Role:     "user",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return u
}

// TestGetUserByOAuth 锁死 JOIN 列名歧义 bug：users 与 user_oauth 同有
// id/created_at，裸列名直接报 Error 1052，OAuth 登录链路必挂。
func TestGetUserByOAuth(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	want := createTestUser(t, s, "13800000001")
	const openID = "openid-" + "oauth-test"

	if _, err := s.OAuth().CreateOAuth(ctx, want.ID, "wechat", openID); err != nil {
		t.Fatalf("CreateOAuth: %v", err)
	}

	got, err := s.OAuth().GetUserByOAuth(ctx, "wechat", openID)
	if err != nil {
		t.Fatalf("GetUserByOAuth: %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("GetUserByOAuth ID = %v, want %v", got.ID, want.ID)
	}
	if got.Nickname != "it-user" {
		t.Errorf("GetUserByOAuth Nickname = %q, want %q", got.Nickname, "it-user")
	}

	// 未绑定的 open_id 必须映射为 ErrNotFound
	if _, err := s.OAuth().GetUserByOAuth(ctx, "wechat", "openid-missing"); err != repo.ErrNotFound {
		t.Errorf("GetUserByOAuth(missing) err = %v, want ErrNotFound", err)
	}
}

// TestTimestampsUseUTC 锁死时区漂移 bug：写入后回读的 CreatedAt 必须与 UTC
// 当前时间对齐（±2 分钟），不允许偏移服务器本地时区（历史 bug 为 +8h）。
func TestTimestampsUseUTC(t *testing.T) {
	s := newTestStore(t)

	u := createTestUser(t, s, "13800000002")

	drift := time.Since(u.CreatedAt.UTC())
	if drift < -2*time.Minute || drift > 2*time.Minute {
		t.Errorf("created_at drift from now = %v (CreatedAt=%v, now=%v); want |drift| <= 2m",
			drift, u.CreatedAt, time.Now().UTC())
	}
	if u.CreatedAt.Location() != time.UTC {
		t.Errorf("created_at location = %v, want UTC", u.CreatedAt.Location())
	}
}

// TestCheckConstraints 锁死 CHECK 缺失 bug：非法 role/status 必须被拒绝，
// 与 PG 侧 000001/000003 的约束语义一致。
func TestCheckConstraints(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	u := createTestUser(t, s, "13800000003")

	if _, err := s.Users().UpdateStatus(ctx, u.ID, "banned"); err == nil {
		t.Error("UpdateStatus(banned) = nil, want CHECK violation")
	}
	if _, err := s.Users().UpdateRole(ctx, u.ID, "superuser"); err == nil {
		t.Error("UpdateRole(superuser) = nil, want CHECK violation")
	}

	// 合法值必须仍然可用
	if _, err := s.Users().UpdateStatus(ctx, u.ID, "disabled"); err != nil {
		t.Errorf("UpdateStatus(disabled): %v", err)
	}
	if _, err := s.Users().UpdateRole(ctx, u.ID, "admin"); err != nil {
		t.Errorf("UpdateRole(admin): %v", err)
	}
}

// TestTodoCRUD 基本回归：CRUD、重复删除映射 ErrNotFound。
// 注意 todos.user_id 有外键约束，必须挂到真实用户。
func TestTodoCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	u := createTestUser(t, s, "13800000004")

	todo, err := s.Todos().Create(ctx, u.ID, "first")
	if err != nil {
		t.Fatalf("create todo: %v", err)
	}
	if _, err := s.Todos().Update(ctx, todo.ID, u.ID, "first", true); err != nil {
		t.Fatalf("update todo: %v", err)
	}
	if err := s.Todos().Delete(ctx, todo.ID, u.ID); err != nil {
		t.Fatalf("delete todo: %v", err)
	}
	if err := s.Todos().Delete(ctx, todo.ID, u.ID); err != repo.ErrNotFound {
		t.Errorf("second delete err = %v, want ErrNotFound", err)
	}
}
