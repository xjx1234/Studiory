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

		JWTAccessTokenTTL:  30 * time.Minute,
		JWTRefreshTokenTTL: 168 * time.Hour,
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

		JWTAccessTokenTTL:  30 * time.Minute,
		JWTRefreshTokenTTL: 168 * time.Hour,
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

		JWTAccessTokenTTL:  30 * time.Minute,
		JWTRefreshTokenTTL: 168 * time.Hour,
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
		ServerRequestTimeout:    5 * time.Second,
		DatabaseURL:             "postgres://postgres:password@localhost:5432/app?sslmode=require",
		RedisURL:                "redis://localhost:6379/0",
		RedisKeyPrefix:          "app",
		RedisReadTimeout:        3 * time.Second,
		RedisWriteTimeout:       3 * time.Second,
		JWTSecret:               "0123456789abcdef0123456789abcdef", // 正好 32 字节
		JWTAccessTokenTTL:       30 * time.Minute,
		JWTRefreshTokenTTL:      168 * time.Hour,
		RateLimitPerMinute:      60,
		CORSAllowOrigins:        []string{"https://example.com"},
		AuthMockCodeEnabled:     false,

		AuthRequireCodeForPasswordRegister: true,
		AuthCodeDailyLimitPerTarget:        10,
		AuthCodeDailyLimitPerIP:            100,
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
		ServerRequestTimeout:    5 * time.Second,
		DatabaseURL:             "postgres://postgres:password@localhost:5432/app?sslmode=disable",
		RedisURL:                "redis://localhost:6379/0",
		RedisKeyPrefix:          "app",
		RedisReadTimeout:        3 * time.Second,
		RedisWriteTimeout:       3 * time.Second,
		JWTSecret:               "dev-secret-change-in-production",
		JWTAccessTokenTTL:       30 * time.Minute,
		JWTRefreshTokenTTL:      168 * time.Hour,
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
		ServerRequestTimeout:    5 * time.Second,
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

// TestValidateRejectsRequestTimeoutLongerThanWriteTimeout 请求超时必须给写回 503 留出时间。
func TestValidateRejectsRequestTimeoutLongerThanWriteTimeout(t *testing.T) {
	cfg := prodBaseConfig()
	cfg.ServerRequestTimeout = cfg.ServerWriteTimeout + time.Second

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected request_timeout > write_timeout to be rejected")
	}
}

// TestValidateRejectsAccessTTLNotShorterThanRefresh 两种 token 的有效期不允许倒挂或相等。
func TestValidateRejectsAccessTTLNotShorterThanRefresh(t *testing.T) {
	for _, ttl := range []time.Duration{168 * time.Hour, 200 * time.Hour} {
		cfg := prodBaseConfig()
		cfg.JWTAccessTokenTTL = ttl

		if err := cfg.Validate(); err == nil {
			t.Errorf("expected access_ttl=%s (>= refresh_ttl) to be rejected", ttl)
		}
	}
}

// TestValidateRejectsWildcardOriginWithCredentials 浏览器不允许 `*` 与 credentials 并用，
// 这类配置应在启动阶段失败，而不是等到前端请求被浏览器静默拦截。
func TestValidateRejectsWildcardOriginWithCredentials(t *testing.T) {
	cfg := prodBaseConfig()
	cfg.CORSAllowOrigins = []string{"https://example.com", "*"}
	cfg.CORSAllowCredentials = true

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected wildcard origin with credentials to be rejected")
	}
}

func TestValidateAllowsWildcardOriginWithoutCredentials(t *testing.T) {
	cfg := prodBaseConfig()
	cfg.CORSAllowOrigins = []string{"*"}
	cfg.CORSAllowCredentials = false

	if err := cfg.Validate(); err != nil {
		t.Fatalf("wildcard origin without credentials should be accepted: %v", err)
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
		ServerRequestTimeout:    5 * time.Second,
		DatabaseURL:             "postgres://postgres:password@localhost:5432/app?sslmode=require",
		RedisURL:                "redis://localhost:6379/0",
		RedisKeyPrefix:          "app",
		RedisReadTimeout:        3 * time.Second,
		RedisWriteTimeout:       3 * time.Second,
		JWTSecret:               "prod-secret-that-is-at-least-32-bytes-long",
		JWTAccessTokenTTL:       30 * time.Minute,
		JWTRefreshTokenTTL:      168 * time.Hour,
		RateLimitPerMinute:      60,
		CORSAllowOrigins:        []string{"https://example.com"},
		AuthMockCodeEnabled:     false,
		// 默认安全值（base.yaml 里就是 true）：关掉它会让密码注册回到「不验归属」的旧行为。
		AuthRequireCodeForPasswordRegister: true,
		// 生产必须开启验证码下发配额（成本保护），默认值与 base.yaml 一致。
		AuthCodeDailyLimitPerTarget: 10,
		AuthCodeDailyLimitPerIP:     100,
	}
}

