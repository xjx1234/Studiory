// cmd/seed 初始化基础数据：确保至少存在一个 admin 用户。
//
// 用法：
//
//	go run ./cmd/seed [--phone=xxx] [--email=xxx] [--password=xxx] [--nickname=xxx]
//
// 所有参数均可通过对应环境变量传入（SEED_ADMIN_PHONE、SEED_ADMIN_EMAIL 等）。
// 若用户已存在则跳过创建，只打印当前 admin 信息。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"backend/internal/config"
	"backend/internal/repo"
	"backend/internal/repo/pg"
	"backend/internal/store"
	"backend/pkg/strutil"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	phone := flag.String("phone", envOr("SEED_ADMIN_PHONE", ""), "admin 手机号（可选）")
	email := flag.String("email", envOr("SEED_ADMIN_EMAIL", ""), "admin 邮箱（可选）")
	password := flag.String("password", envOr("SEED_ADMIN_PASSWORD", ""), "admin 密码（必填）")
	nickname := flag.String("nickname", envOr("SEED_ADMIN_NICKNAME", "管理员"), "admin 昵称")
	flag.Parse()

	if *password == "" {
		fmt.Fprintln(os.Stderr, "错误：必须提供 --password 或环境变量 SEED_ADMIN_PASSWORD")
		os.Exit(1)
	}
	if *phone == "" && *email == "" {
		fmt.Fprintln(os.Stderr, "错误：--phone 或 --email 至少提供一个")
		os.Exit(1)
	}

	cfg := config.Load()
	if cfg.DatabaseURL == "" {
		fmt.Fprintln(os.Stderr, "错误：DATABASE_URL 未配置，请检查 .env 或环境变量")
		os.Exit(1)
	}

	logger, _ := zap.NewDevelopment()
	defer func() { _ = logger.Sync() }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := store.NewPostgres(ctx, cfg.DatabaseURL, store.PostgresOptions{
		MaxConns: 2,
		MinConns: 1,
	}, logger)
	if err != nil {
		logger.Fatal("连接数据库失败", zap.Error(err))
	}
	defer pool.Close()

	pgStore := pg.NewStore(pool)
	users := pgStore.Users()

	// 检查账号是否已存在
	existing, lookupErr := findUser(ctx, users, *phone, *email)
	if lookupErr == nil {
		if existing.Role != repo.RoleAdmin {
			fmt.Fprintf(os.Stderr,
				"错误：账号已存在但角色为 %q（非 admin），请手动将其升级或换一个账号\n  ID: %s\n",
				existing.Role, existing.ID)
			os.Exit(1)
		}
		fmt.Printf("✓ admin 用户已存在，跳过创建\n  ID: %s\n  昵称: %s\n  角色: %s\n",
			existing.ID, existing.Nickname, existing.Role)
		return
	}
	if lookupErr != repo.ErrNotFound {
		logger.Fatal("查询用户失败", zap.Error(lookupErr))
	}

	// 创建 admin 用户
	hash, err := bcrypt.GenerateFromPassword([]byte(*password), bcrypt.DefaultCost)
	if err != nil {
		logger.Fatal("密码哈希失败", zap.Error(err))
	}
	hashStr := string(hash)

	created, err := users.Create(ctx, &repo.CreateUserParams{
		Phone:        strutil.NullableStr(*phone),
		Email:        strutil.NullableStr(*email),
		PasswordHash: &hashStr,
		Nickname:     *nickname,
		Role:         repo.RoleAdmin,
	})
	if err != nil {
		logger.Fatal("创建 admin 用户失败", zap.Error(err))
	}

	fmt.Printf("✓ admin 用户创建成功\n  ID: %s\n  昵称: %s\n  角色: %s\n",
		created.ID, created.Nickname, created.Role)
}

// findUser 按 phone/email 查找用户，不校验角色。
func findUser(ctx context.Context, users repo.UserRepo, phone, email string) (*repo.User, error) {
	if phone != "" {
		u, err := users.GetByPhone(ctx, phone)
		if err == nil {
			return u, nil
		}
		if err != repo.ErrNotFound {
			return nil, err
		}
	}
	if email != "" {
		u, err := users.GetByEmail(ctx, email)
		if err == nil {
			return u, nil
		}
		return nil, err
	}
	return nil, repo.ErrNotFound
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
