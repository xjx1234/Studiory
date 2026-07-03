# Studiory 技术学习指南

本文档从脚手架代码中提炼核心技术点，面向想学习 Go 工程化实践的开发者。每个章节先讲原理，再结合项目代码说明。

---

## 目录

1. [分层架构与依赖注入](#1-分层架构与依赖注入)
2. [配置系统](#2-配置系统)
3. [安全体系](#3-安全体系)
4. [测试策略](#4-测试策略)
5. [CI 流水线与契约校验](#5-ci-流水线与契约校验)
6. [容器化部署](#6-容器化部署)
7. [可观测性](#7-可观测性)
8. [实战：新增一个业务模块](#8-实战新增一个业务模块)
9. [设计模式速查](#9-设计模式速查)

---

## 1. 分层架构与依赖注入

### 1.1 分层结构

```
main.go
  └── internal/app/app.go          ← 应用容器（装配一切）
        ├── internal/http/          ← Handler 层（路由 + 请求解析 + 响应）
        ├── internal/service/       ← 业务逻辑层（auth / user / admin / todo）
        ├── internal/repo/          ← 数据访问接口层
        │     └── internal/repo/pg/ ← 数据访问实现层（sqlc 生成 + 手写）
        ├── internal/store/         ← 基础设施连接（PostgreSQL pool、Redis client）
        └── pkg/                    ← 公共工具包（errcode / resp / validator / i18n）
```

**原则**：每一层只依赖直接下层，绝不跳层调用。Handler 不直接碰 Repo，Service 不知道 HTTP 请求的存在。

### 1.2 依赖注入：Deps 聚合

`internal/http/deps.go` 定义了 Handler 层所需的全部依赖：

```go
type Deps struct {
    Cfg    *config.Config
    Logger *zap.Logger

    AuthService  authservice.Service    // 接口，不是具体类型
    UserService  userservice.Service
    AdminService adminservice.Service
    TodoService  todoservice.Service

    AuthMiddleware          gin.HandlerFunc
    RateLimitMiddleware     gin.HandlerFunc
    UserRateLimitMiddleware gin.HandlerFunc
    MetricsMiddleware       gin.HandlerFunc
    MetricsHandler          http.Handler
    ReadyChecks             []ReadyCheck
}
```

**关键设计**：
- `AuthService` 是**接口类型**而非 `*AuthServiceImpl`，Handler 层完全不知道实现细节
- 中间件也是注入的，测试时可以替换成 fake

### 1.3 应用容器：app.go 的装配流程

`internal/app/app.go` 是整个应用的"组装车间"：

```go
func New(ctx context.Context, cfg *config.Config, logger *zap.Logger) (*App, error) {
    // 1. 连接基础设施
    pool := store.NewPostgres(ctx, cfg.DatabaseURL, ...)
    rdb  := store.NewRedis(ctx, cfg.RedisURL, ...)

    // 2. 创建 Repo 层（sqlc store）
    pgStore := pg.NewStore(pool)

    // 3. 创建 Service 层（Option 模式注入依赖）
    authSvc := authservice.New(pgStore.Users(), rdb,
        authservice.WithTokenIssuer(tokenIssuer),
        authservice.WithOAuthRepo(pgStore.OAuth()),
        authservice.WithSessionStore(sessionStore),
        authservice.WithLogger(logger),
        // ... 更多 Option
    )

    // 4. 组装 Deps
    deps := &internalhttp.Deps{
        AuthService: authSvc,
        // ...
    }

    // 5. 构建路由
    router := internalhttp.NewRouter(deps)

    // 6. 创建 HTTP Server
    server := &http.Server{Addr: cfg.ServerAddr, Handler: router, ...}

    return &App{Router: router, Server: server, ...}, nil
}
```

**学习要点**：`app.New()` 是唯一知道所有具体类型的地方。其它层只通过接口交互。

### 1.4 Option 模式

每个 Service 都用 Option 函数模式管理可选依赖：

```go
type Option func(*AuthServiceImpl)

func WithLogger(logger *zap.Logger) Option {
    return func(s *AuthServiceImpl) {
        s.SetLogger(logger)
    }
}

func WithSessionStore(store *session.Store) Option {
    return func(s *AuthServiceImpl) {
        s.sessions = store
    }
}

// 使用
func New(users repo.UserRepo, rdb redis.UniversalClient, opts ...Option) Service {
    s := &AuthServiceImpl{
        users:    users,
        rdb:      rdb,
        codePrefix: "app",       // 默认值
        allowMockCodeFallback: true,
    }
    for _, opt := range opts {
        opt(s)
    }
    return s
}
```

**优势**：
- 必填依赖（`users`、`rdb`）放函数参数，编译时检查
- 可选依赖（logger、session、oauth）用 Option 注入，不使用时零值
- 新增依赖只需新增一个 Option 函数，不改 `New` 签名

### 1.5 错误传播链

```
pgx.ErrNoRows
    ↓ repo/pg/wrapErr()
repo.ErrNotFound          ← 数据访问层错误
    ↓ service 层 errors.Is 判断
errcode.ErrNotFound       ← 业务错误码（含 HTTP 状态码 + i18n MsgID）
    ↓ resp.Fail(c, err)
JSON: {"code":30001, "message":"未找到资源"}   ← 前端只看 code
```

`pkg/errcode/errcode.go` 定义错误码：

```go
type Error struct {
    Code       int    // 业务错误码
    MsgID      string // i18n 消息 ID
    HTTPStatus int    // HTTP 状态码
}

// 错误码分段
//   0      = 成功
//   1xxxx  = 认证/权限
//   2xxxx  = 请求参数
//   3xxxx  = 业务资源
//   5xxxx  = 服务器内部
```

### 1.6 LogSupport 共享日志能力

`internal/service/logging.go` 提供日志基础设施：

```go
type LogSupport struct {
    Logger *zap.Logger
}

func (l *LogSupport) LogInternal(op string, err error, fields ...zap.Field) {
    if l.Logger != nil && err != nil {
        fields = append(fields, zap.Error(err))
        l.Logger.Error(op, fields...)
    }
}

// 标准化日志字段
func UserIDField(userID string) zap.Field {
    if userID == "" { return zap.Skip() }  // 空值不记录
    return zap.String("user_id", userID)
}
```

各 Service 匿名嵌入 `LogSupport`，直接调用 `s.LogInternal("op", err, baseservice.UserIDField(uid))`。

### 1.7 Store 模式与事务

`internal/repo/pg/store.go` 是 Repo 层的统一入口，封装 sqlc 生成的 Queries：

```go
type Store struct {
    pool *pgxpool.Pool
    q    *sqlcgen.Queries
}

func (s *Store) Users() repo.UserRepo  { return &userRepo{q: s.q} }
func (s *Store) OAuth() repo.OAuthRepo { return &oauthRepo{q: s.q} }
func (s *Store) Todos() repo.TodoRepo  { return &todoRepo{q: s.q} }
```

上层 Service 只依赖 `repo.UserRepo` 接口，不直接碰 sqlc。事务通过 `WithinTx` 封装：

```go
func (s *Store) WithinTx(ctx context.Context, fn func(txStore *Store) error) error {
    tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
    if err != nil { return err }
    defer func() { _ = tx.Rollback(ctx) }()

    txStore := s.WithTx(tx)  // 新 Store 绑定到事务
    if err := fn(txStore); err != nil {
        return err  // 自动 rollback
    }
    return tx.Commit(ctx)
}
```

业务使用示例（OAuth 注册需要原子操作）：

```go
// 创建用户 + 绑定 OAuth 必须在同一事务
s.oauthTx.WithUserOAuthTx(ctx, func(users repo.UserRepo, oauth repo.OAuthRepo) error {
    user, _ := users.Create(ctx, &repo.CreateUserParams{...})
    oauth.CreateOAuth(ctx, user.ID, "wechat", openID)
    return nil
})
```

### 1.8 Provider 路由模式

脚手架有两个典型的多服务道路由抽象，值得学习：

**验证码下发器** (`internal/sender/sender.go`)：

```go
// Provider 接口：单个下发商
type Provider interface {
    Name() string
    Supports(ch Channel) bool  // sms / email
    Send(ctx context.Context, msg Message) error
}

// Router：同渠道多 Provider 顺序故障转移
func (r *Router) Send(ctx context.Context, msg Message) error {
    providers := r.byChannel[msg.Channel]  // 按渠道查 Provider 列表
    for _, p := range providers {
        if err := p.Send(ctx, msg); err != nil {
            r.logger.Warn("send failed, trying next", ...)  // 记录日志，继续下一个
            continue
        }
        return nil  // 成功即返回
    }
    return fmt.Errorf("all providers failed")
}
```

装配逻辑（`app.go`）：有 SMTP 用 SMTP，没有就用 Mock，**同一渠道可注册多个做故障转移**。

**OAuth 校验器** (`internal/oauth/oauth.go`)：同样的模式——Provider 接口 + Router 按平台名路由 + dev_mode 快捷方式。

### 1.9 构建信息注入

`internal/buildinfo/buildinfo.go` 通过 ldflags 在编译时注入：

```go
package buildinfo

var (
    Version   = "unknown"
    Commit    = "unknown"
    BuildTime = "unknown"
)
```

`main.go` 启动时打印版本信息：

```go
zap.L().Info("配置加载完成",
    zap.String("version", buildinfo.Version),
    zap.String("commit", buildinfo.Commit),
    zap.String("build_time", buildinfo.BuildTime),
)
```

`/health` 接口也返回版本，运维随时可查：

```bash
curl http://localhost:8080/health
# {"data":{"status":"ok","version":"v1.0.0","commit":"abc1234","build_time":"2026-..."}}
```

---

## 2. 配置系统

### 2.1 四层配置合并

`internal/config/config.go` 使用 Viper 实现四层配置加载：

```
优先级（高 → 低）：
  1. 系统环境变量          ← 部署平台注入，最高优先
  2. config/local.yaml     ← 本地私有（gitignore），不提交
  3. config/{env}.yaml     ← 环境专用（development.yaml / production.yaml）
  4. config/base.yaml      ← 全局默认值，提交到 git
```

```go
func loadViper() *viper.Viper {
    v := viper.New()
    v.SetConfigName("base")          // 读 base.yaml
    v.ReadInConfig()
    mergeConfig(v, configPath, env)  // 合并 {env}.yaml
    mergeConfig(v, configPath, "local") // 合并 local.yaml
    v.AutomaticEnv()                 // 环境变量覆盖
    bindEnvs(v)                      // 显式绑定（兼容 .env 变量名）
    return v
}
```

**环境变量映射**通过 `bindEnvs` 显式声明，支持点路径自动转换：

```go
bindings := map[string]string{
    "jwt.secret":    "JWT_SECRET",
    "database.url":  "DATABASE_URL",
    "log.level":     "LOG_LEVEL",
    // ... 60+ 个配置项
}
```

**设计要点**：
- `local.yaml` 加入 `.gitignore`，本地开发可以在这里覆盖敏感值
- `DATABASE_URL` / `REDIS_URL` 为空时自动从分项（host/port/user）拼接
- `bindEnvs` 兼容 `.env` 文件的变量名风格（大写 + 下划线）

### 2.2 启动时配置校验

`main.go` 在启动时调用 `cfg.Validate()` 做安全检查：

```go
func (c *Config) Validate() error {
    // 基础检查
    if c.ServerAddr == ""              { return errors.New("...") }
    if c.DatabaseURL == ""             { return errors.New("...") }

    // 生产环境专项检查
    if c.IsProd() {
        if c.JWTSecret == "dev-secret-change-in-production" {
            return errors.New("production 必须设置安全的 JWT_SECRET")
        }
        if c.AuthMockCodeEnabled {
            return errors.New("production 不能启用 mock 验证码")
        }
        if c.OAuthDevMode {
            return errors.New("production 不能启用 OAuth dev mode")
        }
    }
    // ...
}
```

**意义**：避免带着开发环境的不安全配置上生产。

### 2.3 统一响应协议

`pkg/resp/response.go` 定义统一响应格式：

```json
{
    "code":       0,
    "message":    "成功",
    "data":       { ... },
    "request_id": "a1b2c3..."
}
```

四个辅助函数覆盖所有场景：

```go
resp.OK(c, data)                          // 成功，200
resp.Fail(c, errcode.ErrNotFound)         // 业务错误，i18n 翻译消息
resp.FailWithMessage(c, err, "自定义")    // 带文本消息，不走 i18n
resp.FailWithValidation(c, fields)        // 校验失败，返回字段级错误
```

### 2.4 请求绑定与校验

`pkg/request/bind.go` 封装 Gin 的绑定 + go-playground/validator：

```go
// Handler 中
var req struct {
    Title  string  `json:"title" binding:"required,max=200"`
    Amount float64 `json:"amount" binding:"required,gt=0"`
}
if !request.Bind(c, &req) {
    return  // 已自动写入校验错误响应
}
```

校验失败时自动返回结构化错误：

```json
{
    "code": 20002,
    "message": "参数校验失败",
    "data": { "fields": { "title": "title 是必填字段" } }
}
```

支持三种绑定方式：`Bind`（JSON）、`BindQuery`（Query 参数）、`BindURI`（路径参数）。

### 2.5 优雅关闭

`main.go` 实现完整的优雅关闭流程：

```go
// 1. 监听退出信号
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()

// 2. 非阻塞启动 HTTP Server
go func() {
    a.Server.ListenAndServe()
}()

// 3. 等待信号
<-ctx.Done()

// 4. 带超时优雅关闭（10 秒内等待请求完成）
shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
a.Shutdown(shutdownCtx)  // 先停 HTTP Server，再关闭 PG + Redis
```

---

## 3. 安全体系

### 3.1 JWT 双 Token 机制

**Access Token**：短生命（2h），携带用户 ID + Role + SessionID，用于 API 鉴权。

**Refresh Token**：长生命（7d），用于换取新的 Access Token，每次使用后作废（旋转）。

```go
// 签发时同时生成一对
pair, err := tokenIssuer.IssueTokenPair(userID, role, sessionID)

// 刷新流程
func (s *AuthServiceImpl) Refresh(ctx context.Context, refreshToken string) (*auth.TokenPair, error) {
    // 1. 检查 refresh token 是否在黑名单
    if s.rdb.Exists(ctx, s.blacklistKey(refreshToken)) > 0 {
        return nil, errcode.ErrInvalidToken
    }

    // 2. 解析 refresh token 拿到 claims
    claims := s.tokens.ParseRefreshToken(refreshToken)

    // 3. 检查用户是否被禁用
    user := s.users.GetByID(ctx, claims.UserID)
    if user.Status == repo.StatusDisabled {
        return nil, errcode.ErrAccountDisabled
    }

    // 4. 校验 session 是否有效
    if !s.sessions.Validate(ctx, claims.UserID, claims.SessionID) {
        return nil, errcode.ErrInvalidToken
    }

    // 5. 将旧 refresh token 加入黑名单
    s.rdb.Set(ctx, s.blacklistKey(refreshToken), "1", refreshBlacklistTTL)

    // 6. 签发新的 token 对
    return s.tokens.IssueTokenPair(claims.UserID, claims.Role, claims.SessionID)
}
```

**Access Token 用户级吊销**：改密/禁用时写入 `revoke:uid:{uid}` key，Auth 中间件校验时检查。

### 3.2 Session 管理（多设备 / 单设备）

`internal/session/session.go` 实现两种模式：

| 模式 | Redis 结构 | 新登录行为 | 登出行为 |
|------|-----------|-----------|---------|
| 多设备 | `Set:{uid} → [sid1, sid2]` | 创建独立 session | 只吊销当前 |
| 单设备 | `active:{uid} → sid` | 踢掉旧 session | 吊销当前 |

```go
func (s *Store) Validate(ctx context.Context, userID, sessionID string) bool {
    if s.multiDevice {
        // 多设备：查 Set 中是否有该 session
        ok, _ := s.rdb.SIsMember(ctx, s.userSessionsKey(userID), sessionID).Result()
        if ok { return true }
    } else {
        // 单设备：查 active key 是否匹配
        active, _ := s.rdb.Get(ctx, s.activeSessionKey(userID)).Result()
        if active == sessionID { return true }
    }

    // 兜底：session 键本身仍存在也视为有效
    uid, err := s.rdb.Get(ctx, s.sessionKey(sessionID)).Result()
    return err == nil && uid == userID
}
```

### 3.3 暴力破解防护

`internal/service/auth/service.go` 使用 Lua 脚本原子操作：

```lua
-- loginRecordFailLua：一次 Redis 调用完成递增 + 设 TTL + 达到阈值时锁定
local count = redis.call('INCR', KEYS[1])
if count == 1 then
    redis.call('EXPIRE', KEYS[1], tonumber(ARGV[1]))
end
if count >= tonumber(ARGV[2]) then
    redis.call('SET', KEYS[2], '1', 'EX', tonumber(ARGV[3]))
end
return count
```

参数：5 次失败 / 10 分钟窗口 / 锁定 15 分钟。

**设计要点**：
- **fail-open**：Redis 故障时放行登录，不阻断业务
- **SHA-256 key**：账号标识经哈希后作为 Redis key，不暴露原始手机号/邮箱
- **用户不存在也计数**：防止通过响应时间枚举账号

### 3.4 中间件链

```
请求进入
  │
  ├── RequestID           ← 注入 X-Request-Id
  ├── Metrics             ← HTTP 指标采集（可选）
  ├── SecurityHeaders     ← X-Content-Type-Options / X-Frame-Options / HSTS
  ├── I18n                ← 语言解析
  ├── Ginzap              ← 结构化访问日志（request_id + user_id）
  ├── Recovery            ← panic 恢复
  ├── Safe                ← Body 超限检测（errors.As）
  ├── RateLimit(IP)       ← 公开路由按客户端 IP 限流
  ├── CORS                ← 跨域
  │
  └── [路由分组]
        ├── /api/v1/auth/*        ← 无需鉴权
        ├── /api/v1/user/*        ← Auth + RateLimitByUser
        └── /api/v1/admin/*       ← Auth + RateLimitByUser + RequireRole(admin)
```

**限流分层**（`internal/http/middleware/rate_limit.go`）：
- `RateLimit(IP)`：跳过 `/api/v1/user` 和 `/api/v1/admin`，避免同 NAT 误伤
- `RateLimitByUser(user_id)`：挂在 Auth 中间件之后，按用户 ID 独立计数
- Redis 不可用时回退到内存 store

---

## 4. 测试策略

### 4.1 四层测试架构

```
┌─────────────────────────────────────────────────────┐
│  E2E 测试 (e2e/)                                      │  真实 PG + Redis 容器
│  测试完整 HTTP 请求链路：注册→登录→CRUD→登出             │  整个 app.New() 启动
├─────────────────────────────────────────────────────┤
│  Handler 集成测试 (internal/http/*_test.go)            │  fake service + httptest
│  测试路由、中间件、请求绑定、响应格式                    │  不启动真实服务
├─────────────────────────────────────────────────────┤
│  仓库集成测试 (internal/repo/pg/integration_test.go)   │  testcontainers 真实 PG
│  测试 SQL 正确性、事务、约束冲突                        │  直接操作 DB
├─────────────────────────────────────────────────────┤
│  单元测试 (service/*_test.go 等)                       │  fake repo + miniredis
│  测试业务逻辑：输入验证、错误处理、边界条件              │  无外部依赖
└─────────────────────────────────────────────────────┘
```

### 4.2 单元测试：Fake Repo 模式

`internal/testutil/fake_user_repo.go` 提供共享的 fake 实现：

```go
type FakeUserRepo struct {
    Users   map[uuid.UUID]*repo.User
    Created []*repo.User
}

func (r *FakeUserRepo) GetByPhone(_ context.Context, phone string) (*repo.User, error) {
    for _, u := range r.Users {
        if u.Phone != nil && *u.Phone == phone {
            return u, nil
        }
    }
    return nil, repo.ErrNotFound
}

func (r *FakeUserRepo) UpdatePassword(_ context.Context, id uuid.UUID, hash string) (*repo.User, error) {
    u, ok := r.Users[id]
    if !ok { return nil, repo.ErrNotFound }
    u.PasswordHash = &hash  // 真正修改数据，测试可以验证副作用
    return u, nil
}
```

Service 测试中直接注入 fake：

```go
func TestChangePassword_Success(t *testing.T) {
    fake := testutil.NewFakeUserRepo()
    // 预设数据
    fake.Users[uid] = &repo.User{
        ID:           uid,
        PasswordHash: strPtr("$2a$10$oldHash"),
    }

    svc := userservice.New(fake, userservice.WithLogger(zap.NewNop()))
    err := svc.ChangePassword(ctx, uid.String(), &userservice.ChangePasswordInput{
        OldPassword: "old",
        NewPassword: "new",
    })

    if err != nil { t.Fatalf("expected nil, got %v", err) }
    // 验证密码已被更新
    if *fake.Users[uid].PasswordHash == "$2a$10$oldHash" {
        t.Fatal("password should have been updated")
    }
}
```

### 4.3 集成测试：testcontainers

`internal/testutil/integration/containers.go` 封装容器启动：

```go
func StartPostgres(ctx context.Context) (*Postgres, error) {
    container, err := postgres.Run(ctx, "postgres:16-alpine",
        postgres.WithDatabase("app_e2e"),
        postgres.WithUsername("test"),
        postgres.WithPassword("test"),
        testcontainers.WithWaitStrategy(
            wait.ForLog("database system is ready to accept connections").
                WithOccurrence(2).       // PG 启动时打印两次
                WithStartupTimeout(90 * time.Second),
        ),
    )
    // ...
    // 启动后自动执行 migrations
    ApplyMigrations(ctx, pool)
    return &Postgres{DSN: dsn, Pool: pool, container: container}, nil
}
```

仓库集成测试：

```go
//go:build integration

func TestUserRepo_CreateAndGet(t *testing.T) {
    ctx := context.Background()
    pgEnv := integration.MustStartPostgres(ctx, t)
    defer pgEnv.Close(ctx)

    repo := pg.NewStore(pgEnv.Pool).Users()
    // 直接测试真实 SQL
    user, err := repo.Create(ctx, &repo.CreateUserParams{
        Nickname: "test",
    })
    // ...
}
```

### 4.4 E2E 测试：全链路

`e2e/main_test.go` 启动完整的 `app.New()`：

```go
func TestMain(m *testing.M) {
    // 启动真实 PG + Redis 容器
    pgEnv, _ := integration.StartPostgres(ctx)
    redisEnv, _ := integration.StartRedis(ctx)

    // 用 app.New() 构建完整应用（与生产一致）
    cfg := testConfig(pgEnv.DSN, redisEnv.URL)
    a, _ := app.New(ctx, cfg, zap.NewNop())
    e2eRouter = a.Router

    code := m.Run()

    a.Close()
    pgEnv.Close(ctx)
    redisEnv.Close(ctx)
    os.Exit(code)
}
```

E2E 测试用 `httptest` 发送真实 HTTP 请求：

```go
func TestAuthRegisterLoginProfile(t *testing.T) {
    // 注册
    w, body := doJSON(t, http.MethodPost, "/api/v1/auth/register", map[string]any{
        "grant_type": "password",
        "phone":      uniquePhone(),
        "password":   "Str0ng!Pass",
    })

    // 登录
    w, body = doJSON(t, http.MethodPost, "/api/v1/auth/login", ...)

    // 查 profile（带 Bearer token）
    w, body = doJSON(t, http.MethodGet, "/api/v1/user/profile", nil,
        bearer(loginData.Tokens.AccessToken)...)

    // 验证返回的 role 和 phone
    if profile.Role != repo.RoleUser { t.Fatal(...) }
}
```

### 4.5 Handler 集成测试：Fake Service

`internal/http/fakes_test.go` 定义三个 fake service，通过可选函数字段覆写行为：

```go
type fakeAuthService struct {
    onLogin    func(ctx context.Context, req *auth.LoginRequest) (*auth.LoginResult, *errcode.Error)
    onRegister func(ctx context.Context, in *authservice.RegisterInput) (*auth.LoginResult, *errcode.Error)
    // ... 更多可选字段
}

func (f *fakeAuthService) Login(ctx context.Context, req *auth.LoginRequest) (*auth.LoginResult, *errcode.Error) {
    if f.onLogin != nil {
        return f.onLogin(ctx, req)  // 测试可覆写
    }
    return &auth.LoginResult{...}, nil  // 默认返回合理值
}
```

编译时接口校验：

```go
var _ Service = (*fakeAuthService)(nil)
```

测试中只需关注异常路径：

```go
func TestLogin_AccountLocked(t *testing.T) {
    ts := &testServer{
        auth: &fakeAuthService{
            onLogin: func(...) {
                return nil, errcode.ErrAccountLocked
            },
        },
    }
    // 发请求，验证返回 429
}
```

---

## 5. CI 流水线与契约校验

### 5.1 CI 四并行 Job

`.github/workflows/ci.yml`：

```yaml
jobs:
  lint:          # golangci-lint v2.12.2
  vuln:          # govulncheck 依赖漏洞扫描
  integration:   # testcontainers repo 测试 + E2E 测试
  backend:       # sqlc verify + OpenAPI 校验 + go test -race + coverage + build
```

四个 Job 互不依赖，**并行执行**，任何一个失败即整体失败。

### 5.2 OpenAPI 契约校验

`internal/openapi/` 包实现"路由 vs 文档"双向对比：

```go
// 从 openapi.yaml 提取操作列表
doc, _ := openapi.LoadDocument("docs/api/openapi.yaml")
docOps := openapi.OperationsFromDocument(doc)

// 从 Gin 路由表提取操作列表
routerOps := openapi.OperationsFromRoutes(gin.Routes(),
    []string{"/metrics"},  // 排除 Prometheus 端点
    nil,
)

// 双向对比
onlyDoc, onlyRouter := openapi.CompareOperations(docOps, routerOps)
if len(onlyDoc) > 0 {
    // 文档中有但路由没有 → 文档过时
    t.Errorf("operations in openapi.yaml but missing in router:\n  %s", ...)
}
if len(onlyRouter) > 0 {
    // 路由中有但文档没有 → 文档遗漏
    t.Errorf("router operations missing in openapi.yaml:\n  %s", ...)
}
```

CI 中作为 `backend` job 的一个步骤运行：

```yaml
- name: Verify OpenAPI contract
  run: go test -run 'Test(OpenAPI|LoadDocument)' ./internal/openapi/... ./internal/http/...
```

**效果**：任何新增/删除接口如果忘了同步 `openapi.yaml`，CI 直接失败。

---

## 6. 容器化部署

### 6.1 Dockerfile 多阶段构建

```dockerfile
# ── Builder 阶段 ──
FROM golang:alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./

# 从 go.mod 自动提取 Go 版本，避免手动同步
RUN GO_VERSION=$(awk '/^go /{print $2}' go.mod) \
    && GOTOOLCHAIN=go${GO_VERSION}+auto go mod download

COPY . .

# 构建信息注入 + 去符号 + 裁剪
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w \
        -X backend/internal/buildinfo.Version=${VERSION} \
        -X backend/internal/buildinfo.Commit=${COMMIT} \
        -X backend/internal/buildinfo.BuildTime=${build_time}" \
    -o /out/app-api .

# ── Runtime 阶段 ──
FROM alpine:3.22 AS runtime

RUN addgroup -S app && adduser -S -G app app
COPY --from=builder /out/app-api /app/app-api
COPY config /app/config

USER app  # 非 root 运行
HEALTHCHECK CMD wget -qO- http://127.0.0.1:8080/health >/dev/null || exit 1
```

**关键点**：
- `CGO_ENABLED=0`：纯静态链接，alpine 也能跑
- `-trimpath`：去掉编译路径，安全 + 可重现
- `-s -w`：去符号表 + DWARF，缩小镜像体积
- `GOTOOLCHAIN=auto`：Go 工具链自动下载 go.mod 指定的版本

### 6.2 K8s 部署模板

`deploy/k8s/api-deployment.yaml` 核心设计：

```yaml
spec:
  replicas: 2
  strategy:
    rollingUpdate:
      maxSurge: 1          # 最多多一个 Pod
      maxUnavailable: 0    # 滚动更新时零停机

  containers:
    - name: api
      livenessProbe:       # 存活探针
        httpGet:
          path: /health    # 不查 PG/Redis，避免依赖故障误重启
          port: http
        initialDelaySeconds: 10
        periodSeconds: 30

      readinessProbe:      # 就绪探针
        httpGet:
          path: /ready     # PG + Redis 均可用才接流量
          port: http
        periodSeconds: 10

      resources:
        requests: { cpu: 100m, memory: 128Mi }
        limits:   { cpu: 500m, memory: 512Mi }

      securityContext:
        allowPrivilegeEscalation: false
        runAsNonRoot: true
```

**探针分离的意义**：
- **liveness** 问"进程还活着吗？" → 只要进程能响应 HTTP 就算活
- **readiness** 问“能接流量吗？” → PG/Redis 连不通就不接，让 kubelet 从 Service 摘掉该 Pod

### 6.3 docker-compose 编排

`docker-compose.yml` 使用 `depends_on` + healthcheck + profiles 实现全栈启动：

```yaml
services:
  postgres:
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres"]

  migrate:
    profiles: ["full"]   # 普通 compose-up 不启动
    depends_on:
      postgres: { condition: service_healthy }
    command: ["-path", "/migrations", "-database", "...", "up"]

  api:
    profiles: ["full"]
    depends_on:
      postgres:  { condition: service_healthy }
      redis:     { condition: service_healthy }
      migrate:   { condition: service_completed_successfully }
```

启动顺序保证：PG/Redis 健康 → migrate 完成 → API 启动。profiles 机制让日常开发只用 `compose-up` 起 PG+Redis，API 在本地 `make run`。

---

## 7. 可观测性

### 7.1 Prometheus 指标

`internal/metrics/metrics.go` 使用独立 Registry（不污染全局）：

```go
reg := prometheus.NewRegistry()
reg.MustRegister(
    collectors.NewGoCollector(),         // goroutine/GC/FD
    collectors.NewProcessCollector(...), // 进程级指标
)

// 业务指标
m.reqTotal    = prometheus.NewCounterVec(...)    // http_requests_total
m.reqDuration = prometheus.NewHistogramVec(...)  // http_request_duration_seconds
m.inFlight    = prometheus.NewGauge(...)         // http_requests_in_flight
```

**label 用路由模板**（`c.FullPath()`）而非真实路径，避免 label 基数爆炸：

```go
route := c.FullPath()  // "/api/v1/user/todos/:id" 而非 "/api/v1/user/todos/123"
if route == "" { route = "unmatched" }
m.reqTotal.WithLabelValues(c.Request.Method, route, status).Inc()
```

**连接池指标**通过自定义 `prometheus.Collector` 采集：

```go
// pgx_collector.go
type PgxPoolCollector struct {
    pool *pgxpool.Pool
}

func (c *PgxPoolCollector) Collect(ch chan<- prometheus.Metric) {
    stat := c.pool.Stat()
    ch <- prometheus.MustNewConstMetric(acquiredDesc, prometheus.GaugeValue, float64(stat.AcquiredConns()))
    // ... 空闲连接、总连接、新建连接等
}
```

### 7.2 结构化日志

**访问日志字段注入**（`internal/http/middleware/access_log_fields.go`）：

```go
func AccessLogFields(c *gin.Context) []zapcore.Field {
    fields := []zapcore.Field{
        zap.String("request_id", requestid.Get(c)),
    }
    if raw, ok := c.Get(ContextKeyUserID); ok {
        if uid, ok := raw.(string); ok && uid != "" {
            fields = append(fields, zap.String("user_id", uid))
        }
    }
    return fields
}
```

通过 `GinzapWithConfig.Context` 注入，每条访问日志自动带 `request_id` + `user_id`。

**Service 层日志**使用 `UserIDField` helper 确保 `user_id` 字段与访问日志一致：

```go
s.LogInternal("ChangePassword update password", err,
    baseservice.UserIDField(userID),  // 空值自动省略
)
```

**排障链路**：响应中的 `request_id` → 日志检索 → `user_id` 关联。

---

## 8. 实战：新增一个业务模块

以"订单"模块为例，演示完整步骤：

### 步骤 1：Repo 层

```sql
-- migrations/000004_create_orders.up.sql
CREATE TABLE orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id),
    title VARCHAR(200) NOT NULL,
    amount DECIMAL(10,2) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- sqlc/query/orders.sql
-- name: CreateOrder :one
INSERT INTO orders (user_id, title, amount) VALUES ($1, $2, $3) RETURNING *;

-- name: GetOrderByID :one
SELECT * FROM orders WHERE id = $1;

-- name: ListOrdersByUser :many
SELECT * FROM orders WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: CountOrdersByUser :one
SELECT COUNT(*) FROM orders WHERE user_id = $1;
```

然后 `sqlc generate` 自动生成 Go 代码。

> **迁移命名规范**：使用 `00000X_description.up.sql` + 对应的 `.down.sql`（回滚脚本）。数字前缀保证顺序执行，`golang-migrate` 靠文件名排序。

### 步骤 2：Repo 接口

```go
// internal/repo/order_repo.go
type Order struct {
    ID        uuid.UUID
    UserID    uuid.UUID
    Title     string
    Amount    float64
    Status    string
    CreatedAt time.Time
    UpdatedAt time.Time
}

type OrderRepo interface {
    Create(ctx context.Context, params *CreateOrderParams) (*Order, error)
    GetByID(ctx context.Context, id uuid.UUID) (*Order, error)
    ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*Order, error)
    CountByUser(ctx context.Context, userID uuid.UUID) (int, error)
}
```

### 步骤 3：Service 层

```go
// internal/service/order/service.go
package orderservice

type Service interface {
    Create(ctx context.Context, userID string, in *CreateInput) (*OrderResult, *errcode.Error)
    Get(ctx context.Context, userID, orderID string) (*OrderResult, *errcode.Error)
    List(ctx context.Context, userID string, page pagination.Query) (pagination.List[OrderResult], *errcode.Error)
}

type orderServiceImpl struct {
    baseservice.LogSupport
    orders repo.OrderRepo
}

type Option func(*orderServiceImpl)

func New(orders repo.OrderRepo, opts ...Option) Service {
    s := &orderServiceImpl{orders: orders}
    for _, opt := range opts {
        opt(s)
    }
    return s
}

func WithLogger(logger *zap.Logger) Option {
    return func(s *orderServiceImpl) {
        s.SetLogger(logger)
    }
}
```

### 步骤 4：Handler 层

> **关于 `mustUserID()`**：这是 `internal/http/context.go` 中的共享 helper，从 Gin Context 提取 Auth 中间件注入的 user_id，带类型断言 + 空串检查。未登录自动返回 401 并设置 false，调用方只需 `if !ok { return }`。

```go
// internal/http/order.go
type OrderHandler struct {
    svc orderservice.Service
}

func NewOrderHandler(svc orderservice.Service) *OrderHandler {
    return &OrderHandler{svc: svc}
}

func (h *OrderHandler) Create(c *gin.Context) {
    uid, ok := mustUserID(c)
    if !ok { return }

    var req struct {
        Title  string  `json:"title" binding:"required,max=200"`
        Amount float64 `json:"amount" binding:"required,gt=0"`
    }
    if err := request.Bind(c, &req); err != nil { return }

    result, svcErr := h.svc.Create(c.Request.Context(), uid, &orderservice.CreateInput{
        Title:  req.Title,
        Amount: req.Amount,
    })
    if svcErr != nil {
        resp.Fail(c, svcErr)
        return
    }
    resp.OK(c, result)
}
```

### 步骤 5：注入到 Deps 和路由

```go
// internal/http/deps.go — 新增字段
type Deps struct {
    // ...
    OrderService orderservice.Service
}

// internal/http/router.go — 注册路由
user := v1.Group("/user", deps.AuthMiddleware, deps.UserRateLimitMiddleware)
// ...
orderHandler := NewOrderHandler(deps.OrderService)
user.POST("/orders", orderHandler.Create)
user.GET("/orders", orderHandler.List)
user.GET("/orders/:id", orderHandler.Get)
```

### 步骤 6：app.go 装配

```go
// internal/app/app.go
deps := &internalhttp.Deps{
    // ...
    OrderService: orderservice.New(pgStore.Orders(),
        orderservice.WithLogger(logger),
    ),
}
```

### 步骤 7：测试

```go
// internal/service/order/service_test.go
func TestCreate_Success(t *testing.T) {
    fake := &FakeOrderRepo{}
    svc := orderservice.New(fake, orderservice.WithLogger(zap.NewNop()))

    result, err := svc.Create(ctx, userID, &orderservice.CreateInput{
        Title: "Test Order", Amount: 99.99,
    })

    if err != nil { t.Fatalf("expected nil, got %v", err) }
    if result.Title != "Test Order" { t.Fatal("title mismatch") }
}

// internal/http/router_test.go — 补充 handler 测试
// e2e/api_test.go — 补充 E2E 测试
```

### 步骤 8：文档同步

更新 `docs/api/openapi.yaml`，添加 `/api/v1/user/orders` 路径定义。CI 会自动校验。

> **日常开发工作流**：`make compose-up` 起 PG+Redis → `make migrate-up` 建表 → `make run` 启动 API → 改代码自动重启。提交前执行 `make lint && make test && make openapi-check` 确保 CI 能过。

---

## 9. 设计模式速查

本项目中用到的核心设计模式，以及它们解决的问题：

| 模式 | 项目中的应用 | 解决的问题 |
|------|----------------|----------|
| **依赖注入（DI）** | `Deps` 聚合 + `app.New()` 装配 | 层间解耦，Handler 不感知具体实现 |
| **Functional Options** | 每个 Service 的 `WithXxx()` | 可选依赖不影响构造函数签名 |
| **接口隔离** | `repo.UserRepo` / `authservice.Service` | 依赖接口而非具体类型，可替换 fake |
| **Provider + Router** | `sender` 和 `oauth` 包 | 多服务商故障转移，新增服务商不改业务代码 |
| **错误码** | `errcode.Error` (Code+MsgID+HTTP) | 统一错误格式，前端只看 code |
| **Fail-Open** | 暴力破解防护、Session Validate、限流回退 memory | Redis 故障不阻断业务 |
| **嵌入组合** | `LogSupport` 匿名嵌入 | 多个 Service 共享日志能力，零重复 |
| **装饰器** | `zap.Field` helpers（UserIDField 等） | 空值自动省略，避免无效日志 |
| **构建时注入** | `buildinfo` + ldflags | 编译时注入版本，运行时无额外开销 |
| **配置分层** | base → env → local → 环境变量 | 不同环境共享配置，敏感值不提交 |
| **契约测试** | `openapi` 包 | 代码与文档双向校验，防止漂移 |
| **分层测试** | unit → repo → handler → E2E | 从快到慢，从隔离到全链路 |

---

## 附录：关键文件索引

| 能力 | 文件 |
|------|------|
| 入口 + 优雅关闭 | `main.go` |
| 应用装配 | `internal/app/app.go` |
| 配置加载 + 校验 | `internal/config/config.go` |
| 路由构建 | `internal/http/router.go` |
| 依赖聚合 | `internal/http/deps.go` |
| 统一响应 | `pkg/resp/response.go` |
| 请求绑定 | `pkg/request/bind.go` |
| 错误码 | `pkg/errcode/errcode.go` |
| 认证服务 | `internal/service/auth/service.go` |
| 会话管理 | `internal/session/session.go` |
| 验证码下发 | `internal/sender/sender.go` |
| OAuth 校验 | `internal/oauth/oauth.go` |
| 构建信息 | `internal/buildinfo/buildinfo.go` |
| Store + 事务 | `internal/repo/pg/store.go` |
| 安全头 | `internal/http/middleware/security_headers.go` |
| 限流 | `internal/http/middleware/rate_limit.go` |
| 结构化日志字段 | `internal/http/middleware/access_log_fields.go` |
| Service 日志基础 | `internal/service/logging.go` |
| 指标采集 | `internal/metrics/metrics.go` |
| testcontainers 工具 | `internal/testutil/integration/containers.go` |
| OpenAPI 契约校验 | `internal/openapi/contract.go` |
| E2E 测试 | `e2e/api_test.go` |
| CI 流水线 | `.github/workflows/ci.yml` |
| Dockerfile | `backend/Dockerfile` |
| docker-compose | `docker-compose.yml` |
| K8s 模板 | `deploy/k8s/` |
| Makefile | `Makefile` |
