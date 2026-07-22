package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"backend/pkg/strutil"

	"github.com/spf13/viper"
	"github.com/subosito/gotenv"
)

// Config 汇聚所有运行配置（扁平结构，便于全项目直接访问字段）。
//
// 加载优先级（高 → 低）：
//  1. 系统环境变量
//  2. config/local.yaml     （本地私有，gitignore）
//  3. config/{env}.yaml     （环境专用，如 development.yaml）
//  4. config/base.yaml      （全局默认值，提交到 git）
type Config struct {
	// ── 服务 ─────────────────────────────────────────────────────────────
	AppEnv     string
	ServerAddr string
	// HTTP Server 超时（防 Slowloris、慢客户端与连接泄漏）
	ServerReadHeaderTimeout time.Duration
	ServerReadTimeout       time.Duration
	ServerWriteTimeout      time.Duration
	ServerIdleTimeout       time.Duration

	// ── PostgreSQL ────────────────────────────────────────────────────────
	DatabaseURL   string
	DBHost        string
	DBPort        string
	DBUser        string
	DBPassword    string
	DBName        string
	DBSSLMode     string
	DBMaxConns    int
	DBMinConns    int
	DBMaxConnIdle time.Duration
	DBMaxConnLife time.Duration

	// ── Redis ─────────────────────────────────────────────────────────────
	RedisURL       string
	RedisHost      string
	RedisPort      string
	RedisPassword  string
	RedisDB        int
	RedisPoolSize  int
	RedisKeyPrefix string

	// ── JWT ───────────────────────────────────────────────────────────────
	JWTSecret          string
	JWTAccessTokenTTL  time.Duration
	JWTRefreshTokenTTL time.Duration

	// ── Auth ──────────────────────────────────────────────────────────────
	AuthMockCodeEnabled    bool
	AuthMultiDeviceEnabled bool // true=多设备同时在线；false=单设备（新登录踢掉旧会话）
	AuthRedisFailOpen      bool // Redis故障时鉴权策略：true=fail-open（放行，可用性优先）；false=fail-closed（拒绝，安全性优先）

	// ── 验证码下发（SMTP 邮件，可选）────────────────────────────────────────
	// SMTPHost 为空时不启用 SMTP Provider；短信服务商按需在 app 层接入。
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string

	// ── OAuth ─────────────────────────────────────────────────────────────
	OAuthDevMode         bool
	OAuthProviders       []string
	OAuthWechatAppID     string
	OAuthWechatAppSecret string
	OAuthAppleClientID   string
	OAuthGoogleClientID  string

	// ── 日志 ──────────────────────────────────────────────────────────────
	LogLevel  string
	LogFormat string

	// ── 限流 ──────────────────────────────────────────────────────────────
	RateLimitPerMinute     int // 未登录/公开路由：按 IP
	RateLimitUserPerMinute int // 已鉴权路由：按 user_id

	// ── 可观测 ────────────────────────────────────────────────────────────
	MetricsEnabled bool
	MetricsToken   string // /metrics 端点的 bearer token，生产环境必须配置

	// ── CORS ──────────────────────────────────────────────────────────────
	CORSAllowOrigins     []string
	CORSAllowCredentials bool
}

