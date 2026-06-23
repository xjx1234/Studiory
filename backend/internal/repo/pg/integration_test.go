//go:build integration

// Package pg 集成测试：用 testcontainers 起一个临时 PostgreSQL，
// 跑真实迁移后验证 repo 层 CRUD、唯一约束、NotFound 等行为。
//
// 运行方式（需要本机有 Docker）：
//
//	go test -tags=integration ./internal/repo/pg/...
//
// 不带 -tags=integration 时本文件不参与编译，普通单测与 CI 主测试任务不受影响。
package pg

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"backend/internal/repo"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var testStore *Store

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("app_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		log.Fatalf("start postgres container: %v", err)
	}

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = container.Terminate(ctx)
		log.Fatalf("connection string: %v", err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		_ = container.Terminate(ctx)
		log.Fatalf("connect pool: %v", err)
	}

	if err := applyMigrations(ctx, pool); err != nil {
		pool.Close()
		_ = container.Terminate(ctx)
		log.Fatalf("apply migrations: %v", err)
	}

	testStore = NewStore(pool)

	code := m.Run()

	pool.Close()
	_ = container.Terminate(ctx)
	os.Exit(code)
}

// applyMigrations 按文件名顺序执行 migrations 目录下所有 *.up.sql。
func applyMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	dir := filepath.Join("..", "..", "..", "migrations")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, name := range files {
		sqlBytes, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			return err
		}
	}
	return nil
}

// strPtr 返回字符串指针，方便构造可空字段。
func strPtr(s string) *string { return &s }

// uniquePhone 生成一个唯一的合法手机号片段（避免跨用例冲突）。
func uniquePhone() string {
	// 138 + 8 位时间戳尾数，保证长度合法且唯一
	return "138" + uuid.NewString()[0:8]
}

func createTestUser(t *testing.T, role string) *repo.User {
	t.Helper()
	users := testStore.Users()
	u, err := users.Create(context.Background(), &repo.CreateUserParams{
		Phone:        strPtr(uniquePhone()),
		Email:        strPtr(uuid.NewString() + "@example.com"),
		PasswordHash: strPtr("hash"),
		Nickname:     "tester",
		Role:         role,
	})
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}
	return u
}

// ── 用户 ───────────────────────────────────────────────────────────────────────

func TestUserRepo_CreateAndGet(t *testing.T) {
	ctx := context.Background()
	users := testStore.Users()

	phone := uniquePhone()
	email := uuid.NewString() + "@example.com"
	created, err := users.Create(ctx, &repo.CreateUserParams{
		Phone:        strPtr(phone),
		Email:        strPtr(email),
		PasswordHash: strPtr("hash"),
		Nickname:     "alice",
		Role:         repo.RoleUser,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == uuid.Nil {
		t.Fatal("expected non-nil user ID")
	}

	byID, err := users.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if byID.Nickname != "alice" || byID.Role != repo.RoleUser {
		t.Errorf("GetByID mismatch: %+v", byID)
	}

	byPhone, err := users.GetByPhone(ctx, phone)
	if err != nil {
		t.Fatalf("GetByPhone: %v", err)
	}
	if byPhone.ID != created.ID {
		t.Errorf("GetByPhone ID = %v, want %v", byPhone.ID, created.ID)
	}

	byEmail, err := users.GetByEmail(ctx, email)
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}
	if byEmail.ID != created.ID {
		t.Errorf("GetByEmail ID = %v, want %v", byEmail.ID, created.ID)
	}
}

func TestUserRepo_DuplicatePhone_AlreadyExists(t *testing.T) {
	ctx := context.Background()
	users := testStore.Users()

	phone := uniquePhone()
	_, err := users.Create(ctx, &repo.CreateUserParams{
		Phone:    strPtr(phone),
		Nickname: "first",
		Role:     repo.RoleUser,
	})
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}

	_, err = users.Create(ctx, &repo.CreateUserParams{
		Phone:    strPtr(phone),
		Nickname: "second",
		Role:     repo.RoleUser,
	})
	if !errors.Is(err, repo.ErrAlreadyExists) {
		t.Errorf("duplicate phone error = %v, want ErrAlreadyExists", err)
	}
}

func TestUserRepo_GetByID_NotFound(t *testing.T) {
	ctx := context.Background()
	users := testStore.Users()

	_, err := users.GetByID(ctx, uuid.New())
	if !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("GetByID(random) error = %v, want ErrNotFound", err)
	}
}