// TestValidateRejectsDisabledCodeQuotaInProduction 生产环境不允许关闭验证码下发配额：
// 否则单个 IP 每分钟可对 60 个不同号码发码，短信账单不可控。
func TestValidateRejectsDisabledCodeQuotaInProduction(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Config)
	}{
		{"target quota", func(c *Config) { c.AuthCodeDailyLimitPerTarget = 0 }},
		{"ip quota", func(c *Config) { c.AuthCodeDailyLimitPerIP = -1 }},
	}

	for _, tc := range cases {
		cfg := prodBaseConfig()
		tc.mutate(&cfg)
		if err := cfg.Validate(); err == nil {
			t.Errorf("expected disabled %s to be rejected in production", tc.name)
		}
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

// TestValidateAcceptsSMTPWithinSendCodeBudget 默认链路 dial(5)+send(10) < 预算(20) ≤ write(30) 应通过。
func TestValidateAcceptsSMTPWithinSendCodeBudget(t *testing.T) {
	cfg := prodBaseConfig()
	cfg.SMTPHost = "smtp.example.com"
	cfg.SMTPDialTimeout = 5 * time.Second
	cfg.SMTPSendTimeout = 10 * time.Second
	cfg.ServerSendCodeRequestTimeout = 20 * time.Second

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expect the documented default chain to pass, got %v", err)
	}
}

// TestValidateIgnoresSMTPBudgetChainWhenSMTPDisabled 没启用 SMTP 的部署不受新键影响：
// 全部预算/SMTP 超时留空也必须校验通过，否则升级会让原本正常的部署凭空启动失败。
func TestValidateIgnoresSMTPBudgetChainWhenSMTPDisabled(t *testing.T) {
	cfg := prodBaseConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("SMTP-less config with unset timeouts/budget should stay valid, got %v", err)
	}
}

// TestValidateRejectsInconsistentTimeoutBudget 「外层小于内层」这类错配要在启动时炸，
// 而不是等线上发码恒 503 才发现。
func TestValidateRejectsInconsistentTimeoutBudget(t *testing.T) {
	cases := []struct {
		name       string
		dial, send time.Duration
		budget     time.Duration
	}{
		// 预算留空 = 沿用 request_timeout(5s)，装不下 15s 的 SMTP
		{"no budget, slow smtp", 5 * time.Second, 10 * time.Second, 0},
		// 预算恰好等于 dial+send：Redis/DB 与响应写回一点余量都不剩
		{"budget equals smtp sum", 5 * time.Second, 10 * time.Second, 15 * time.Second},
		// 预算超过 write_timeout：503 信封来不及写回客户端
		{"budget above write timeout", 5 * time.Second, 10 * time.Second, 40 * time.Second},
		// 预算小于 request_timeout：写了等于没写，放宽没发生
		{"budget below request timeout", 5 * time.Second, 2 * time.Second, 2 * time.Second},
		// SMTP 超时被配成 0：连接一建立就被打断
		{"zero dial timeout", 0, 10 * time.Second, 20 * time.Second},
		{"zero send timeout", 5 * time.Second, 0, 20 * time.Second},
	}

	for _, tc := range cases {
		cfg := prodBaseConfig()
		cfg.SMTPHost = "smtp.example.com"
		cfg.SMTPDialTimeout = tc.dial
		cfg.SMTPSendTimeout = tc.send
		cfg.ServerSendCodeRequestTimeout = tc.budget

		if err := cfg.Validate(); err == nil {
			t.Errorf("%s: expected Validate() to reject dial=%s send=%s budget=%s",
				tc.name, tc.dial, tc.send, tc.budget)
		}
	}
}

// TestSendCodeRequestBudget 预算取值只有一处实现：中间件与启动校验共用它。
func TestSendCodeRequestBudget(t *testing.T) {
	cfg := prodBaseConfig()
	if got := cfg.SendCodeRequestBudget(); got != cfg.ServerRequestTimeout {
		t.Errorf("未配置单独预算时应沿用 request_timeout，got %s want %s", got, cfg.ServerRequestTimeout)
	}

	cfg.ServerSendCodeRequestTimeout = 20 * time.Second
	if got := cfg.SendCodeRequestBudget(); got != 20*time.Second {
		t.Errorf("配置了单独预算时应取该值，got %s", got)
	}
}