// Load 按 base → {env} → local → 环境变量 的顺序加载并合并配置。
func Load() *Config {
	v := loadViper()

	cfg := &Config{
		AppEnv:                  v.GetString("app.env"),
		ServerAddr:              v.GetString("app.server_addr"),
		ServerReadHeaderTimeout: v.GetDuration("app.read_header_timeout"),
		ServerReadTimeout:       v.GetDuration("app.read_timeout"),
		ServerWriteTimeout:      v.GetDuration("app.write_timeout"),
		ServerIdleTimeout:       v.GetDuration("app.idle_timeout"),

		DBHost:        v.GetString("database.host"),
		DBPort:        v.GetString("database.port"),
		DBUser:        v.GetString("database.user"),
		DBPassword:    v.GetString("database.password"),
		DBName:        v.GetString("database.name"),
		DBSSLMode:     v.GetString("database.ssl_mode"),
		DBMaxConns:    v.GetInt("database.pool.max_conns"),
		DBMinConns:    v.GetInt("database.pool.min_conns"),
		DBMaxConnIdle: v.GetDuration("database.pool.max_conn_idle"),
		DBMaxConnLife: v.GetDuration("database.pool.max_conn_life"),

		RedisHost:      v.GetString("redis.host"),
		RedisPort:      v.GetString("redis.port"),
		RedisPassword:  v.GetString("redis.password"),
		RedisDB:        v.GetInt("redis.db"),
		RedisPoolSize:  v.GetInt("redis.pool_size"),
		RedisKeyPrefix: v.GetString("redis.key_prefix"),

		JWTSecret:          v.GetString("jwt.secret"),
		JWTAccessTokenTTL:  v.GetDuration("jwt.access_ttl"),
		JWTRefreshTokenTTL: v.GetDuration("jwt.refresh_ttl"),

		AuthMockCodeEnabled:    v.GetBool("auth.mock_code_enabled"),
		AuthMultiDeviceEnabled: v.GetBool("auth.multi_device_enabled"),
		AuthRedisFailOpen:      v.GetBool("auth.redis_fail_open"),

		SMTPHost:     v.GetString("smtp.host"),
		SMTPPort:     v.GetInt("smtp.port"),
		SMTPUsername: v.GetString("smtp.username"),
		SMTPPassword: v.GetString("smtp.password"),
		SMTPFrom:     v.GetString("smtp.from"),

		OAuthDevMode:         v.GetBool("oauth.dev_mode"),
		OAuthProviders:       getStringSlice(v, "oauth.providers"),
		OAuthWechatAppID:     v.GetString("oauth.wechat.app_id"),
		OAuthWechatAppSecret: v.GetString("oauth.wechat.app_secret"),
		OAuthAppleClientID:   v.GetString("oauth.apple.client_id"),
		OAuthGoogleClientID:  v.GetString("oauth.google.client_id"),

		LogLevel:  v.GetString("log.level"),
		LogFormat: v.GetString("log.format"),

		RateLimitPerMinute:     v.GetInt("rate_limit.per_minute"),
		RateLimitUserPerMinute: v.GetInt("rate_limit.user_per_minute"),

		MetricsEnabled: v.GetBool("metrics.enabled"),
		MetricsToken:   v.GetString("metrics.token"),

		CORSAllowOrigins:     getStringSlice(v, "cors.allow_origins"),
		CORSAllowCredentials: v.GetBool("cors.allow_credentials"),
	}

	// DATABASE_URL 和 REDIS_URL 优先（为空时从分项拼接）
	cfg.DatabaseURL = strutil.FirstNonEmpty(v.GetString("database.url"), cfg.buildDatabaseURL())
	cfg.RedisURL = strutil.FirstNonEmpty(v.GetString("redis.url"), cfg.buildRedisURL())

	if cfg.RateLimitUserPerMinute <= 0 {
		cfg.RateLimitUserPerMinute = cfg.RateLimitPerMinute
	}

	return cfg
}

// Validate 检查启动所需的关键配置，避免生产环境带着不安全默认值运行。
func (c *Config) Validate() error {
	if c.ServerAddr == "" {
		return errors.New("app.server_addr 不能为空")
	}
	if c.ServerReadHeaderTimeout <= 0 {
		return errors.New("app.read_header_timeout 必须大于 0")
	}
	if c.ServerReadTimeout <= 0 {
		return errors.New("app.read_timeout 必须大于 0")
	}
	if c.ServerWriteTimeout <= 0 {
		return errors.New("app.write_timeout 必须大于 0")
	}
	if c.ServerIdleTimeout <= 0 {
		return errors.New("app.idle_timeout 必须大于 0")
	}
	if c.ServerReadTimeout < c.ServerReadHeaderTimeout {
		return errors.New("app.read_timeout 不能小于 app.read_header_timeout")
	}
	if c.DatabaseURL == "" {
		return errors.New("database.url 不能为空")
	}
	if c.RedisURL == "" {
		return errors.New("redis.url 不能为空")
	}
	if c.JWTSecret == "" {
		return errors.New("jwt.secret 不能为空")
	}
	if c.IsProd() {
		if c.JWTSecret == "dev-secret-change-in-production" {
			return errors.New("production 环境必须设置安全的 JWT_SECRET")
		}
		if len(c.JWTSecret) < 32 {
			return errors.New("production 环境的 jwt.secret 长度不能少于 32 字节（HS256 安全要求）")
		}
		if c.AuthMockCodeEnabled {
			return errors.New("production 环境不能启用 auth.mock_code_enabled")
		}
		if c.OAuthDevMode {
			return errors.New("production 环境不能启用 oauth.dev_mode")
		}
		for _, p := range c.OAuthProviders {
			switch strings.ToLower(strings.TrimSpace(p)) {
			case "google":
				if c.OAuthGoogleClientID == "" {
					return errors.New("production 环境启用 google 登录时必须配置 oauth.google.client_id")
				}
			case "apple":
				if c.OAuthAppleClientID == "" {
					return errors.New("production 环境启用 apple 登录时必须配置 oauth.apple.client_id")
				}
			case "wechat":
				if c.OAuthWechatAppID == "" {
					return errors.New("production 环境启用 wechat 登录时必须配置 oauth.wechat.app_id")
				}
			}
		}
		if c.MetricsEnabled && c.MetricsToken == "" {
			return errors.New("production 环境启用 metrics 时必须配置 metrics.token（bearer token）")
		}
	}
	if c.RateLimitPerMinute <= 0 {
		return errors.New("rate_limit.per_minute 必须大于 0")
	}
	if len(c.CORSAllowOrigins) == 0 {
		return errors.New("cors.allow_origins 至少需要配置一个来源")
	}
	return nil
}

