package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"backend/pkg/strutil"

	"github.com/go-sql-driver/mysql"
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

	// ServerRequestTimeout 是单个请求的 context 超时（DB/Redis 调用共享该 ctx）。
	// 必须小于 write_timeout：否则超时后已经没有时间把 503 信封写回客户端。
	ServerRequestTimeout time.Duration
	// ServerSendCodeRequestTimeout 是 /api/v1/auth/send-code 专用的请求预算，0 = 沿用
	// ServerRequestTimeout。发码是同步外呼（SMTP/短信），默认预算装不下 SMTP 的建连+全流程
	// 超时，慢 SMTP 下每次请求都会以 503 收场，因此这条路由单独放宽。
	// 不变量链由 Validate 强制：smtp.dial_timeout + smtp.send_timeout < 预算 ≤ write_timeout。
	ServerSendCodeRequestTimeout time.Duration
	// ServerTrustedProxies 反向代理/LB 的来源 IP 或 CIDR 列表（如 "10.0.0.0/8"）。
	// Gin 默认信任所有代理，这会让客户端能自行伪造 X-Forwarded-For；这里显式收紧：
	// 列表为空（默认值）时不信任任何代理，XFF 一律忽略，限流与访问日志只认 TCP 对端地址。
	// 部署在 nginx/SLB 之后时必须填入代理网段，否则所有用户会共享同一个 IP 限流桶。
	ServerTrustedProxies []string

	// ── Database ──────────────────────────────────────────────────────────
	DBDriver      string
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

	// DBStatementTimeout PostgreSQL statement_timeout（每个连接建立时下发）。
	// 防止慢查询长期占用连接池；0 表示不设置（PostgreSQL 默认无限制）。
	DBStatementTimeout time.Duration

	// ── Redis ─────────────────────────────────────────────────────────────
	RedisURL       string
	RedisHost      string
	RedisPort      string
	RedisPassword  string
	RedisDB        int
	RedisPoolSize  int
	RedisKeyPrefix string

	// RedisReadTimeout / RedisWriteTimeout 显式限制单次 Redis 往返；0 沿用 go-redis 默认（3s）。
	RedisReadTimeout  time.Duration
	RedisWriteTimeout time.Duration

	// ── JWT ───────────────────────────────────────────────────────────────
	JWTSecret          string
	JWTAccessTokenTTL  time.Duration
	JWTRefreshTokenTTL time.Duration

	// ── Auth ──────────────────────────────────────────────────────────────
	AuthMockCodeEnabled    bool
	AuthMultiDeviceEnabled bool // true=多设备同时在线；false=单设备（新登录踢掉旧会话）
	AuthRedisFailOpen      bool // Redis故障时鉴权策略：true=fail-open（放行，可用性优先）；false=fail-closed（拒绝，安全性优先）
	// AuthRequireCodeForPasswordRegister 密码注册是否必须先通过一次性验证码（证明持有手机号/邮箱）。
	// 默认 true；设为 false 会重新引入「抢注他人号码」和「用 /register 枚举已注册账号」两个问题，
	// 因此 production 环境启动校验会拒绝 false。
	AuthRequireCodeForPasswordRegister bool
	// AuthCodeChannels 允许下发验证码的渠道白名单（sms/email）。
	// 空 = 自动按已配置的 Provider 推导：未接入短信时自动禁用 sms，避免「启动成功但短信全 500」。
	// 显式配置时每个渠道都必须有 Provider，否则启动失败。
	AuthCodeChannels []string
	// AuthCodeDailyLimitPerTarget / AuthCodeDailyLimitPerIP 是验证码下发的滚动 24 小时上限。
	// <=0 表示关闭该维度（便于本地/测试）；production 启动校验要求两者都 > 0。
	AuthCodeDailyLimitPerTarget int
	AuthCodeDailyLimitPerIP     int

	// ── 验证码下发（SMTP 邮件，可选）────────────────────────────────────────
	// SMTPHost 为空时不启用 SMTP Provider；短信服务商按需在 app 层接入。
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string
	// SMTPDialTimeout / SMTPSendTimeout 是 SMTP 建连与全流程超时。
	// 外呼预算（ServerSendCodeRequestTimeout）必须容得下两者之和，否则请求 ctx 会先于
	// SMTP 自身超时到期，把连接 deadline 拉到当下、读写立刻失败（启动阶段即校验）。
	SMTPDialTimeout time.Duration
	SMTPSendTimeout time.Duration

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
	RateLimitAuthPerMinute int // 登录/注册：按 IP，独立于公共桶（bcrypt 成本高，需更严）

	// ── 可观测 ────────────────────────────────────────────────────────────
	MetricsEnabled bool
	MetricsToken   string // /metrics 端点的 bearer token，生产环境必须配置

	// ── CORS ──────────────────────────────────────────────────────────────
	CORSAllowOrigins     []string
	CORSAllowCredentials bool
}

