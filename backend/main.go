package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"backend/internal/config"
	internalhttp "backend/internal/http"
	"backend/internal/store"
	pkgvalidator "backend/pkg/validator"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	logger := initLogger()
	defer logger.Sync()
	zap.ReplaceGlobals(logger)

	cfg := config.Load()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// PostgreSQL
	pool, err := store.NewPostgres(ctx, cfg.DatabaseURL, zap.L())
	if err != nil {
		zap.L().Fatal("PostgreSQL 连接失败", zap.Error(err))
	}
	defer pool.Close()

	// Redis
	rdb, err := store.NewRedis(ctx, cfg.RedisURL, zap.L())
	if err != nil {
		zap.L().Fatal("Redis 连接失败", zap.Error(err))
	}
	defer rdb.Close()

	// 初始化参数校验器（注册 zh/en 翻译 + 自定义规则）
	pkgvalidator.Init()

	// 后续可将 pool、rdb 注入到 router 或 handler 中使用
	_ = pool
	_ = rdb

	r := internalhttp.NewRouter()

	zap.L().Info("拾习社主后端启动", zap.String("addr", cfg.ServerAddr))

	if err := r.Run(cfg.ServerAddr); err != nil {
		zap.L().Fatal("服务器启动失败", zap.Error(err))
	}
}

// initLogger 初始化 Zap。
// 开发环境用彩色控制台输出；生产环境通过 APP_ENV=production 切换为 JSON 格式。
func initLogger() *zap.Logger {
	var (
		logger *zap.Logger
		err    error
	)

	if os.Getenv("APP_ENV") == "production" {
		cfg := zap.NewProductionConfig()
		cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		logger, err = cfg.Build()
	} else {
		logger, err = zap.NewDevelopment()
	}

	if err != nil {
		panic("初始化 zap logger 失败: " + err.Error())
	}

	return logger
}
