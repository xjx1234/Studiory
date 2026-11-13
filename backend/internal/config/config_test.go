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
		// 其余生产项保持安全默认，让这条用例只因 JWT 默认密钥而失败。
		AuthRequireCodeForPasswordRegister: true,
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected production default JWT secret to be rejected")
	}
}

func TestValidateRejectsOAuthDevModeInProduction(t *testing.T) {
	cfg := &Config{
		AppEnv:                             "production",
		ServerAddr:                         ":8080",
		DatabaseURL:                        "postgres://postgres:password@localhost:5432/app?sslmode=disable",
		RedisURL:                           "redis://localhost:6379/0",
		JWTSecret:                          "prod-secret-that-is-at-least-32-bytes-long",
		RateLimitPerMinute:                 60,
		CORSAllowOrigins:                   []string{"https://example.com"},
		AuthMockCodeEnabled:                false,
		AuthRequireCodeForPasswordRegister: true,
		OAuthDevMode:                       true,
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected production oauth dev mode to be rejected")
	}
}

func TestValidateRejectsShortJWTSecretInProduction(t *testing.T) {
	cfg := &Config{
		AppEnv:                             "production",
		ServerAddr:                         ":8080",
		DatabaseURL:                        "postgres://postgres:password@localhost:5432/app?sslmode=disable",
		RedisURL:                           "redis://localhost:6379/0",
		JWTSecret:                          "short-secret-only-20ch",
		RateLimitPerMinute:                 60,
		CORSAllowOrigins:                   []string{"https://example.com"},
		AuthMockCodeEnabled:                false,
		AuthRequireCodeForPasswordRegister: true,
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
		DatabaseURL:             "postgres://postgres:password@localhost:5432/app?sslmode=require",
		RedisURL:                "redis://localhost:6379/0",
		JWTSecret:               "0123456789abcdef0123456789abcdef", // 正好 32 字节
		RateLimitPerMinute:      60,
		CORSAllowOrigins:        []string{"https://example.com"},
		AuthMockCodeEnabled:     false,

		AuthRequireCodeForPasswordRegister: true,
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
		DatabaseURL:             "postgres://postgres:password@localhost:5432/app?sslmode=require",
		RedisURL:                "redis://localhost:6379/0",
		JWTSecret:               "prod-secret-that-is-at-least-32-bytes-long",
		RateLimitPerMinute:      60,
		CORSAllowOrigins:        []string{"https://example.com"},
		AuthMockCodeEnabled:     false,
		// 默认安全值（base.yaml 里就是 true）：关掉它会让密码注册回到「不验归属」的旧行为。
		AuthRequireCodeForPasswordRegister: true,
	}
}

// TestValidateRejectsPasswordRegisterWithoutCodeInProduction 生产环境不允许把密码注册退回「不验验证码」，
// 否则任何人都能抢注他人手机号/邮箱，并用 /register 枚举已注册账号。
func TestValidateRejectsPasswordRegisterWithoutCodeInProduction(t *testing.T) {
	cfg := prodBaseConfig()
	cfg.AuthRequireCodeForPasswordRegister = false

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected disabling the password-register code check to be rejected in production")
	}
}

func TestValidateAcceptsPasswordRegisterWithoutCodeOutsideProduction(t *testing.T) {
	cfg := prodBaseConfig()
	cfg.AppEnv = "development"
	cfg.AuthRequireCodeForPasswordRegister = false

	if err := cfg.Validate(); err != nil {
		t.Fatalf("开发/内网部署允许显式关掉该开关: %v", err)
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

func TestValidateRejectsInsecureDatabaseSSLModeInProduction(t *testing.T) {
	insecure := []string{
		"postgres://postgres:password@localhost:5432/app?sslmode=disable",
		"postgres://postgres:password@localhost:5432/app?sslmode=prefer",
		// 未携带 sslmode：pgx 按 prefer 处理，可能静默回落明文，同样拒绝
		"postgres://postgres:password@localhost:5432/app",
		"host=localhost port=5432 dbname=app sslmode=disable",
	}
	for _, dsn := range insecure {
		cfg := prodBaseConfig()
		cfg.DatabaseURL = dsn
		if err := cfg.Validate(); err == nil {
			t.Errorf("expected insecure DSN to be rejected in production: %s", dsn)
		}
	}
}

func TestValidateAcceptsSecureDatabaseSSLModeInProduction(t *testing.T) {
	secure := []string{
		"postgres://postgres:password@localhost:5432/app?sslmode=require",
		"postgres://postgres:password@localhost:5432/app?sslmode=verify-full",
		"host=localhost port=5432 dbname=app sslmode=verify-ca",
	}
	for _, dsn := range secure {
		cfg := prodBaseConfig()
		cfg.DatabaseURL = dsn
		if err := cfg.Validate(); err != nil {
			t.Errorf("expected secure DSN to be accepted in production: %s, got %v", dsn, err)
		}
	}
}

func TestValidateRejectsMetricsWithoutTokenInProduction(t *testing.T) {
	cfg := prodBaseConfig()
	cfg.MetricsEnabled = true
	// MetricsToken 为空

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected metrics without token to be rejected in production")
	}
}

func TestValidateAcceptsMetricsWithTokenInProduction(t *testing.T) {
	cfg := prodBaseConfig()
	cfg.MetricsEnabled = true
	cfg.MetricsToken = "secret-metrics-token"

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected metrics with token to be accepted in production: %v", err)
	}
}

// TestValidateAcceptsTrustedProxies 合法的 IP / IPv6 / CIDR 都应通过校验。
// 这些值会原样交给 gin 的 SetTrustedProxies，校验口径必须与 gin 一致，
// 否则会出现「配置能读、启动即挂」的假阳性。
func TestValidateAcceptsTrustedProxies(t *testing.T) {
	valid := [][]string{
		nil,
		{},
		{"192.168.1.10"},
		{"10.0.0.0/8"},
		{"::1"},
		{"fd00::/64"},
		{"10.0.0.0/8", "172.16.0.1", "fd00::/64"},
	}

	for _, proxies := range valid {
		cfg := prodBaseConfig()
		cfg.ServerTrustedProxies = proxies
		if err := cfg.Validate(); err != nil {
			t.Errorf("expected trusted proxies %v to be accepted, got %v", proxies, err)
		}
	}
}

func TestValidateRejectsInvalidTrustedProxies(t *testing.T) {
	invalid := []string{
		"not-an-ip",
		"192.168.1.256",
		"10.0.0.0/33",
		"10.0.0.0/8/1",
		"",
	}

	for _, proxy := range invalid {
		cfg := prodBaseConfig()
		cfg.ServerTrustedProxies = []string{proxy}
		if err := cfg.Validate(); err == nil {
			t.Errorf("expected trusted proxy %q to be rejected", proxy)
		}
	}
}
