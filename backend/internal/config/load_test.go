package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// setEnvs 是 t.Setenv 的批量封装，测试结束后自动恢复。
func setEnvs(t *testing.T, envs map[string]string) {
	t.Helper()
	for k, v := range envs {
		t.Setenv(k, v)
	}
}

func TestLoad_ReadsFromEnvironmentVariables(t *testing.T) {
	// 隔离真实 backend/config 目录，避免受本机/CI 上已存在的 yaml 文件影响。
	t.Setenv("CONFIG_PATH", t.TempDir())

	setEnvs(t, map[string]string{
		"APP_ENV":                    "development",
		"SERVER_ADDR":                ":9090",
		"SERVER_READ_HEADER_TIMEOUT": "5s",
		"SERVER_READ_TIMEOUT":        "15s",
		"SERVER_WRITE_TIMEOUT":       "30s",
		"SERVER_IDLE_TIMEOUT":        "120s",
		"DATABASE_URL":               "postgres://custom-url",
		"REDIS_URL":                  "redis://custom-url",
		"JWT_SECRET":                 "env-secret",
		"JWT_ACCESS_TTL":             "1h",
		"JWT_REFRESH_TTL":            "168h",
		"AUTH_MOCK_CODE_ENABLED":     "true",
		"AUTH_MULTI_DEVICE_ENABLED":  "false",
		"AUTH_REDIS_FAIL_OPEN":       "false",
		"RATE_LIMIT_PER_MINUTE":      "100",
		"RATE_LIMIT_USER_PER_MINUTE": "200",
		"METRICS_ENABLED":            "true",
		"OAUTH_DEV_MODE":             "true",
		"OAUTH_PROVIDERS":            "wechat,apple,google",
		"CORS_ALLOW_ORIGINS":         "https://a.com,https://b.com",
		"CORS_ALLOW_CREDENTIALS":     "true",
	})

	cfg := Load()

	if cfg.AppEnv != "development" {
		t.Errorf("AppEnv = %q, want development", cfg.AppEnv)
	}
	if cfg.ServerAddr != ":9090" {
		t.Errorf("ServerAddr = %q, want :9090", cfg.ServerAddr)
	}
	if cfg.ServerReadHeaderTimeout != 5*time.Second {
		t.Errorf("ServerReadHeaderTimeout = %v, want 5s", cfg.ServerReadHeaderTimeout)
	}
	if cfg.DatabaseURL != "postgres://custom-url" {
		t.Errorf("DatabaseURL = %q, want postgres://custom-url (DATABASE_URL should take priority)", cfg.DatabaseURL)
	}
	if cfg.RedisURL != "redis://custom-url" {
		t.Errorf("RedisURL = %q, want redis://custom-url (REDIS_URL should take priority)", cfg.RedisURL)
	}
	if cfg.JWTSecret != "env-secret" {
		t.Errorf("JWTSecret = %q, want env-secret", cfg.JWTSecret)
	}
	if cfg.JWTAccessTokenTTL != time.Hour {
		t.Errorf("JWTAccessTokenTTL = %v, want 1h", cfg.JWTAccessTokenTTL)
	}
	if !cfg.AuthMockCodeEnabled {
		t.Error("AuthMockCodeEnabled = false, want true")
	}
	if cfg.AuthMultiDeviceEnabled {
		t.Error("AuthMultiDeviceEnabled = true, want false")
	}
	if cfg.AuthRedisFailOpen {
		t.Error("AuthRedisFailOpen = true, want false")
	}
	if cfg.RateLimitPerMinute != 100 {
		t.Errorf("RateLimitPerMinute = %d, want 100", cfg.RateLimitPerMinute)
	}
	if cfg.RateLimitUserPerMinute != 200 {
		t.Errorf("RateLimitUserPerMinute = %d, want 200", cfg.RateLimitUserPerMinute)
	}
	if !cfg.MetricsEnabled {
		t.Error("MetricsEnabled = false, want true")
	}
	if !cfg.OAuthDevMode {
		t.Error("OAuthDevMode = false, want true")
	}
	want := []string{"wechat", "apple", "google"}
	if !equalStringSlices(cfg.OAuthProviders, want) {
		t.Errorf("OAuthProviders = %v, want %v", cfg.OAuthProviders, want)
	}
	wantOrigins := []string{"https://a.com", "https://b.com"}
	if !equalStringSlices(cfg.CORSAllowOrigins, wantOrigins) {
		t.Errorf("CORSAllowOrigins = %v, want %v", cfg.CORSAllowOrigins, wantOrigins)
	}
	if !cfg.CORSAllowCredentials {
		t.Error("CORSAllowCredentials = false, want true")
	}
}