func TestUserRepo_DefaultStatusActive(t *testing.T) {
	u := createTestUser(t, repo.RoleUser)
	if u.Status != repo.StatusActive {
		t.Errorf("new user status = %q, want %q", u.Status, repo.StatusActive)
	}
}

func TestUserRepo_UpdateRoleAndStatus(t *testing.T) {
	ctx := context.Background()
	users := testStore.Users()
	u := createTestUser(t, repo.RoleUser)

	roleUpdated, err := users.UpdateRole(ctx, u.ID, repo.RoleAdmin)
	if err != nil {
		t.Fatalf("UpdateRole: %v", err)
	}
	if roleUpdated.Role != repo.RoleAdmin {
		t.Errorf("role = %q, want admin", roleUpdated.Role)
	}

	statusUpdated, err := users.UpdateStatus(ctx, u.ID, repo.StatusDisabled)
	if err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	if statusUpdated.Status != repo.StatusDisabled {
		t.Errorf("status = %q, want disabled", statusUpdated.Status)
	}

	if _, err := users.UpdateRole(ctx, uuid.New(), repo.RoleAdmin); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("UpdateRole(random) error = %v, want ErrNotFound", err)
	}
}

func TestUserRepo_ListAndCount_KeywordAndStatusFilter(t *testing.T) {
	ctx := context.Background()
	users := testStore.Users()

	marker := uuid.NewString()[0:8]
	nick := "list-" + marker
	// 两个 active、一个 disabled，昵称含唯一 marker 便于过滤定位
	for i := 0; i < 2; i++ {
		if _, err := users.Create(ctx, &repo.CreateUserParams{
			Phone:    strPtr(uniquePhone()),
			Nickname: nick,
			Role:     repo.RoleUser,
		}); err != nil {
			t.Fatalf("create active user: %v", err)
		}
	}
	disabled, err := users.Create(ctx, &repo.CreateUserParams{
		Phone:    strPtr(uniquePhone()),
		Nickname: nick,
		Role:     repo.RoleUser,
	})
	if err != nil {
		t.Fatalf("create user to disable: %v", err)
	}
	if _, err := users.UpdateStatus(ctx, disabled.ID, repo.StatusDisabled); err != nil {
		t.Fatalf("disable user: %v", err)
	}

	// 关键字过滤：应命中 3 个
	total, err := users.Count(ctx, nick, "")
	if err != nil {
		t.Fatalf("Count(keyword): %v", err)
	}
	if total != 3 {
		t.Errorf("Count(keyword) = %d, want 3", total)
	}

	// 关键字 + 状态过滤：disabled 只有 1 个
	disabledTotal, err := users.Count(ctx, nick, repo.StatusDisabled)
	if err != nil {
		t.Fatalf("Count(keyword,disabled): %v", err)
	}
	if disabledTotal != 1 {
		t.Errorf("Count(keyword,disabled) = %d, want 1", disabledTotal)
	}

	// 分页：page_size=2 应返回 2 条
	list, err := users.List(ctx, &repo.ListUsersParams{Keyword: nick, Limit: 2, Offset: 0})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("List len = %d, want 2 (page size)", len(list))
	}
}

func TestUserRepo_UpdatePasswordAndProfile(t *testing.T) {
	ctx := context.Background()
	users := testStore.Users()
	u := createTestUser(t, repo.RoleUser)

	updated, err := users.UpdatePassword(ctx, u.ID, "new-hash")
	if err != nil {
		t.Fatalf("UpdatePassword: %v", err)
	}
	if updated.PasswordHash == nil || *updated.PasswordHash != "new-hash" {
		t.Errorf("password not updated: %+v", updated.PasswordHash)
	}

	profile, err := users.UpdateProfile(ctx, u.ID, "bob", "http://avatar")
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if profile.Nickname != "bob" || profile.Avatar != "http://avatar" {
		t.Errorf("profile not updated: %+v", profile)
	}
}

// ── OAuth ──────────────────────────────────────────────────────────────────────

