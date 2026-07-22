package config

import (
	"testing"
	"time"
)

func TestValidateRejectsUnsafeProductionDefaults(t *testing.T) {
	cfg := &Config{
		AppEnv:              "production",
		ServerAddr:          ":8080",
		DatabaseURL:         "postgres://postgres:password@localhost:5432/app?sslmode=disable",
		RedisURL:            "redis://localhost:6379/0",
		JWTSecret:           "dev-secret-change-in-production",
		RateLimitPerMinute:  60,
		CORSAllowOrigins:    []string{"https://example.com"},
		AuthMockCodeEnabled: false,
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected production default JWT secret to be rejected")
	}
}

func TestValidateRejectsOAuthDevModeInProduction(t *testing.T) {
	cfg := &Config{
		AppEnv:              "production",
		ServerAddr:          ":8080",
		DatabaseURL:         "postgres://postgres:password@localhost:5432/app?sslmode=disable",
		RedisURL:            "redis://localhost:6379/0",
		JWTSecret:           "prod-secret-that-is-at-least-32-bytes-long",
		RateLimitPerMinute:  60,
		CORSAllowOrigins:    []string{"https://example.com"},
		AuthMockCodeEnabled: false,
		OAuthDevMode:        true,
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected production oauth dev mode to be rejected")
	}
}

func TestValidateRejectsShortJWTSecretInProduction(t *testing.T) {
	cfg := &Config{
		AppEnv:              "production",
		ServerAddr:          ":8080",
		DatabaseURL:         "postgres://postgres:password@localhost:5432/app?sslmode=disable",
		RedisURL:            "redis://localhost:6379/0",
		JWTSecret:           "short-secret-only-20ch",
		RateLimitPerMinute:  60,
		CORSAllowOrigins:    []string{"https://example.com"},
		AuthMockCodeEnabled: false,
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected short JWT secret to be rejected in production")
	}
}

func TestValidateAccepts32ByteJWTSecretInProduction(t *testing.T) {
	cfg := &Config{
		AppEnv:                  "production",
		ServerAddr:              ":8080",
		ServerReadHeaderTimeout: 5 * time.Second,
		ServerReadTimeout:       15 * time.Second,
		ServerWriteTimeout:      30 * time.Second,
		ServerIdleTimeout:       120 * time.Second,
		DatabaseURL:             "postgres://postgres:password@localhost:5432/app?sslmode=disable",
		RedisURL:                "redis://localhost:6379/0",
		JWTSecret:               "0123456789abcdef0123456789abcdef", // 正好 32 字节
		RateLimitPerMinute:      60,
		CORSAllowOrigins:        []string{"https://example.com"},
		AuthMockCodeEnabled:     false,
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected 32-byte JWT secret to be accepted in production: %v", err)
	}
}

func TestValidateAcceptsDevelopmentDefaults(t *testing.T) {
	cfg := &Config{
		AppEnv:                  "development",
		ServerAddr:              ":8080",
		ServerReadHeaderTimeout: 5 * time.Second,
		ServerReadTimeout:       15 * time.Second,
		ServerWriteTimeout:      30 * time.Second,
		ServerIdleTimeout:       120 * time.Second,
		DatabaseURL:             "postgres://postgres:password@localhost:5432/app?sslmode=disable",
		RedisURL:                "redis://localhost:6379/0",
		JWTSecret:               "dev-secret-change-in-production",
		RateLimitPerMinute:      120,
		CORSAllowOrigins:        []string{"http://localhost:5173"},
		AuthMockCodeEnabled:     true,
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected development config to be valid: %v", err)
	}
}

func TestValidateRejectsInvalidServerTimeouts(t *testing.T) {
	base := Config{
		AppEnv:                  "development",
		ServerAddr:              ":8080",
		ServerReadHeaderTimeout: 5 * time.Second,
		ServerReadTimeout:       15 * time.Second,
		ServerWriteTimeout:      30 * time.Second,
		ServerIdleTimeout:       120 * time.Second,
		DatabaseURL:             "postgres://postgres:password@localhost:5432/app?sslmode=disable",
		RedisURL:                "redis://localhost:6379/0",
		JWTSecret:               "dev-secret",
		RateLimitPerMinute:      120,
		CORSAllowOrigins:        []string{"http://localhost:5173"},
	}

	cfg := base
	cfg.ServerReadTimeout = 3 * time.Second
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected read_timeout < read_header_timeout to be rejected")
	}
}

func prodBaseConfig() Config {
	return Config{
		AppEnv:                  "production",
		ServerAddr:              ":8080",
		ServerReadHeaderTimeout: 5 * time.Second,
		ServerReadTimeout:       15 * time.Second,
		ServerWriteTimeout:      30 * time.Second,
		ServerIdleTimeout:       120 * time.Second,
		DatabaseURL:             "postgres://postgres:password@localhost:5432/app?sslmode=disable",
		RedisURL:                "redis://localhost:6379/0",
		JWTSecret:               "prod-secret-that-is-at-least-32-bytes-long",
		RateLimitPerMinute:      60,
		CORSAllowOrigins:        []string{"https://example.com"},
		AuthMockCodeEnabled:     false,
	}
}

func TestValidateRejectsGoogleWithoutClientIDInProduction(t *testing.T) {
	cfg := prodBaseConfig()
	cfg.OAuthProviders = []string{"google"}
	// OAuthGoogleClientID 为空

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected google without client_id to be rejected in production")
	}
}

func TestValidateRejectsAppleWithoutClientIDInProduction(t *testing.T) {
	cfg := prodBaseConfig()
	cfg.OAuthProviders = []string{"apple"}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected apple without client_id to be rejected in production")
	}
}

func TestValidateRejectsWechatWithoutAppIDInProduction(t *testing.T) {
	cfg := prodBaseConfig()
	cfg.OAuthProviders = []string{"wechat"}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected wechat without app_id to be rejected in production")
	}
}

func TestValidateAcceptsConfiguredOAuthProvidersInProduction(t *testing.T) {
	cfg := prodBaseConfig()
	cfg.OAuthProviders = []string{"google", "apple", "wechat"}
	cfg.OAuthGoogleClientID = "google-client-id"
	cfg.OAuthAppleClientID = "apple-client-id"
	cfg.OAuthWechatAppID = "wechat-app-id"

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected fully configured OAuth providers to be accepted: %v", err)
	}
}