func TestLoad_DatabaseURLFallsBackToComponents(t *testing.T) {
	t.Setenv("CONFIG_PATH", t.TempDir())
	setEnvs(t, map[string]string{
		"DB_HOST":     "db.internal",
		"DB_PORT":     "5432",
		"DB_USER":     "app",
		"DB_PASSWORD": "s3cr3t",
		"DB_NAME":     "appdb",
		"DB_SSL_MODE": "require",
		"REDIS_HOST":  "redis.internal",
		"REDIS_PORT":  "6379",
	})

	cfg := Load()

	want := "postgres://app:s3cr3t@db.internal:5432/appdb?sslmode=require"
	if cfg.DatabaseURL != want {
		t.Errorf("DatabaseURL = %q, want %q", cfg.DatabaseURL, want)
	}
}

func TestLoad_RedisURLFallsBackToComponents(t *testing.T) {
	t.Setenv("CONFIG_PATH", t.TempDir())
	setEnvs(t, map[string]string{
		"REDIS_HOST":     "redis.internal",
		"REDIS_PORT":     "6379",
		"REDIS_PASSWORD": "r3dis",
		"REDIS_DB":       "3",
	})

	cfg := Load()

	want := "redis://:r3dis@redis.internal:6379/3"
	if cfg.RedisURL != want {
		t.Errorf("RedisURL = %q, want %q", cfg.RedisURL, want)
	}
}

func TestLoad_RateLimitUserPerMinuteFallsBackWhenUnset(t *testing.T) {
	t.Setenv("CONFIG_PATH", t.TempDir())
	t.Setenv("RATE_LIMIT_PER_MINUTE", "90")
	// 不设置 RATE_LIMIT_USER_PER_MINUTE，期望回落到 RATE_LIMIT_PER_MINUTE。

	cfg := Load()

	if cfg.RateLimitUserPerMinute != 90 {
		t.Errorf("RateLimitUserPerMinute = %d, want 90 (fallback to RateLimitPerMinute)", cfg.RateLimitUserPerMinute)
	}
}

func TestLoad_YAMLLayersMergeWithCorrectPriority(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "base.yaml", `
database:
  host: base-host
  port: "5432"
app:
  env: development
`)
	writeYAML(t, dir, "development.yaml", `
database:
  host: dev-host
`)
	writeYAML(t, dir, "local.yaml", `
database:
  host: local-host
`)

	t.Setenv("CONFIG_PATH", dir)
	t.Setenv("APP_ENV", "development")

	cfg := Load()

	// local.yaml 优先级最高，应覆盖 development.yaml 和 base.yaml 的同名字段。
	if cfg.DBHost != "local-host" {
		t.Errorf("DBHost = %q, want local-host (local.yaml should win)", cfg.DBHost)
	}
	// 未被覆盖的字段应保留 base.yaml 的值。
	if cfg.DBPort != "5432" {
		t.Errorf("DBPort = %q, want 5432 (from base.yaml)", cfg.DBPort)
	}
}

func TestLoad_EnvironmentVariableOverridesYAML(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "base.yaml", `
database:
  host: base-host
`)
	writeYAML(t, dir, "local.yaml", `
database:
  host: local-host
`)

	t.Setenv("CONFIG_PATH", dir)
	t.Setenv("DB_HOST", "env-host")

	cfg := Load()

	if cfg.DBHost != "env-host" {
		t.Errorf("DBHost = %q, want env-host (环境变量应拥有最高优先级)", cfg.DBHost)
	}
}

func TestLoad_DefaultsToDevelopmentWhenAppEnvUnset(t *testing.T) {
	t.Setenv("CONFIG_PATH", t.TempDir())
	t.Setenv("APP_ENV", "") // 显式置空，避免受宿主机真实环境变量影响

	cfg := Load()

	if !cfg.IsDev() {
		t.Errorf("expected IsDev() to be true when APP_ENV is unset, AppEnv = %q", cfg.AppEnv)
	}
}

func writeYAML(t *testing.T, dir, filename, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", filename, err)
	}
}