// Load 按 base → {env} → local → 环境变量 的顺序加载并合并配置。
//
// 只有「配置文件不存在」会被忽略（允许完全依赖环境变量）；YAML 语法错误、
// 读取失败或合并失败都会返回 error，避免带着零值配置静默启动。
func Load() (*Config, error) {
	v, err := loadViper()
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		AppEnv:                       v.GetString("app.env"),
		ServerAddr:                   v.GetString("app.server_addr"),
		ServerReadHeaderTimeout:      v.GetDuration("app.read_header_timeout"),
		ServerReadTimeout:            v.GetDuration("app.read_timeout"),
		ServerWriteTimeout:           v.GetDuration("app.write_timeout"),
		ServerIdleTimeout:            v.GetDuration("app.idle_timeout"),
		ServerRequestTimeout:         v.GetDuration("app.request_timeout"),
		ServerSendCodeRequestTimeout: v.GetDuration("app.send_code_request_timeout"),
		ServerTrustedProxies:         getStringSlice(v, "app.trusted_proxies"),

		DBDriver:      strings.ToLower(v.GetString("database.driver")),
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

		DBStatementTimeout: v.GetDuration("database.statement_timeout"),

		RedisHost:      v.GetString("redis.host"),
		RedisPort:      v.GetString("redis.port"),
		RedisPassword:  v.GetString("redis.password"),
		RedisDB:        v.GetInt("redis.db"),
		RedisPoolSize:  v.GetInt("redis.pool_size"),
		RedisKeyPrefix: v.GetString("redis.key_prefix"),

		RedisReadTimeout:  v.GetDuration("redis.read_timeout"),
		RedisWriteTimeout: v.GetDuration("redis.write_timeout"),

		JWTSecret:          v.GetString("jwt.secret"),
		JWTAccessTokenTTL:  v.GetDuration("jwt.access_ttl"),
		JWTRefreshTokenTTL: v.GetDuration("jwt.refresh_ttl"),

		AuthMockCodeEnabled:    v.GetBool("auth.mock_code_enabled"),
		AuthMultiDeviceEnabled: v.GetBool("auth.multi_device_enabled"),
		AuthRedisFailOpen:      v.GetBool("auth.redis_fail_open"),
		// 安全默认值：老配置里没有这个 key 时必须保持「要求验证码」，不能因为缺省而静默降级
		AuthRequireCodeForPasswordRegister: v.GetBool("auth.require_code_for_password_register"),
		AuthCodeChannels:                   getStringSlice(v, "auth.code_channels"),
		AuthCodeDailyLimitPerTarget:        v.GetInt("auth.code_daily_limit_per_target"),
		AuthCodeDailyLimitPerIP:            v.GetInt("auth.code_daily_limit_per_ip"),

		SMTPHost:        v.GetString("smtp.host"),
		SMTPPort:        v.GetInt("smtp.port"),
		SMTPUsername:    v.GetString("smtp.username"),
		SMTPPassword:    v.GetString("smtp.password"),
		SMTPFrom:        v.GetString("smtp.from"),
		SMTPDialTimeout: v.GetDuration("smtp.dial_timeout"),
		SMTPSendTimeout: v.GetDuration("smtp.send_timeout"),

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
		RateLimitAuthPerMinute: v.GetInt("rate_limit.auth_per_minute"),

		MetricsEnabled: v.GetBool("metrics.enabled"),
		MetricsToken:   v.GetString("metrics.token"),

		CORSAllowOrigins:     getStringSlice(v, "cors.allow_origins"),
		CORSAllowCredentials: v.GetBool("cors.allow_credentials"),
	}

	// DATABASE_URL 和 REDIS_URL 优先（为空时从分项拼接）
	if cfg.DBDriver == "" {
		cfg.DBDriver = "postgres"
	}
	cfg.DatabaseURL = strutil.FirstNonEmpty(v.GetString("database.url"), cfg.buildDatabaseURL())
	cfg.RedisURL = strutil.FirstNonEmpty(v.GetString("redis.url"), cfg.buildRedisURL())

	// MySQL DSN 归一化：database.url / DATABASE_URL 会旁路 buildDatabaseURL，
	// 此处兜底强制 parseTime/loc/time_zone，保证 DATETIME 读写与 PG 的 UTC 语义一致。
	if cfg.DBDriver == "mysql" {
		if normalized, err := NormalizeMySQLDSN(cfg.DatabaseURL); err == nil {
			cfg.DatabaseURL = normalized
		}
	}

	if cfg.RateLimitUserPerMinute <= 0 {
		cfg.RateLimitUserPerMinute = cfg.RateLimitPerMinute
	}

	return cfg, nil
}

