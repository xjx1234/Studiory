package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"backend/internal/app"
	"backend/internal/buildinfo"
	"backend/internal/config"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "配置错误: %v\n", err)
		os.Exit(1)
	}

	logger := initLogger(cfg)
	defer func() { _ = logger.Sync() }()
	zap.ReplaceGlobals(logger)

	zap.L().Info("配置加载完成",
		zap.String("env", cfg.AppEnv),
		zap.String("addr", cfg.ServerAddr),
		zap.String("version", buildinfo.Version),
		zap.String("commit", buildinfo.Commit),
		zap.String("build_time", buildinfo.BuildTime),
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	a, err := app.New(ctx, cfg, zap.L())
	if err != nil {
		zap.L().Fatal("应用初始化失败", zap.Error(err))
	}

	zap.L().Info("API 服务启动", zap.String("addr", cfg.ServerAddr))

	// 非阻塞启动 HTTP Server，让主 goroutine 等待退出信号
	go func() {
		if err := a.Server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			zap.L().Fatal("服务器启动失败", zap.Error(err))
		}
	}()

	// 等待退出信号（Ctrl+C 或 SIGTERM）
	<-ctx.Done()
	zap.L().Info("收到退出信号，开始优雅停机")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	a.Shutdown(shutdownCtx)

	zap.L().Info("服务已安全退出")
}

// initLogger 根据配置初始化 Zap Logger。
func initLogger(cfg *config.Config) *zap.Logger {
	var (
		logger *zap.Logger
		err    error
	)

	level := zap.NewAtomicLevel()
	if err := level.UnmarshalText([]byte(strings.ToLower(cfg.LogLevel))); err != nil {
		level.SetLevel(zap.InfoLevel)
	}

	switch cfg.LogFormat {
	case "json":
		zapCfg := zap.NewProductionConfig()
		zapCfg.Level = level
		zapCfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		logger, err = zapCfg.Build()
	default:
		zapCfg := zap.NewDevelopmentConfig()
		zapCfg.Level = level
		logger, err = zapCfg.Build()
	}

	if err != nil {
		panic("初始化 zap logger 失败: " + err.Error())
	}

	return logger
}
