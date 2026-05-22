package config

import "testing"

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
		JWTSecret:           "prod-secret-with-enough-length",
		RateLimitPerMinute:  60,
		CORSAllowOrigins:    []string{"https://example.com"},
		AuthMockCodeEnabled: false,
		OAuthDevMode:        true,
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected production oauth dev mode to be rejected")
	}
}

func TestValidateAcceptsDevelopmentDefaults(t *testing.T) {
	cfg := &Config{
		AppEnv:              "development",
		ServerAddr:          ":8080",
		DatabaseURL:         "postgres://postgres:password@localhost:5432/app?sslmode=disable",
		RedisURL:            "redis://localhost:6379/0",
		JWTSecret:           "dev-secret-change-in-production",
		RateLimitPerMinute:  120,
		CORSAllowOrigins:    []string{"http://localhost:5173"},
		AuthMockCodeEnabled: true,
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected development config to be valid: %v", err)
	}
}