// AuthRateLimitPerMinuteOrDefault 返回登录/注册专用限流额度。
//
// 未配置（<=0）时取 20：bcrypt cost 12 下一次校验约 200–400ms，20/min/IP 足够真人
// 使用（含办公 NAT），同时把单 IP 每分钟能触发的 CPU 放大压到可接受范围。
// 登录与注册共用同一个桶，避免在两个端点之间交替请求把额度翻倍。
func (c *Config) AuthRateLimitPerMinuteOrDefault() int {
	if c.RateLimitAuthPerMinute > 0 {
		return c.RateLimitAuthPerMinute
	}
	return 20
}

// Validate 检查启动所需的关键配置，避免生产环境带着不安全默认值运行。
func (c *Config) Validate() error {
	driver := c.DBDriver
	if driver == "" {
		driver = "postgres"
	}
	if driver != "postgres" && driver != "mysql" {
		return fmt.Errorf("database.driver 不支持 %q（可选 postgres、mysql）", driver)
	}
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
	if c.ServerRequestTimeout <= 0 {
		return errors.New("app.request_timeout 必须大于 0")
	}
	if c.ServerRequestTimeout > c.ServerWriteTimeout {
		return errors.New("app.request_timeout 不能大于 app.write_timeout（超时后需要留出写回 503 的时间）")
	}
	// 发码预算：0 表示不单独放宽（沿用 request_timeout），非 0 时才受下面两条约束。
	if c.ServerSendCodeRequestTimeout > 0 {
		if c.ServerSendCodeRequestTimeout < c.ServerRequestTimeout {
			// 这条预算存在的意义是给外呼放宽时间；小于 request_timeout 等于把它写成收紧，
			// 配置写反时宁可启动失败，也不要看着配了、实际仍是全局值封顶。
			return errors.New("app.send_code_request_timeout 不能小于 app.request_timeout（要整体收紧请改 request_timeout）")
		}
		if c.ServerSendCodeRequestTimeout > c.ServerWriteTimeout {
			return errors.New("app.send_code_request_timeout 不能大于 app.write_timeout（超时后需要留出写回 503 的时间）")
		}
	}
	if c.SMTPHost != "" {
		if c.SMTPDialTimeout <= 0 {
			return errors.New("smtp.dial_timeout 必须大于 0")
		}
		if c.SMTPSendTimeout <= 0 {
			return errors.New("smtp.send_timeout 必须大于 0")
		}
		// 「外层小于内层」是慢 SMTP 下发码恒 503 的根因：请求 ctx 先到期会把 SMTP 连接
		// deadline 拉到当下、读写立刻失败。这类错配必须在启动时炸，而不是等线上 503。
		// 取严格小于：预算等于 dial+send 意味着 Redis/DB 与响应写回一点时间都不剩。
		if budget := c.SendCodeRequestBudget(); c.SMTPDialTimeout+c.SMTPSendTimeout >= budget {
			return fmt.Errorf(
				"smtp.dial_timeout(%s)+smtp.send_timeout(%s) 必须小于发码请求预算 %s："+
					"请放宽 app.send_code_request_timeout（并保持在 app.write_timeout 之内）或收紧 SMTP 超时",
				c.SMTPDialTimeout, c.SMTPSendTimeout, budget)
		}
	}
	if c.ServerReadTimeout < c.ServerReadHeaderTimeout {
		return errors.New("app.read_timeout 不能小于 app.read_header_timeout")
	}
	if err := validateTrustedProxies(c.ServerTrustedProxies); err != nil {
		return err
	}
	for _, ch := range c.AuthCodeChannels {
		switch strings.ToLower(strings.TrimSpace(ch)) {
		case "sms", "email":
		default:
			return fmt.Errorf("auth.code_channels 包含不支持的渠道 %q（可选 sms、email）", ch)
		}
	}
	if c.DatabaseURL == "" {
		return errors.New("database.url 不能为空")
	}
	if c.RedisURL == "" {
		return errors.New("redis.url 不能为空")
	}
	// key_prefix 参与拼接所有 Redis 键（{prefix}:session:{sid}、{prefix}:revoke:uid:{uid} 等），
	// 含冒号或空白会破坏键的层级，导致中间件读写的键与运维排查时看到的不一致。
	if strings.TrimSpace(c.RedisKeyPrefix) == "" {
		return errors.New("redis.key_prefix 不能为空")
	}
	if strings.ContainsAny(c.RedisKeyPrefix, ": \t\n") {
		return errors.New("redis.key_prefix 不能包含冒号或空白字符")
	}
	// 显式要求超时为正数：0 会悄悄退回 go-redis 默认值，配置写错时无从发现。
	if c.RedisReadTimeout <= 0 {
		return errors.New("redis.read_timeout 必须大于 0")
	}
	if c.RedisWriteTimeout <= 0 {
		return errors.New("redis.write_timeout 必须大于 0")
	}
	if c.JWTSecret == "" {
		return errors.New("jwt.secret 不能为空")
	}
	// Access Token 必须明显短于 Refresh Token：权限回收（改密/降权/禁用）依赖 access 自然过期兜底，
	// 两者的有效期若倒挂或相等，refresh 轮换就失去意义。
	if c.JWTAccessTokenTTL <= 0 {
		return errors.New("jwt.access_ttl 必须大于 0")
	}
	if c.JWTRefreshTokenTTL <= 0 {
		return errors.New("jwt.refresh_ttl 必须大于 0")
	}
	if c.JWTAccessTokenTTL >= c.JWTRefreshTokenTTL {
		return errors.New("jwt.access_ttl 必须小于 jwt.refresh_ttl")
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
		if !c.AuthRequireCodeForPasswordRegister {
			return errors.New("production 环境必须要求密码注册通过验证码（auth.require_code_for_password_register=true）")
		}
		// 滚动 24 小时的验证码下发配额是成本保护：生产环境不允许关闭，
		// 否则单 IP 每分钟可对 60 个不同号码发码，短信账单不可控。
		if c.AuthCodeDailyLimitPerTarget <= 0 {
			return errors.New("production 环境必须设置 auth.code_daily_limit_per_target > 0（建议 10）")
		}
		if c.AuthCodeDailyLimitPerIP <= 0 {
			return errors.New("production 环境必须设置 auth.code_daily_limit_per_ip > 0（建议 100）")
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
		// DATABASE_URL 优先于分项配置，database.ssl_mode 对完整 DSN 不生效，
		// 必须要求 DSN 显式携带安全的 sslmode，防止 TLS 被静默绕过
		// （sslmode 缺省时 pgx 按 prefer 处理，服务端不支持 TLS 会静默回落明文连接）。
		if driver == "postgres" {
			switch databaseSSLMode(c.DatabaseURL) {
			case "require", "verify-ca", "verify-full":
			default:
				return errors.New("production 环境的 PostgreSQL database.url 必须显式携带 sslmode=require/verify-ca/verify-full")
			}
		} else if !strings.Contains(c.DatabaseURL, "tls=true") {
			return errors.New("production 环境的 MySQL database.url 必须显式携带 tls=true")
		}
	}
	if c.RateLimitPerMinute <= 0 {
		return errors.New("rate_limit.per_minute 必须大于 0")
	}
	if len(c.CORSAllowOrigins) == 0 {
		return errors.New("cors.allow_origins 至少需要配置一个来源")
	}
	// 浏览器规范不允许 Access-Control-Allow-Origin: * 与 credentials 同时使用；
	// 放行会在运行期被浏览器拒绝，且等价于把带 Cookie 的接口暴露给任意站点。
	if c.CORSAllowCredentials {
		for _, origin := range c.CORSAllowOrigins {
			if strings.TrimSpace(origin) == "*" {
				return errors.New("cors.allow_credentials=true 时 cors.allow_origins 不能包含 *")
			}
		}
	}
	return nil
}

// IsDev 是否为开发环境。
func (c *Config) IsDev() bool {
	return c.AppEnv == "development" || c.AppEnv == ""
}

// databaseSSLMode 从 DSN 中提取 sslmode，兼容 URL（postgres://...?sslmode=x）
// 与 key=value（host=... sslmode=x）两种格式；未携带或解析失败返回空串。
func databaseSSLMode(dsn string) string {
	if strings.Contains(dsn, "://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return ""
		}
		return u.Query().Get("sslmode")
	}
	for _, kv := range strings.Fields(dsn) {
		if v, ok := strings.CutPrefix(kv, "sslmode="); ok {
			return strings.Trim(v, "'")
		}
	}
	return ""
}