// IsDev 是否为开发环境。
func (c *Config) IsDev() bool {
	return c.AppEnv == "development" || c.AppEnv == ""
}

// IsProd 是否为生产环境。
func (c *Config) IsProd() bool {
	return c.AppEnv == "production"
}

// ── 内部 ─────────────────────────────────────────────────────────────────────

func loadViper() *viper.Viper {
	loadDotEnv()

	v := viper.New()
	v.SetConfigType("yaml")

	// 配置目录：优先使用 CONFIG_PATH 环境变量，默认 config/
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config"
	}

	// 第一层：读取 base.yaml（全局默认值）
	v.SetConfigName("base")
	v.AddConfigPath(configPath)
	if err := v.ReadInConfig(); err != nil {
		// base.yaml 不存在时不中断，继续用空配置
		_ = err
	}

	// 第二层：合并 {env}.yaml（环境专用配置）
	// APP_ENV 此时从环境变量获取（因 viper 还没完成加载）
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = v.GetString("app.env")
	}
	if env == "" {
		env = "development"
	}
	mergeConfig(v, configPath, env)

	// 第三层：合并 local.yaml（本地私有，不提交到 git）
	mergeConfig(v, configPath, "local")

	// 第四层：环境变量覆盖（点路径 → 下划线大写，如 database.password → DATABASE_PASSWORD）
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()

	// 显式绑定常用的环境变量（保持对原有 .env 变量名的兼容）
	bindEnvs(v)

	return v
}

// mergeConfig 合并指定名称的 yaml 文件（文件不存在时静默跳过）。
func mergeConfig(base *viper.Viper, path, name string) {
	v := viper.New()
	v.SetConfigType("yaml")
	v.SetConfigName(name)
	v.AddConfigPath(path)

	if err := v.ReadInConfig(); err == nil {
		_ = base.MergeConfigMap(v.AllSettings())
	}
}

