//go:build integration

// Package integration 提供 E2E / 集成测试共用的 testcontainers 启动与迁移工具。
package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Postgres 封装临时 PostgreSQL 容器。
type Postgres struct {
	DSN       string
	Pool      *pgxpool.Pool
	container *postgres.PostgresContainer
}

// StartPostgres 启动 PostgreSQL 16 并执行 migrations。
func StartPostgres(ctx context.Context) (*Postgres, error) {
	container, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("app_e2e"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(90*time.Second),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("start postgres: %w", err)
	}

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("postgres dsn: %w", err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("postgres pool: %w", err)
	}

	if err := ApplyMigrations(ctx, pool); err != nil {
		pool.Close()
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("apply migrations: %w", err)
	}

	return &Postgres{DSN: dsn, Pool: pool, container: container}, nil
}

// MustStartPostgres 同 StartPostgres，失败时终止测试。
func MustStartPostgres(ctx context.Context, t *testing.T) *Postgres {
	t.Helper()
	p, err := StartPostgres(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func (p *Postgres) Close(ctx context.Context) {
	if p == nil {
		return
	}
	if p.Pool != nil {
		p.Pool.Close()
	}
	if p.container != nil {
		_ = p.container.Terminate(ctx)
	}
}

// Redis 封装临时 Redis 容器。
type Redis struct {
	URL       string
	container testcontainers.Container
}

// StartRedis 启动 Redis 7。
func StartRedis(ctx context.Context) (*Redis, error) {
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "redis:7-alpine",
			ExposedPorts: []string{"6379/tcp"},
			WaitingFor:   wait.ForListeningPort("6379/tcp").WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		return nil, fmt.Errorf("start redis: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("redis host: %w", err)
	}
	port, err := container.MappedPort(ctx, "6379/tcp")
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("redis port: %w", err)
	}

	return &Redis{
		URL:       fmt.Sprintf("redis://%s:%s/0", host, port.Port()),
		container: container,
	}, nil
}

// MustStartRedis 同 StartRedis，失败时终止测试。
func MustStartRedis(ctx context.Context, t *testing.T) *Redis {
	t.Helper()
	r, err := StartRedis(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func (r *Redis) Close(ctx context.Context) {
	if r == nil || r.container == nil {
		return
	}
	_ = r.container.Terminate(ctx)
}

// ApplyMigrations 按文件名顺序执行 backend/migrations 下所有 *.up.sql。
func ApplyMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	dir := MigrationsDir()
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
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

// MigrationsDir 返回 backend/migrations 绝对路径。
func MigrationsDir() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "migrations"
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "migrations"))
}
