package config

import (
	"testing"

	"github.com/spf13/viper"
)

// newTestViper 构造一个仅包含给定嵌套配置的 viper 实例，用于隔离测试 getStringSlice 等辅助函数，
// 不受真实 config/ 目录或进程环境变量影响。
func newTestViper(t *testing.T, data map[string]any) *viper.Viper {
	t.Helper()
	v := viper.New()
	if err := v.MergeConfigMap(data); err != nil {
		t.Fatalf("MergeConfigMap failed: %v", err)
	}
	return v
}

func TestIsDev(t *testing.T) {
	cases := []struct {
		env  string
		want bool
	}{
		{"development", true},
		{"", true}, // 空值默认视为开发环境
		{"production", false},
		{"staging", false},
	}
	for _, tc := range cases {
		cfg := &Config{AppEnv: tc.env}
		if got := cfg.IsDev(); got != tc.want {
			t.Errorf("IsDev() with AppEnv=%q = %v, want %v", tc.env, got, tc.want)
		}
	}
}

func TestIsProd(t *testing.T) {
	cases := []struct {
		env  string
		want bool
	}{
		{"production", true},
		{"development", false},
		{"", false},
		{"staging", false},
	}
	for _, tc := range cases {
		cfg := &Config{AppEnv: tc.env}
		if got := cfg.IsProd(); got != tc.want {
			t.Errorf("IsProd() with AppEnv=%q = %v, want %v", tc.env, got, tc.want)
		}
	}
}

func TestBuildDatabaseURL_WithPassword(t *testing.T) {
	cfg := &Config{
		DBHost: "localhost", DBPort: "5432", DBUser: "app",
		DBPassword: "secret", DBName: "appdb", DBSSLMode: "disable",
	}
	want := "postgres://app:secret@localhost:5432/appdb?sslmode=disable"
	if got := cfg.buildDatabaseURL(); got != want {
		t.Errorf("buildDatabaseURL() = %q, want %q", got, want)
	}
}

func TestBuildDatabaseURL_WithoutPassword(t *testing.T) {
	cfg := &Config{
		DBHost: "localhost", DBPort: "5432", DBUser: "app",
		DBName: "appdb", DBSSLMode: "require",
	}
	want := "postgres://app@localhost:5432/appdb?sslmode=require"
	if got := cfg.buildDatabaseURL(); got != want {
		t.Errorf("buildDatabaseURL() = %q, want %q", got, want)
	}
}

func TestBuildRedisURL_WithPassword(t *testing.T) {
	cfg := &Config{RedisHost: "localhost", RedisPort: "6379", RedisPassword: "secret", RedisDB: 2}
	want := "redis://:secret@localhost:6379/2"
	if got := cfg.buildRedisURL(); got != want {
		t.Errorf("buildRedisURL() = %q, want %q", got, want)
	}
}

func TestBuildRedisURL_WithoutPassword(t *testing.T) {
	cfg := &Config{RedisHost: "localhost", RedisPort: "6379", RedisDB: 0}
	want := "redis://localhost:6379/0"
	if got := cfg.buildRedisURL(); got != want {
		t.Errorf("buildRedisURL() = %q, want %q", got, want)
	}
}

func TestValidate_RequiredFieldsIndividually(t *testing.T) {
	validBase := func() Config {
		return Config{
			AppEnv:                  "development",
			ServerAddr:              ":8080",
			ServerReadHeaderTimeout: 5,
			ServerReadTimeout:       15,
			ServerWriteTimeout:      30,
			ServerIdleTimeout:       120,
			DatabaseURL:             "postgres://u:p@localhost:5432/app?sslmode=disable",
			RedisURL:                "redis://localhost:6379/0",
			JWTSecret:               "some-secret",
			RateLimitPerMinute:      60,
			CORSAllowOrigins:        []string{"https://example.com"},
		}
	}

	// 基线配置本身必须先验证通过，否则下面的“单字段致坏”用例失去意义。
	base := validBase()
	if err := base.Validate(); err != nil {
		t.Fatalf("baseline valid config unexpectedly rejected: %v", err)
	}

	cases := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{"empty server addr", func(c *Config) { c.ServerAddr = "" }, "app.server_addr"},
		{"zero read header timeout", func(c *Config) { c.ServerReadHeaderTimeout = 0 }, "app.read_header_timeout"},
		{"zero read timeout", func(c *Config) { c.ServerReadTimeout = 0 }, "app.read_timeout"},
		{"zero write timeout", func(c *Config) { c.ServerWriteTimeout = 0 }, "app.write_timeout"},
		{"zero idle timeout", func(c *Config) { c.ServerIdleTimeout = 0 }, "app.idle_timeout"},
		{"empty database url", func(c *Config) { c.DatabaseURL = "" }, "database.url"},
		{"empty redis url", func(c *Config) { c.RedisURL = "" }, "redis.url"},
		{"empty jwt secret", func(c *Config) { c.JWTSecret = "" }, "jwt.secret"},
		{"zero rate limit", func(c *Config) { c.RateLimitPerMinute = 0 }, "rate_limit.per_minute"},
		{"empty cors origins", func(c *Config) { c.CORSAllowOrigins = nil }, "cors.allow_origins"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validBase()
			tc.mutate(&cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
		})
	}
}

func TestValidate_RejectsReadTimeoutLessThanReadHeaderTimeout(t *testing.T) {
	cfg := Config{
		AppEnv:                  "development",
		ServerAddr:              ":8080",
		ServerReadHeaderTimeout: 10,
		ServerReadTimeout:       5, // < ServerReadHeaderTimeout
		ServerWriteTimeout:      30,
		ServerIdleTimeout:       120,
		DatabaseURL:             "postgres://u:p@localhost:5432/app?sslmode=disable",
		RedisURL:                "redis://localhost:6379/0",
		JWTSecret:               "some-secret",
		RateLimitPerMinute:      60,
		CORSAllowOrigins:        []string{"https://example.com"},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error when read_timeout < read_header_timeout")
	}
}

func TestGetStringSlice_SplitsSingleCommaSeparatedValue(t *testing.T) {
	v := newTestViper(t, map[string]any{"cors": map[string]any{"allow_origins": "https://a.com, https://b.com ,https://c.com"}})

	got := getStringSlice(v, "cors.allow_origins")
	want := []string{"https://a.com", "https://b.com", "https://c.com"}
	if !equalStringSlices(got, want) {
		t.Errorf("getStringSlice() = %v, want %v", got, want)
	}
}

func TestGetStringSlice_PassesThroughNativeList(t *testing.T) {
	v := newTestViper(t, map[string]any{"cors": map[string]any{"allow_origins": []any{"https://a.com", "https://b.com"}}})

	got := getStringSlice(v, "cors.allow_origins")
	want := []string{"https://a.com", "https://b.com"}
	if !equalStringSlices(got, want) {
		t.Errorf("getStringSlice() = %v, want %v", got, want)
	}
}

func TestGetStringSlice_MissingKeyReturnsEmpty(t *testing.T) {
	v := newTestViper(t, map[string]any{})

	got := getStringSlice(v, "cors.allow_origins")
	if len(got) != 0 {
		t.Errorf("getStringSlice() = %v, want empty", got)
	}
}

func TestGetStringSlice_IgnoresEmptySegments(t *testing.T) {
	v := newTestViper(t, map[string]any{"oauth": map[string]any{"providers": "wechat,,apple,"}})

	got := getStringSlice(v, "oauth.providers")
	want := []string{"wechat", "apple"}
	if !equalStringSlices(got, want) {
		t.Errorf("getStringSlice() = %v, want %v", got, want)
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