// validateTrustedProxies 校验 app.trusted_proxies 每一项都是合法 IP 或 CIDR。
// Gin 会解析同一份列表，非法值在启动阶段就报错比运行期静默降级更好排查。
func validateTrustedProxies(proxies []string) error {
	for _, p := range proxies {
		if strings.Contains(p, "/") {
			if _, _, err := net.ParseCIDR(p); err != nil {
				return fmt.Errorf("app.trusted_proxies 中的 %q 不是合法 CIDR：%w", p, err)
			}
			continue
		}
		if net.ParseIP(p) == nil {
			return fmt.Errorf("app.trusted_proxies 中的 %q 既不是 IP 也不是 CIDR", p)
		}
	}
	return nil
}

// IsProd 是否为生产环境。
func (c *Config) IsProd() bool {
	return c.AppEnv == "production"
}

// SendCodeRequestBudget 返回发码端点实际使用的请求预算：
// 未单独配置时沿用全局 request_timeout。中间件与启动校验共用这一处判断，
// 避免两边各写一遍取值规则、日后放宽忘了同步校验。
func (c *Config) SendCodeRequestBudget() time.Duration {
	if c.ServerSendCodeRequestTimeout > 0 {
		return c.ServerSendCodeRequestTimeout
	}
	return c.ServerRequestTimeout
}