// bindEnvs 显式绑定关键环境变量，保持与原有 .env 变量名兼容。
//
//nolint:gosec // G101 误报：这是「配置项 -> 环境变量名」的映射表，值均为变量名，不含真实密钥。
func bindEnvs(v *viper.Viper) {
	bindings := map[string]string{
		"app.env":                     "APP_ENV",
		"app.server_addr":             "SERVER_ADDR",
		"app.read_header_timeout":     "SERVER_READ_HEADER_TIMEOUT",
		"app.read_timeout":            "SERVER_READ_TIMEOUT",
		"app.write_timeout":           "SERVER_WRITE_TIMEOUT",
		"app.idle_timeout":            "SERVER_IDLE_TIMEOUT",
		"database.url":                "DATABASE_URL",
		"database.host":               "DB_HOST",
		"database.port":               "DB_PORT",
		"database.user":               "DB_USER",
		"database.password":           "DB_PASSWORD",
		"database.name":               "DB_NAME",
		"database.ssl_mode":           "DB_SSL_MODE",
		"database.pool.max_conns":     "DB_MAX_CONNS",
		"database.pool.min_conns":     "DB_MIN_CONNS",
		"database.pool.max_conn_idle": "DB_MAX_CONN_IDLE",
		"database.pool.max_conn_life": "DB_MAX_CONN_LIFE",
		"redis.url":                   "REDIS_URL",
		"redis.host":                  "REDIS_HOST",
		"redis.port":                  "REDIS_PORT",
		"redis.password":              "REDIS_PASSWORD",
		"redis.db":                    "REDIS_DB",
		"redis.pool_size":             "REDIS_POOL_SIZE",
		"redis.key_prefix":            "REDIS_KEY_PREFIX",
		"jwt.secret":                  "JWT_SECRET",
		"jwt.access_ttl":              "JWT_ACCESS_TTL",
		"jwt.refresh_ttl":             "JWT_REFRESH_TTL",
		"auth.mock_code_enabled":      "AUTH_MOCK_CODE_ENABLED",
		"auth.multi_device_enabled":   "AUTH_MULTI_DEVICE_ENABLED",
		"auth.redis_fail_open":        "AUTH_REDIS_FAIL_OPEN",
		"smtp.host":                   "SMTP_HOST",
		"smtp.port":                   "SMTP_PORT",
		"smtp.username":               "SMTP_USERNAME",
		"smtp.password":               "SMTP_PASSWORD",
		"smtp.from":                   "SMTP_FROM",
		"oauth.dev_mode":              "OAUTH_DEV_MODE",
		"oauth.providers":             "OAUTH_PROVIDERS",
		"oauth.wechat.app_id":         "OAUTH_WECHAT_APP_ID",
		"oauth.wechat.app_secret":     "OAUTH_WECHAT_APP_SECRET",
		"oauth.apple.client_id":       "OAUTH_APPLE_CLIENT_ID",
		"oauth.google.client_id":      "OAUTH_GOOGLE_CLIENT_ID",
		"log.level":                   "LOG_LEVEL",
		"log.format":                  "LOG_FORMAT",
		"rate_limit.per_minute":       "RATE_LIMIT_PER_MINUTE",
		"rate_limit.user_per_minute":  "RATE_LIMIT_USER_PER_MINUTE",
		"metrics.enabled":             "METRICS_ENABLED",
		"metrics.token":               "METRICS_TOKEN",
		"cors.allow_origins":          "CORS_ALLOW_ORIGINS",
		"cors.allow_credentials":      "CORS_ALLOW_CREDENTIALS",
	}

	for key, envVar := range bindings {
		_ = v.BindEnv(key, envVar)
	}
}

func loadDotEnv() {
	// gotenv.Load 不覆盖已经存在的系统环境变量，适合本地开发。
	_ = gotenv.Load(".env", "backend/.env", "../.env")
}

func (c *Config) buildDatabaseURL() string {
	u := &url.URL{
		Scheme:   "postgres",
		Host:     fmt.Sprintf("%s:%s", c.DBHost, c.DBPort),
		Path:     c.DBName,
		RawQuery: "sslmode=" + url.QueryEscape(c.DBSSLMode),
	}
	if c.DBPassword != "" {
		u.User = url.UserPassword(c.DBUser, c.DBPassword)
	} else {
		u.User = url.User(c.DBUser)
	}
	return u.String()
}

func (c *Config) buildRedisURL() string {
	u := &url.URL{
		Scheme: "redis",
		Host:   fmt.Sprintf("%s:%s", c.RedisHost, c.RedisPort),
		Path:   fmt.Sprintf("%d", c.RedisDB),
	}
	if c.RedisPassword != "" {
		u.User = url.UserPassword("", c.RedisPassword)
	}
	return u.String()
}

// getStringSlice 从配置读取字符串列表。
//
// viper 对形如 "a, b ,c" 的字符串值会先按空白拆分（而不是按逗号），
// 可能产生 ["a,", "b", ",c"] 这种带残留逗号的中间结果；这里统一重新拼接、
// 按逗号切分并去除首尾空格，使 "a,b,c"、"a, b, c"、原生 yaml 列表都能得到
// 一致的解析结果。
func getStringSlice(v *viper.Viper, key string) []string {
	raw := v.GetStringSlice(key)
	joined := strings.Join(raw, ",")
	if joined == "" {
		return nil
	}

	parts := strings.Split(joined, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}
