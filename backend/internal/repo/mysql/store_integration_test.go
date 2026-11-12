//go:build integration

// Package mysql 集成测试：用 testcontainers 起一个临时 MySQL，灌入真实迁移后
// 验证与 PG 侧等价的关键行为。重点锁死三个移植回归点：
//  1. GetUserByOAuth JOIN 列名歧义（Error 1052）；
//  2. DATETIME 时区漂移（必须与 UTC 对齐，不允许偏移服务器本地时区）；
//  3. users.role / users.status 的 CHECK 约束必须生效。
//
// 与 PG 侧同构：TestMain 起一个容器供整包复用（MySQL 8.4 初始化要几十秒，
// 每个用例起一个容器会显著拖慢并逼近 go test 默认 10m 超时）。
//
// 运行方式（需要本机有 Docker）：
//
//	go test -tags=integration ./internal/repo/mysql/...
//
// 不带 -tags=integration 时本文件不参与编译，普通单测不受影响。
package mysql

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"backend/internal/config"
	"backend/internal/repo"
	"backend/internal/store"

	"github.com/testcontainers/testcontainers-go"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
	"github.com/testcontainers/testcontainers-go/wait"
)

// migrationsFile 相对本包目录（backend/internal/repo/mysql）向上三层。
// 注意不是 ../../ —— 那会指到并不存在的 backend/internal/migrations。
const migrationsFile = "../../../migrations/mysql/000001_init_schema.up.sql"

var testStorage repo.Storage

func TestMain(m *testing.M) {
	ctx := context.Background()

	// 先自检脚本路径：路径写错时 testcontainers 只会抛一个含糊的 copy 错误，
	// 这里提前给出可直接定位的信息（CI annotation 里只显示输出尾部）。
	abs, err := os.Getwd()
	if err != nil {
		log.Fatalf("getwd: %v", err)
	}
	if _, err := os.Stat(migrationsFile); err != nil {
		log.Fatalf("migration script %s unavailable from %s: %v", migrationsFile, abs, err)
	}

	container, err := tcmysql.Run(ctx, "mysql:8.4",
		tcmysql.WithDatabase("app_test"),
		tcmysql.WithUsername("test"),
		tcmysql.WithPassword("test"),
		tcmysql.WithScripts(migrationsFile),
		// 模块默认策略就是 wait.ForLog("port: 3306  MySQL Community Server")，
		// 这里沿用同一标记但把启动窗口放宽到 3 分钟：mysql:8.4 首次 initdb
		// 再执行 initdb.d 里的迁移，在 CI runner 上可能超过默认 60s。
		testcontainers.WithWaitStrategy(
			wait.ForLog("port: 3306  MySQL Community Server").
				WithStartupTimeout(3*time.Minute),
		),
	)
	if err != nil {
		log.Fatalf("start mysql: %v", err)
	}

	// 与生产同款路径：容器裸 DSN → NormalizeMySQLDSN（parseTime/loc/time_zone）→ store.NewMySQL
	rawDSN, err := container.ConnectionString(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		log.Fatalf("mysql dsn: %v", err)
	}
	dsn, err := config.NormalizeMySQLDSN(rawDSN)
	if err != nil {
		_ = container.Terminate(ctx)
		log.Fatalf("normalize dsn: %v", err)
	}
	db, err := store.NewMySQL(ctx, dsn, store.MySQLOptions{MaxOpenConns: 4}, nil)
	if err != nil {
		_ = container.Terminate(ctx)
		log.Fatalf("connect mysql: %v", err)
	}

	testStorage = NewStore(db)

	code := m.Run()

	db.Close()
	if err := container.Terminate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "terminate mysql: %v\n", err)
	}
	os.Exit(code)
}

func createTestUser(t *testing.T, phone string) *repo.User {
	t.Helper()
	u, err := testStorage.Users().Create(context.Background(), &repo.CreateUserParams{
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
	ctx := context.Background()

	want := createTestUser(t, "13800000001")
	const openID = "openid-oauth-test"

	if _, err := testStorage.OAuth().CreateOAuth(ctx, want.ID, "wechat", openID); err != nil {
		t.Fatalf("CreateOAuth: %v", err)
	}

	got, err := testStorage.OAuth().GetUserByOAuth(ctx, "wechat", openID)
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
	if _, err := testStorage.OAuth().GetUserByOAuth(ctx, "wechat", "openid-missing"); err != repo.ErrNotFound {
		t.Errorf("GetUserByOAuth(missing) err = %v, want ErrNotFound", err)
	}
}

// TestTimestampsUseUTC 锁死时区漂移 bug：写入后回读的 CreatedAt 必须与 UTC
// 当前时间对齐（±2 分钟），不允许偏移服务器本地时区（历史 bug 为 +8h）。
func TestTimestampsUseUTC(t *testing.T) {
	u := createTestUser(t, "13800000002")

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
	ctx := context.Background()

	u := createTestUser(t, "13800000003")

	if _, err := testStorage.Users().UpdateStatus(ctx, u.ID, "banned"); err == nil {
		t.Error("UpdateStatus(banned) = nil, want CHECK violation")
	}
	if _, err := testStorage.Users().UpdateRole(ctx, u.ID, "superuser"); err == nil {
		t.Error("UpdateRole(superuser) = nil, want CHECK violation")
	}

	// 合法值必须仍然可用
	if _, err := testStorage.Users().UpdateStatus(ctx, u.ID, "disabled"); err != nil {
		t.Errorf("UpdateStatus(disabled): %v", err)
	}
	if _, err := testStorage.Users().UpdateRole(ctx, u.ID, "admin"); err != nil {
		t.Errorf("UpdateRole(admin): %v", err)
	}
}

// TestTodoCRUD 基本回归：CRUD、重复删除映射 ErrNotFound。
// 注意 todos.user_id 有外键约束，必须挂到真实用户。
func TestTodoCRUD(t *testing.T) {
	ctx := context.Background()

	u := createTestUser(t, "13800000004")

	todo, err := testStorage.Todos().Create(ctx, u.ID, "first")
	if err != nil {
		t.Fatalf("create todo: %v", err)
	}
	if _, err := testStorage.Todos().Update(ctx, todo.ID, u.ID, "first", true); err != nil {
		t.Fatalf("update todo: %v", err)
	}
	if err := testStorage.Todos().Delete(ctx, todo.ID, u.ID); err != nil {
		t.Fatalf("delete todo: %v", err)
	}
	if err := testStorage.Todos().Delete(ctx, todo.ID, u.ID); err != repo.ErrNotFound {
		t.Errorf("second delete err = %v, want ErrNotFound", err)
	}
}