// ── 内部 ─────────────────────────────────────────────────────────────────────

func loadViper() (*viper.Viper, error) {
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
		if !isConfigNotFound(err) {
			return nil, fmt.Errorf("读取 %s: %w", filepath.Join(configPath, "base.yaml"), err)
		}
		// base.yaml 不存在时继续：所有配置仍可由环境变量提供
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
	if err := mergeConfig(v, configPath, env); err != nil {
		return nil, err
	}

	// 第三层：合并 local.yaml（本地私有，不提交到 git）
	if err := mergeConfig(v, configPath, "local"); err != nil {
		return nil, err
	}

	// 第四层：环境变量覆盖（点路径 → 下划线大写，如 database.password → DATABASE_PASSWORD）
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()

	// 显式绑定常用的环境变量（保持对原有 .env 变量名的兼容）
	bindEnvs(v)

	// 安全相关 bool 的缺省值必须显式声明：viper 对缺失的 key 返回 false，
	// 而这里的 false 恰好代表「更不安全」。老部署目录里没有该 key 时不能静默降级。
	v.SetDefault("auth.require_code_for_password_register", true)

	// 验证码下发配额：默认 10 次/目标、100 次/IP（滚动 24 小时）。
	// 老配置目录没有这些 key 时也必须生效，否则升级后配额形同虚设。
	v.SetDefault("auth.code_daily_limit_per_target", 10)
	v.SetDefault("auth.code_daily_limit_per_ip", 100)

	// SMTP 超时同理：老配置目录里没有这两个 key 时，缺省值必须与代码内默认一致，
	// 否则升级后要么预算缺失、要么 SMTP 拿到 0 超时立刻打断读写。
	// app.send_code_request_timeout 刻意不给缺省：留空就是「不放宽」，由 base.yaml 提供 20s，
	// 避免把 write_timeout 配得比 20s 小的部署在升级瞬间卡在启动校验上。
	v.SetDefault("smtp.dial_timeout", "5s")
	v.SetDefault("smtp.send_timeout", "10s")

	return v, nil
}