func TestOAuthRepo_CreateAndGet(t *testing.T) {
	ctx := context.Background()
	u := createTestUser(t, repo.RoleUser)
	oauth := testStore.OAuth()

	openID := uuid.NewString()
	created, err := oauth.CreateOAuth(ctx, u.ID, "wechat", openID)
	if err != nil {
		t.Fatalf("CreateOAuth: %v", err)
	}
	if created.UserID != u.ID {
		t.Errorf("CreateOAuth UserID = %v, want %v", created.UserID, u.ID)
	}

	got, err := oauth.GetOAuth(ctx, "wechat", openID)
	if err != nil {
		t.Fatalf("GetOAuth: %v", err)
	}
	if got.OpenID != openID {
		t.Errorf("GetOAuth OpenID = %q, want %q", got.OpenID, openID)
	}

	gotUser, err := oauth.GetUserByOAuth(ctx, "wechat", openID)
	if err != nil {
		t.Fatalf("GetUserByOAuth: %v", err)
	}
	if gotUser.ID != u.ID {
		t.Errorf("GetUserByOAuth ID = %v, want %v", gotUser.ID, u.ID)
	}
}

func TestOAuthRepo_Duplicate_AlreadyExists(t *testing.T) {
	ctx := context.Background()
	u := createTestUser(t, repo.RoleUser)
	oauth := testStore.OAuth()

	openID := uuid.NewString()
	if _, err := oauth.CreateOAuth(ctx, u.ID, "google", openID); err != nil {
		t.Fatalf("first CreateOAuth: %v", err)
	}
	_, err := oauth.CreateOAuth(ctx, u.ID, "google", openID)
	if !errors.Is(err, repo.ErrAlreadyExists) {
		t.Errorf("duplicate oauth error = %v, want ErrAlreadyExists", err)
	}
}

func TestOAuthRepo_GetOAuth_NotFound(t *testing.T) {
	ctx := context.Background()
	oauth := testStore.OAuth()

	_, err := oauth.GetOAuth(ctx, "apple", uuid.NewString())
	if !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("GetOAuth(random) error = %v, want ErrNotFound", err)
	}
}

// ── Todo ───────────────────────────────────────────────────────────────────────

func TestTodoRepo_CRUD(t *testing.T) {
	ctx := context.Background()
	u := createTestUser(t, repo.RoleUser)
	todos := testStore.Todos()

	created, err := todos.Create(ctx, u.ID, "first todo")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Title != "first todo" || created.Done {
		t.Errorf("unexpected created todo: %+v", created)
	}

	got, err := todos.GetByIDAndUserID(ctx, created.ID, u.ID)
	if err != nil {
		t.Fatalf("GetByIDAndUserID: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("Get ID = %v, want %v", got.ID, created.ID)
	}

	count, err := todos.CountByUserID(ctx, u.ID)
	if err != nil {
		t.Fatalf("CountByUserID: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}

	list, err := todos.ListByUserIDPaginated(ctx, u.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListByUserIDPaginated: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("list len = %d, want 1", len(list))
	}

	updated, err := todos.Update(ctx, created.ID, u.ID, "updated", true)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Title != "updated" || !updated.Done {
		t.Errorf("unexpected updated todo: %+v", updated)
	}
}

func TestTodoRepo_Delete_NotFoundOnSecondDelete(t *testing.T) {
	ctx := context.Background()
	u := createTestUser(t, repo.RoleUser)
	todos := testStore.Todos()

	created, err := todos.Create(ctx, u.ID, "to delete")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := todos.Delete(ctx, created.ID, u.ID); err != nil {
		t.Fatalf("first Delete: %v", err)
	}

	// 第二次删除：记录已不存在 → ErrNotFound（验证 DeleteTodo RETURNING + wrapErr）
	if err := todos.Delete(ctx, created.ID, u.ID); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("second Delete error = %v, want ErrNotFound", err)
	}
}

func TestTodoRepo_Get_NotFound(t *testing.T) {
	ctx := context.Background()
	u := createTestUser(t, repo.RoleUser)
	todos := testStore.Todos()

	_, err := todos.GetByIDAndUserID(ctx, uuid.New(), u.ID)
	if !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("Get(random) error = %v, want ErrNotFound", err)
	}
}

// ── 事务 ───────────────────────────────────────────────────────────────────────

func TestStore_WithinTx_Rollback(t *testing.T) {
	ctx := context.Background()
	sentinel := errors.New("rollback please")

	phone := uniquePhone()
	err := testStore.WithinTx(ctx, func(txStore *Store) error {
		if _, err := txStore.Users().Create(ctx, &repo.CreateUserParams{
			Phone:    strPtr(phone),
			Nickname: "tx-user",
			Role:     repo.RoleUser,
		}); err != nil {
			return err
		}
		return sentinel // 触发回滚
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("WithinTx err = %v, want sentinel", err)
	}

	// 回滚后该用户不应存在
	if _, err := testStore.Users().GetByPhone(ctx, phone); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("after rollback GetByPhone err = %v, want ErrNotFound", err)
	}
}