// mergeConfig 合并指定名称的 yaml 文件；文件不存在时跳过，其余错误向上返回。
func mergeConfig(base *viper.Viper, path, name string) error {
	v := viper.New()
	v.SetConfigType("yaml")
	v.SetConfigName(name)
	v.AddConfigPath(path)

	file := filepath.Join(path, name+".yaml")
	if err := v.ReadInConfig(); err != nil {
		if isConfigNotFound(err) {
			return nil
		}
		return fmt.Errorf("读取 %s: %w", file, err)
	}
	if err := base.MergeConfigMap(v.AllSettings()); err != nil {
		return fmt.Errorf("合并 %s: %w", file, err)
	}
	return nil
}

// isConfigNotFound 判断错误是否为「配置文件不存在」。
//
// viper 的 ConfigFileNotFoundError 没有实现 Unwrap(fs.ErrNotExist)，
// 因此不能用 errors.Is(err, fs.ErrNotExist)，只能按具体类型判断。
func isConfigNotFound(err error) bool {
	var notFound viper.ConfigFileNotFoundError
	return errors.As(err, &notFound)
}

// bindEnvs 显式绑定关键环境变量，保持与原有 .env 变量名兼容。
//
//nolint:gosec // G101 误报：这是「配置项 -> 环境变量名」的映射表，值均为变量名，不含真实密钥。
func bindEnvs(v *viper.Viper) {
	bindings := map[string]string{
		"database.driver":                         "DB_DRIVER",
		"app.env":                                 "APP_ENV",
		"app.server_addr":                         "SERVER_ADDR",
		"app.read_header_timeout":                 "SERVER_READ_HEADER_TIMEOUT",
		"app.read_timeout":                        "SERVER_READ_TIMEOUT",
		"app.write_timeout":                       "SERVER_WRITE_TIMEOUT",
		"app.idle_timeout":                        "SERVER_IDLE_TIMEOUT",
		"app.request_timeout":                     "SERVER_REQUEST_TIMEOUT",
		"app.send_code_request_timeout":           "SERVER_SEND_CODE_REQUEST_TIMEOUT",
		"app.trusted_proxies":                     "SERVER_TRUSTED_PROXIES",
		"database.url":                            "DATABASE_URL",
		"database.host":                           "DB_HOST",
		"database.port":                           "DB_PORT",
		"database.user":                           "DB_USER",
		"database.password":                       "DB_PASSWORD",
		"database.name":                           "DB_NAME",
		"database.ssl_mode":                       "DB_SSL_MODE",
		"database.pool.max_conns":                 "DB_MAX_CONNS",
		"database.pool.min_conns":                 "DB_MIN_CONNS",
		"database.pool.max_conn_idle":             "DB_MAX_CONN_IDLE",
		"database.pool.max_conn_life":             "DB_MAX_CONN_LIFE",
		"database.statement_timeout":              "DB_STATEMENT_TIMEOUT",
		"redis.url":                               "REDIS_URL",
		"redis.host":                              "REDIS_HOST",
		"redis.port":                              "REDIS_PORT",
		"redis.password":                          "REDIS_PASSWORD",
		"redis.db":                                "REDIS_DB",
		"redis.pool_size":                         "REDIS_POOL_SIZE",
		"redis.read_timeout":                      "REDIS_READ_TIMEOUT",
		"redis.write_timeout":                     "REDIS_WRITE_TIMEOUT",
		"redis.key_prefix":                        "REDIS_KEY_PREFIX",
		"jwt.secret":                              "JWT_SECRET",
		"jwt.access_ttl":                          "JWT_ACCESS_TTL",
		"jwt.refresh_ttl":                         "JWT_REFRESH_TTL",
		"auth.mock_code_enabled":                  "AUTH_MOCK_CODE_ENABLED",
		"auth.multi_device_enabled":               "AUTH_MULTI_DEVICE_ENABLED",
		"auth.redis_fail_open":                    "AUTH_REDIS_FAIL_OPEN",
		"auth.require_code_for_password_register": "AUTH_REQUIRE_CODE_FOR_PASSWORD_REGISTER",
		"auth.code_channels":                      "AUTH_CODE_CHANNELS",
		"auth.code_daily_limit_per_target":        "AUTH_CODE_DAILY_LIMIT_PER_TARGET",
		"auth.code_daily_limit_per_ip":            "AUTH_CODE_DAILY_LIMIT_PER_IP",
		"smtp.host":                               "SMTP_HOST",
		"smtp.port":                               "SMTP_PORT",
		"smtp.username":                           "SMTP_USERNAME",
		"smtp.password":                           "SMTP_PASSWORD",
		"smtp.from":                               "SMTP_FROM",
		"smtp.dial_timeout":                       "SMTP_DIAL_TIMEOUT",
		"smtp.send_timeout":                       "SMTP_SEND_TIMEOUT",
		"oauth.dev_mode":                          "OAUTH_DEV_MODE",
		"oauth.providers":                         "OAUTH_PROVIDERS",
		"oauth.wechat.app_id":                     "OAUTH_WECHAT_APP_ID",
		"oauth.wechat.app_secret":                 "OAUTH_WECHAT_APP_SECRET",
		"oauth.apple.client_id":                   "OAUTH_APPLE_CLIENT_ID",
		"oauth.google.client_id":                  "OAUTH_GOOGLE_CLIENT_ID",
		"log.level":                               "LOG_LEVEL",
		"log.format":                              "LOG_FORMAT",
		"rate_limit.per_minute":                   "RATE_LIMIT_PER_MINUTE",
		"rate_limit.user_per_minute":              "RATE_LIMIT_USER_PER_MINUTE",
		"rate_limit.auth_per_minute":              "RATE_LIMIT_AUTH_PER_MINUTE",
		"metrics.enabled":                         "METRICS_ENABLED",
		"metrics.token":                           "METRICS_TOKEN",
		"cors.allow_origins":                      "CORS_ALLOW_ORIGINS",
		"cors.allow_credentials":                  "CORS_ALLOW_CREDENTIALS",
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
	if c.DBDriver == "mysql" {
		return (&mysql.Config{
			User: c.DBUser, Passwd: c.DBPassword, Net: "tcp",
			Addr: c.DBHost + ":" + c.DBPort, DBName: c.DBName,
			ParseTime: true, Loc: time.UTC,
			Params: map[string]string{"time_zone": "'+00:00'", "loc": "UTC"},
		}).FormatDSN()
	}
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

// NormalizeMySQLDSN 为 MySQL DSN 强制补齐 parseTime=true、loc=UTC 与
// time_zone='+00:00'：DATETIME 列不带时区，会话时区钉在 UTC 才能保证
// CURRENT_TIMESTAMP 写入的就是 UTC 时刻、回读不再产生本地时区偏移。
// 解析失败时返回错误，调用方保留原值（连接阶段会暴露真正的语法问题）。
func NormalizeMySQLDSN(dsn string) (string, error) {
	mc, err := mysql.ParseDSN(dsn)
	if err != nil {
		return "", err
	}
	mc.ParseTime = true
	mc.Loc = time.UTC
	if mc.Params == nil {
		mc.Params = map[string]string{}
	}
	// FormatDSN 在 Loc 为零值默认外不会输出 loc 参数（UTC 时直接省略），
	// 而驱动解析 DSN 时缺省 loc=Local，必须显式写入避免读回时刻漂移。
	if _, ok := mc.Params["loc"]; !ok {
		mc.Params["loc"] = "UTC"
	}
	if _, ok := mc.Params["time_zone"]; !ok {
		mc.Params["time_zone"] = "'+00:00'"
	}
	return mc.FormatDSN(), nil
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
