# Studiory 后端脚手架：代码评审、问题清单与演进建议

> 评审时间：2026-11-25　范围：`backend/`、`docs/`、`deploy/k8s/`、`docker-compose.yml`、`Makefile`、`.github/workflows/`
> 方法：通读全部手写代码（排除 `sqlc/gen`）与文档、CI、部署清单；**本机未安装 Go，未执行 `go build` / `go test` / `make lint`**，以下结论均来自静态阅读，涉及行为的部分已标注依据。
> 复核（2026-11-25，第二批改动后）：见下方「零、复核结果」。结论先行——**19 条中仅 P1-2 已修复，P1-1/P1-3~P1-6 与 P2-1~P2-13 全部未动**；本批改动质量高，但引入了 1 处约定违反与 2 处低危口径偏差。

---

## 零、复核结果（第二批改动后）

### 0.1 复核范围与方法

- 复核基线为当前 HEAD `39523b7`；`git status` 干净，**无未提交改动**，评审后只有 `39523b7 fix(auth): 手机/邮箱标识统一归一化…` 一个提交改动代码（`git log` 可证）。
- 本机仍无 `go` / `sqlc` / `docker`（已确认），因此本批复核同样是静态阅读，未编译、未跑测试；新增测试的断言逻辑已逐段阅读。
- 判定口径：**已修复** = 问题描述的危害在所有写入/读取路径上消失且有测试兜底；**部分修复** = 主路径改好但仍有旁路或口径分裂；**未动** = 现状与首轮描述一致。

### 0.2 逐条判定

| 编号 | 结论 | 依据（当前代码现状） |
|------|------|----------------------|
| P1-1 | **未动** | `repo/mysql`、`migrations/mysql/000001`、compose `mysql`/`api-mysql` profile、Makefile `DB_DRIVER` 分支、CI MySQL job 全部存在；[README.md](file:///Users/pxiong/Code/personal/Studiory/README.md#L11) 仍只写“PostgreSQL 迁移”，`docs/*.md` 全文 0 处 `mysql`，[env-vars.md](file:///Users/pxiong/Code/personal/Studiory/docs/env-vars.md#L33) 仍无 `DB_DRIVER`；[sqlc.yaml](file:///Users/pxiong/Code/personal/Studiory/backend/sqlc.yaml#L1-L10) 仍只有 postgresql 一组 |
| P1-2 | **已修复** | 归一化落在 [strutil.go](file:///Users/pxiong/Code/personal/Studiory/backend/pkg/strutil/strutil.go#L69-L118)，覆盖 `SendCode`/三个验证码键/`loginWithCode`/`findUserByAccount`/`loginAccountID`/`RegisterInput.Canonicalize`/`checkAccountExists`/`cmd/seed`/`SendCodeReq.Validate`；[000005](file:///Users/pxiong/Code/personal/Studiory/backend/migrations/000005_identity_canonical.up.sql) 清洗历史邮箱并建 `lower(email)` 部分唯一索引；三张单测表 + `canonical_test.go` + PG/MySQL 集成断言齐备。另确认 `UpdateProfile` 只改昵称/头像（[user/service.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/service/user/service.go#L28-L33)），不存在绕过归一化改邮箱/手机号的旁路，覆盖面完整 |
| P1-3 | **未动** | [token.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/service/auth/token.go#L57-L104) 的 `Refresh` 仍只校验+签发，无 `Expire` 续期；[session.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/session/session.go#L114-L140) 的“session 键兜底”分支仍在，集合续期副作用依旧隐式 |
| P1-4 | **未动** | [base.yaml](file:///Users/pxiong/Code/personal/Studiory/backend/config/base.yaml#L45) 仍 `access_ttl: 2h`；[config.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/config/config.go#L220-L328) 的 `Validate` 仍无 access/refresh TTL 关系校验 |
| P1-5 | **未动** | [router.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/http/router.go#L32-L64) 中间件链无 `Timeout`；[postgres.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/store/postgres.go#L16-L51) 未设 `statement_timeout`/`query_timeout`；[redis.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/store/redis.go#L13-L33) 未显式设读写超时（修正一处：go-redis 默认 3s 读写、5s dial，Redis 侧实际有兜底，**PG 语句超时才是真缺口**） |
| P1-6 | **未动** | [rate_limit.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/http/middleware/rate_limit.go#L96-L112) 仍是 `NewStoreWithOptions` 失败即 `memory.NewStore()`，无日志、无指标 |
| P2-1 | **未动** | [auth/service.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/service/auth/service.go#L26-L34) 等接口仍返回 `*errcode.Error` |
| P2-2 | **未动** | [logging.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/service/logging.go#L17-L23) 的 `LogInternal` 无 `ctx` 参数，日志无 `request_id` |
| P2-3 | **未动** | 仅有 `go.mod` 里 otel 的 **indirect** 依赖（[go.mod](file:///Users/pxiong/Code/personal/Studiory/backend/go.mod#L116-L120)），无 `TracerProvider`/gin 中间件/pgx tracer 初始化代码 |
| P2-4 | **未动** | [admin/service.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/service/admin/service.go#L145-L217) 仍只有“不能改/禁自己”，无“最后一个 admin”统计；`migrations/` 与 `internal/` 全文无 `audit` |
| P2-5 | **未动** | [cors.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/http/middleware/cors.go#L16-L34) `ExposeHeaders` 仍只有 `Content-Length`（`X-Request-Id` 已由 requestid 写入响应头，但浏览器读不到，排障体验未改善）；`Validate` 仍不拒绝 `allow_origins: ["*"]` + `allow_credentials: true`；L18-L20 的 localhost 兜底仍是死代码（[config.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/config/config.go#L324-L326) 已禁止空 origins） |
| P2-6 | **未动** | `Validate` 仍只校验 server/限流/origins 非空与生产项，未加 `access_ttl < refresh_ttl`、`key_prefix` 非空、CORS 组合校验 |
| P2-7 | **未动** | [app.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/app/app.go#L218-L225) 的 `postgresReadyCheck` 仍无生产调用（L173-L176 用的是 `databaseReadyCheck`），仅被 `app_test.go:22` 引用 |
| P2-8 | **未动** | [health.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/http/health.go#L26-L40) 仍串行 `for` + 共享一个 2s ctx，一个慢探针会拖垮全部 |
| P2-9 | **未动** | [architecture.md](file:///Users/pxiong/Code/personal/Studiory/docs/architecture.md#L220-L222) 仍列 `frontend/`、`apps/node/`、`apps/python/`；L66-L67 示例仍用 `000002_orders`，与实际的 `000002_example_todos` 冲突 |
| P2-10 | **未动** | [deps.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/http/deps.go#L26-L52) `Deps` 字段未变 |
| P2-11 | **未动** | [middleware/auth.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/http/middleware/auth.go#L54-L92) 仍是 `GET revoke` + `session.Validate`（`SIsMember` + 兜底 `GET`），最坏 3 次往返 |
| P2-12 | **未动** | PG [users.sql](file:///Users/pxiong/Code/personal/Studiory/backend/internal/repo/sqlc/query/users.sql#L57-L70) 仍 `ILIKE '%kw%'`，MySQL [store.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/repo/mysql/store.go#L114-L131) 仍 `LIKE '%kw%'`；两边都是 `LIMIT/OFFSET` 深翻页 |
| P2-13 | **未动** | `oauth.wechat.app_secret` 仍只被读取映射（[config.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/config/config.go#L180)），[env-vars.md](file:///Users/pxiong/Code/personal/Studiory/docs/env-vars.md#L116) 标注“预留”，无代码消费 |

**汇总**：已修复 1 / 部分修复 0 / 未动 18。第二批改动实际只覆盖了 P1-2。

### 0.3 这批改动是否引入新问题 / 破坏原有约定

| # | 判定 | 说明 |
|---|------|------|
| N-1 | **违反既有约定（应修）** | [architecture.md](file:///Users/pxiong/Code/personal/Studiory/docs/architecture.md#L70) 明确要求“执行 `migrate up` 后，把新文件加入 `backend/sqlc.yaml` 的 schema 列表”，但 `000005` **没有登记**。当前无功能影响（只含数据清洗+表达式索引，不改表结构，`sqlc generate` 不会产出 diff），但同一次提交就漏掉了这条规则，说明该约定目前只靠人记；下次迁移若含列变更会被静默漏检。修复成本 1 行 |
| N-2 | 新引入的口径不一致（低） | `SendCode` 现在接受 `+86 138-0013-8000`（[auth.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/http/auth.go#L76-L93)），但 `RegisterReq.Phone` / `LoginReq.Phone` 仍绑 `phone_cn`（[auth.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/http/auth.go#L40-L45)、[rules.go](file:///Users/pxiong/Code/personal/Studiory/backend/pkg/validator/rules.go#L11)）只接受裸 11 位 → 同一个号码“能收码、不能注册/登录”。建议二者取齐：要么绑定前先归一化，要么 send-code 也只收裸号 |
| N-3 | 迁移覆盖不全（低-中） | `000005` 只清洗 PG 的 `email`，不清洗 `phone`。当前注册/登录 binding 一直是 `phone_cn`，线上应无脏号；风险在历史 `cmd/seed`（本批才加归一化，此前 `-phone` 原样入库）或直连 SQL 写入的 `+86…`/`86138…` 行——这些行会“查不到、还能再注册一个”。建议上线前跑一次 `SELECT count(*) FROM users WHERE phone !~ '^1[3-9][0-9]{9}$'` 核对 |
| N-4 | 部署说明缺失（低） | `000005` 的 `UPDATE users SET email = lower(btrim(email))` 在历史重复邮箱上会因 `users_email_key` 冲突**刻意中止迁移**（SQL 注释已说明），但 `docs/deploy.md` 没有对应处置说明，滚动发布会表现为“新 Pod 起不来”。建议补一句：先查重、人工合并、再发布 |
| N-5 | 理论边界（可忽略） | `lower(email)` 在 PG 由数据库 locale 决定，Go 的 `strings.ToLower` 走 Unicode 规则；极端非 ASCII（如土耳其语 `İ`）在特定 `lc_ctype` 下清洗结果可能与写入形式不一致。实际邮箱基本都是 ASCII，建议在迁移注释里限定 ASCII 即可 |
| N-6 | 未破坏分层/契约 | 归一化放在 `pkg/strutil` + service 入口，repo/sqlc 查询未改、handler 未写 SQL；OpenAPI 未新增必填或格式约束，契约测试不受影响；migration 编号 `000005` 单调，未与既有文件冲突 |

### 0.4 复核后的下一步（只列差异）

1. 补 `000005` 到 `sqlc.yaml`；顺手统一 `send-code` 与 `register/login` 的手机号入参口径（N-1、N-2）。
2. 阶段 0 剩余项：`/ready` 并发化与死代码清理（P2-7、P2-8）、CORS 收口（P2-5）；TTL 校验（P1-4）、请求/语句超时（P1-5）、限流回退日志（P1-6）已在第三批完成（见 0.5，MySQL 语句超时除外）。
3. P1-1 的“留或删 MySQL”仍需拍板——本批又给 MySQL 增加了一条集成测试（值得肯定），但双栈治理债因此更实，建议尽快定调。

### 0.5 第三批改动（P1-3 ~ P1-6）

- **P1-3 已修复**：明确采用**绝对过期**（[session.md](file:///Users/pxiong/Code/personal/Studiory/docs/session.md) 新增“有效期语义”）；`Refresh` 刻意不续期；`Validate` 只认 `session:{sid}` 键（存在且 userID 匹配），`sessions:uid` 退化为 `RevokeAll` 的枚举索引——集合续期不再复活过期 sid。新增 `TestStore_SessionExpiresAbsolutely`。
- **P1-4 已修复**：`jwt.access_ttl` 默认 `2h → 30m`；`Config.Validate` 增加 `access_ttl > 0`、`refresh_ttl > 0`、`access_ttl < refresh_ttl`，并补齐成功/失败用例。
- **P1-5 部分修复**：新增 `middleware.Timeout`（`app.request_timeout`，ctx 透传 DB/Redis，超时统一 503）；PG 连接下发 `statement_timeout`；Redis 显式读写超时。**MySQL 侧语句超时未实现，随 P1-1 双栈治理一并处理**。
- **P1-6 已修复**：限流 store 降级改为 `logger.Error` + `rate_limit_store_fallback_total{scope}`（metrics 关闭时仍记日志），并提供 `TestRateLimit_RedisUnavailableFallsBackWithObservation`。
- 本批同样**未编译、未跑测试**（本机无 Go）。上方 0.2 表格是第二批的时点快照，其中 P1-3~P1-6 的“未动”结论已被本节取代。

---

## 一、总体结论

这是一套**明显高于市面平均水平**的 Go API 脚手架：分层落地扎实，安全细节做过专门打磨，CI 门禁不是摆设。核心骨架可以直接承载真实业务。

**值得保留的强项**

| 维度 | 具体表现 |
|------|----------|
| 分层 | handler 只做绑定/调用/返回；service 编排 repo；repo 接口与 `sqlcgen` 解耦；未发现 handler 直连 SQL |
| 安全 | 可信代理收紧（[router.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/http/router.go#L31-L41)）、生产 fail-closed（[config.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/config/config.go#L265-L320)）、canonical UUID 吊销键、refresh 黑名单 TTL 跟随票据寿命、验证码配额/冷却用 Lua 原子化 |
| 契约 | OpenAPI 与路由**双向**比对（[contract.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/openapi/contract.go#L74-L92)），CI 强制生成物 drift 检查 |
| 测试 | 非测试代码 9,032 行 / 测试 10,803 行；单测 + testcontainers 集成 + HTTP E2E 三层，CI 覆盖率门槛 50% |
| 可观测 | 自建 Registry（不污染全局）、路由模板做 label 防基数爆炸、method 收敛为 OTHER |
| 交付 | 多阶段构建 + 非 root + 只读根文件系统、K8s 探针/PDB/preStop/优雅停机对齐、Docker 镜像冒烟 job |

**结构性风险（不是"有 bug"，而是"会随时间腐化"）**

1. **双数据库驱动已经事实上存在，但只有一套治理流程**——文档完全不提 MySQL。
2. **会话/令牌的有效期语义不一致**，刷新路径没有续期，行为与文档描述有偏差。
3. **Service 层直接返回 `*errcode.Error`**，把 HTTP 语义固化进业务层，后续做重试、编排、复用会卡住。

---

## 二、文档约定 vs 实际落实

| `docs/architecture.md` 的约定 | 落实情况 |
|------|------|
| HTTP 层只做绑定/调用/返回，不写 SQL | ✅ 全部 handler 符合，包括 admin/todo |
| Service 依赖 repo 接口，不依赖 `sqlcgen` | ✅ 仅 `repo/pg` 引用 `sqlcgen` |
| Repo 接口 + `pg` 实现 + `sqlc` 生成 | ✅ PG 侧完整；⚠️ MySQL 侧绕开 sqlc 手写 SQL |
| 新增模块六步（migration → sqlc → repo → service → app → http） | ✅ Todo / Admin 两个示例完整走通 |
| 技术栈 PostgreSQL | ❌ 代码/compose/CI 支持 MySQL，README 与全部 docs 零提及 |
| 中间件顺序 | ✅ 与文档一致，且 CORS 先于限流的理由在代码里有落地 |
| `frontend/`、`apps/node`、`apps/python` 子服务 | ❌ 仓库中不存在，文档以"可创建"措辞保留，但容易误导新人 |
| 配置项清单 | ✅ 基本齐全；⚠️ `docs/env-vars.md` 缺 `DB_DRIVER` 与 MySQL 相关说明 |

**判断**：文档承诺的"分层不泄漏"是真实兑现的，这是本仓库最大的资产；漂移集中在**数据库栈**和**目录规划**两处。

---

## 三、问题清单（按严重度）

> P0 = 可直接导致数据/权限事故；P1 = 明确的行为缺陷或运维风险；P2 = 设计债与体验问题。本次**未发现 P0 级漏洞**。

### P1-1 双数据库驱动缺少治理：支持 MySQL，但文档、迁移、SQL 全部半套

- 代码里 `cfg.DBDriver == "mysql"` 分支完整存在（[app.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/app/app.go#L60-L90)），并有 250 行左右的手写 SQL repo（[store.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/repo/mysql/store.go)）与独立的 MySQL 集成测试。
- 迁移只有一份压缩版 [000001](file:///Users/pxiong/Code/personal/Studiory/backend/migrations/mysql/000001_init_schema.up.sql)，把 PG 的 000001/000002/000003/000004 合并进去了。下次加业务表时，**必须同时改两套迁移**，而流程、CI 校验、文档都没有约束这件事。
- `sqlc.yaml` 只声明 `engine: postgresql`（[sqlc.yaml](file:///Users/pxiong/Code/personal/Studiory/backend/sqlc.yaml)），MySQL 侧是第二份手写 SQL，等于**同一查询维护两遍**，且 MySQL 侧 `List`/`Count` 用 `(?='' OR phone LIKE ?)` 这种写法，PG 侧用 sqlc 参数化，两边索引利用方式也不同。
- README 第 1 行写"技术栈固定"，[README.md](file:///Users/pxiong/Code/personal/Studiory/README.md#L21-L31) 只有 PG；但 compose 有 `mysql` profile（[docker-compose.yml](file:///Users/pxiong/Code/personal/Studiory/docker-compose.yml#L74-L127)），Makefile 有 `DB_DRIVER=mysql` 分支（[Makefile](file:///Users/pxiong/Code/personal/Studiory/Makefile#L6-L9)），CI 有 MySQL 集成 job（[ci.yml](file:///Users/pxiong/Code/personal/Studiory/.github/workflows/ci.yml#L120-L122)）。

**影响**：新同学按 README 只会在 PG 上开发；一旦有项目选 MySQL，迁移脚本、索引、时间语义（`DATETIME(6)` vs `TIMESTAMPTZ`）都会靠人肉记忆维护。

**建议（二选一，不要悬空）**

- **方案 A（推荐，除非确有多云/存量 MySQL 需求）**：删除 MySQL 栈。收益：少 1,100 行重复实现、少一套迁移、少一个 CI job，脚手架更"固定"。删除成本低——只涉及 `repo/mysql`、`migrations/mysql`、compose 两个 service、Makefile 分支、CI 一个 job、`config` 的 driver 分支。
- **方案 B**：正式把 MySQL 做成一等公民。要求：
  1. `sqlc.yaml` 增加 `engine: mysql` 的第二组配置，生成 `sqlc/genmysql`；
  2. MySQL 迁移与 PG 一一对应（编号相同、语义相同），CI 增加"两套迁移表结构 diff"检查（可用 `atlas schema diff` 或 `schemadiff`）；
  3. README/docs 增加"选择数据库"章节，明确时间类型、排序规则（collation）、唯一约束的差异；
  4. 新增模块 checklist 加一步"同步两份 migration + 两套 sqlc"。

---

### P1-2 邮箱/手机号未规范化，PG 与 MySQL 行为不一致

注册写入时 `Phone`/`Email` 原样落库（[register.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/service/auth/register.go#L51-L90)），登录查询做精确匹配（[login.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/service/auth/login.go#L262-L296)）。但：

- PG 的 `email VARCHAR(255) UNIQUE` 是**大小写敏感**（[000001](file:///Users/pxiong/Code/personal/Studiory/backend/migrations/000001_init_schema.up.sql#L5-L14)）；
- MySQL 的 `utf8mb4_unicode_ci` 是**大小写不敏感**（[mysql 000001](file:///Users/pxiong/Code/personal/Studiory/backend/migrations/mysql/000001_init_schema.up.sql#L1-L14)）。

**后果**：PG 上 `A@x.com` 与 `a@x.com` 可注册成两个账号，用户换大小写就登录失败；MySQL 上则报 1062。同类问题还有手机号：`13800138000` 与 `+8613800138000` 视为不同账号。

**修复**：在 HTTP/service 边界统一 canonical 化（email → `strings.ToLower(strings.TrimSpace())`；手机号 → E.164 或纯 11 位），并在 PG 加 `CREATE UNIQUE INDEX ... ON users (lower(email)) WHERE email IS NOT NULL`；`strutil` 里加 `NormalizeEmail` / `NormalizePhone` 单测。当前 `loginAccountID` 已经 lower 了账号用于防爆破计数（[brute_force.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/service/auth/brute_force.go#L93-L103)），说明"应该统一"这件事已被认可，只是没落到存储层。

**状态**：已修复。归一化原语落在 [strutil.go](file:///Users/pxiong/Code/personal/Studiory/backend/pkg/strutil/strutil.go)（`NormalizeEmail` / `NormalizePhone` / `NormalizeAccount`，均幂等，邮箱不剥 `+tag`、手机号仅在剩余部分匹配 `1[3-9]\d{9}` 时才剥 `0086`/`86`）；`internal/service/auth` 的五个入口统一过它——`SendCode` 与三个验证码键构造函数（[sendcode.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/service/auth/sendcode.go#L258-L290)）、`loginWithCode` + `findUserByAccount`（[login.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/service/auth/login.go#L67-L69)）、`loginAccountID`（[brute_force.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/service/auth/brute_force.go#L95-L108)）、`RegisterInput.Canonicalize` + `checkAccountExists`（[register.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/service/auth/register.go#L19-L23)）；`cmd/seed` 与 `SendCodeReq.Validate` 同口径。存储层由 [000005_identity_canonical](file:///Users/pxiong/Code/personal/Studiory/backend/migrations/000005_identity_canonical.up.sql) 清洗历史行 + `lower(email)` 表达式唯一索引兜底（不引入 `citext`，托管 PG 常拿不到 `CREATE EXTENSION` 权限），MySQL 侧 `utf8mb4_unicode_ci` 本就等价、无需迁移。测试：`strutil_test.go` 三张用例表、`canonical_test.go`（变体共用冷却/配额、大小写不产生重复账号、失败计数同桶触发锁定）、PG/MySQL 集成测试各一条驱动语义对齐断言。

**复核（2026-11-25）**：判定维持“已修复”，且 `UpdateProfile` 不含邮箱/手机号字段，确认无旁路。本批残留的口径问题（send-code 与 register/login 入参宽严不一、phone 历史数据未清洗）见 0.3 的 N-2、N-3。

---

### P1-3 会话有效期与 Refresh Token 有效期语义不一致

- 会话键 TTL 取 refresh TTL（[app.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/app/app.go#L99)）。
- `Refresh` 保持同一 `sid` 并签发新的 refresh token（[token.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/service/auth/token.go#L50-L104)），但**没有续期 Redis 里的 session 键与 `sessions:uid:*` 集合**。
- 多设备模式下每次 `Register` 都会 `Expire(setKey, sessionTTL)`（[session.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/session/session.go#L69-L76)），这会**顺带延长所有旧 sid 的成员有效性**；而集合成员本身没有独立过期时间。

**可观察到的错误行为**：用户第 6 天刷新拿到"还有 7 天"的新 refresh token，但第 7 天 session 到期 → 刷新被拒，被迫重新登录；反之，过期很久的旧 sid 因为集合被后续登录续期而仍能通过 `Validate`。

**修复方向**：明确一种模型并写进 `docs/session.md`：
- 绝对过期（推荐）：session 写入 `expire_at`，刷新时校验但不延长；文档写明"最长 7 天必须重新登录"；
- 滑动过期：刷新时 `Expire` 续期 session 键与集合，且集合成员按 sid 单独设 TTL（例如 `session:{sid}` 键 + `SADD` 后定期清理，或改用 `ZADD` 存过期时间戳）；
- 顺手把 `Validate` 的"兜底分支"（[session.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/session/session.go#L117-L121)）去掉或注释清楚——它现在让"集合续期"的副作用变成隐式行为。

---

### P1-4 Access Token 默认 2 小时，权限回收完全依赖 Redis 吊销

[base.yaml](file:///Users/pxiong/Code/personal/Studiory/backend/config/base.yaml#L33-L35) 里 `access_ttl: 2h`。这意味着改密/降权/禁用之后，**只要吊销键写入失败或 Redis 短暂不可用（fail-open 开发默认）**，旧 token 最长还能活 2 小时。当前代码已经把"吊销是否成功"透出为 `sessions_revoked`，这是对的；但默认 TTL 偏大放大了这个窗口。

**建议**：access TTL 降到 15–30 分钟（refresh 保持 7–30 天），并在 `Config.Validate()` 增加 `access_ttl < refresh_ttl`、`access_ttl <= 1h`（warn 即可）的校验。README 快速开始里的 curl 流程不受影响。

---

### P1-5 缺少请求级超时与数据库语句超时

- 路由全局中间件里没有 `context.WithTimeout`（[router.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/http/router.go#L43-L62)）；
- pgx 连接池未设置 `statement_timeout` / `query_timeout`（[postgres.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/store/postgres.go#L20-L46) 只设置了池大小与生命周期）。

`WriteTimeout=30s` 只截断响应，**不会释放**还在执行的 DB 查询；一次慢查询（缺索引、锁等待）就能长期占住连接，`max_conns=20` 很快被打满，表现为全站 503。

**修复**：
1. 加 `middleware.Timeout(5s)`：`c.Request = c.Request.WithContext(ctx)`，超时返回统一 503 信封；
2. DSN 追加 `statement_timeout=3000`（PG）/ `readTimeout,writeTimeout`（MySQL），或在 `NewPostgres` 里用 `AfterConnect` 执行 `SET statement_timeout`；
3. Redis 侧设置 `ReadTimeout/WriteTimeout`（`store.NewRedis` 目前只设了 `PoolSize`）。

---

### P1-6 限流存储失败时静默降级为进程内内存

[rate_limit.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/http/middleware/rate_limit.go#L96-L112)：`limiteredis.NewStoreWithOptions` 失败就 `memory.NewStore()`，**没有任何日志**。多副本部署下每个 Pod 各算一份配额，实际阈值变成 `N × per_minute`，而运维看不到任何异常。

**修复**：降级时 `logger.Error` 并打点（`rate_limit_store_fallback_total`）；生产环境建议直接返回启动错误（fail fast），或显式配置 `rate_limit.allow_memory_fallback: false`。

---

### P2 级问题（设计债 / 体验）

| 编号 | 问题 | 位置 | 建议 |
|------|------|------|------|
| P2-1 | Service 接口返回 `*errcode.Error`，业务层绑定 HTTP 错误码；无法 `errors.Is`、无法携带 cause，且存在 typed-nil 风险（`nil *errcode.Error` 传给 `resp.Fail` 会 panic） | [service.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/service/auth/service.go#L27-L34)、user/admin service 同理 | 引入 `internal/domainerr`（或直接返回 `error` + sentinel），HTTP 层统一 `errcode.From(err)` 映射；repo 已经用 sentinel（`repo.ErrNotFound`），两套错误模型应统一 |
| P2-2 | service 日志不带 `request_id`，与访问日志无法串联 | [logging.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/service/logging.go#L9-L24) | `LogSupport` 增加 `WithContext(ctx)` 取 context 里的 logger；`RequestID` 中间件把 `request_id` 注入 context |
| P2-3 | 无分布式追踪 | 全局 | 接 OTel：gin 中间件 + pgx `QueryTracer` + `redis.Hook`，至少打通 trace_id 与日志 |
| P2-4 | 缺少"最后一个 admin"保护与后台审计 | [admin/service.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/service/admin/service.go#L145-L215) | 降权/禁用前统计 `role=admin AND status=active` 数量；增加 `admin_audit_logs` 表（actor/target/action/before/after/ip） |
| P2-5 | CORS 兜底是死代码；`*` + credentials 未校验；`X-Request-Id` 未暴露 | [cors.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/http/middleware/cors.go#L17-L32) | 删除默认 localhost 兜底（`Validate` 已强制非空）；`ExposeHeaders` 加 `X-Request-Id`；`Validate` 拒绝 `*` 与 `allow_credentials=true` 组合 |
| P2-6 | 配置校验缺 TTL / Redis 超时 / `key_prefix` 约束 | [config.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/config/config.go#L220-L328) | 补 `access_ttl > 0`、`refresh_ttl > access_ttl`、`redis.key_prefix` 非空且不含 `:` |
| P2-7 | 死代码 `postgresReadyCheck` 仅被测试引用 | [app.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/app/app.go#L218-L225) | 删除，或改用 `pool.Ping` 直接构造 `ReadyCheck` |
| P2-8 | `/ready` 串行检查且共享 2s context，DB 卡住时 Redis 根本没被探测 | [health.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/http/health.go#L22-L40) | `errgroup.WithContext` 并发 + 每项独立超时；响应体带上每个依赖的结果 |
| P2-9 | 文档漂移：`frontend/`、`apps/node`、`apps/python` 不存在；示例 migration 编号 `000002` 已被占用 | [architecture.md](file:///Users/pxiong/Code/personal/Studiory/docs/architecture.md#L218-L233)、[architecture.md](file:///Users/pxiong/Code/personal/Studiory/docs/architecture.md#L64-L70) | 改成"若需要则创建"的语气并注明当前不存在；示例编号改 `000005_orders` |
| P2-10 | `Deps` 是上帝结构体：每加一个模块要改 5 处（Deps 字段、validateDeps、app 装配、router 挂载、docs） | [deps.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/http/deps.go)、[router.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/http/router.go#L89-L118) | 模块化注册：`type Module interface{ Register(*gin.RouterGroup) }`，`app.New` 收集 `[]Module`，`validateDeps` 退化为启动时逐个 `module.Init()` 报错 |
| P2-11 | 每个已鉴权请求 2 次 Redis 往返（revoke + session） | [auth.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/http/middleware/auth.go#L46-L54) | 用 pipeline/MGET 合并；或把 revoke 时间戳缓存 5–10s（接受极小延迟） |
| P2-12 | 后台列表 `keyword LIKE '%x%'` 全表扫描，且 `Page` 很大时 OFFSET 深翻 | [mysql store.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/repo/mysql/store.go#L109-L133) | 明确列表规模上限；若增长，改成前缀匹配 + 游标分页（`created_at,id`） |
| P2-13 | `oauth.wechat.app_secret` 配置存在但代码未使用（当前只校验客户端传入的 access_token） | [config.go](file:///Users/pxiong/Code/personal/Studiory/backend/internal/config/config.go#L95-L101) | 要么实现 code→token 流程，要么删字段并在文档标注"预留" |

---

## 四、优化方向（按主题，含优先级）

### A. 数据层：先做减法，再谈扩展
- **决策点**：MySQL 留或删（见 P1-1）。这是当前唯一会持续产生双倍维护成本的决策，越早定越好。
- 迁移治理：引入 `squawk`/`atlas lint` 检查破坏性 DDL；给 `migrate down` 加 CI 冒烟（现在有 down 脚本但没人执行过）。
- sqlc 之外再补一层"查询必须有索引"的 review 约定；`todos(user_id, created_at)` 已经做对，可作为范式。

### B. 错误模型：把 HTTP 语义从 service 拿出去
- 新增 `internal/domainerr`：`NotFound/Wrapf/...` + `errors.Is/As`。
- `pkg/errcode` 只保留在 HTTP 边界做映射；`resp.Fail` 增加 `err == nil` 防护。
- 收益：service 可被 gRPC/CLI/worker 复用；日志能保留 cause；测试不再断言错误码常量。

### C. 会话与令牌：把语义写死并测试
- 按 P1-3 选定绝对/滑动模型，用表驱动测试覆盖"登录→刷新→过期→登出"四态。
- Refresh Token Rotation + 复用检测：在 Redis 存 `refresh_jti -> sid`，旧 jti 被二次使用则整条会话家族吊销（现在的单 token 黑名单已能防重放，但无法识别"家族被盗"）。
- 预留 `kid` 支持，为密钥轮换铺路（现在是单一 HS256 secret）。

### D. 可观测性：从"能看"到"能定位"
1. **trace**：OTel + gin + pgx + redis hook，采样率可配；
2. **日志**：service 日志注入 `request_id`/`trace_id`/`user_id`，统一字段名（现在 `user_id` 有了，缺前两个）；
3. **指标**：补 `db_pool_acquired_conns` 告警（collector 已有，缺文档与告警规则）、`auth_login_fail_total{reason}`、`rate_limit_rejected_total`；
4. **告警**：`docs/metrics.md` 增加 3 条起步规则（5xx 比例、P99、ready 失败）。

### E. 性能与稳定性
- 请求超时 + DB statement_timeout（P1-5）优先于任何缓存优化——先保证"慢"不会变成"挂"。
- Docker/compose 给 `api` 加 healthcheck；K8s 补 `seccompProfile: RuntimeDefault`、`automountServiceAccountToken: false`、NetworkPolicy 示例。
- 压测基线：用 `k6` 跑 login/refresh/todo 三条链路，把结果写进 docs，作为后续回归门槛。

### F. 安全加固（在现有良好基础上补口）
1. 邮箱/手机号 canonical 化 + PG `lower(email)` 唯一索引（P1-2）；
2. 管理员"最后一人"保护 + 审计表（P2-4）；
3. 登录响应时间恒定化：现在"用户不存在"走 `recordLoginFail` 后立即返回，密码校验分支多一次 bcrypt（~60ms），存在**时序侧信道**可用于枚举账号；建议不存在时也执行一次 dummy bcrypt；
4. 验证码尝试次数：当前只有账号级锁定（5 次/10 分钟），建议再按 `code_key` 维度限制（同一验证码最多错 3 次即作废）。

> 第 3 点值得单独说明：代码注释里写了"防止通过响应时间枚举账号"，但实际只做了"失败也计数"，没有做等时处理——这是**注释与实现不一致**，属于会误导后续维护者的隐患。

### G. 工程化
- CI 增加：`go vet` 全量、每包覆盖率报告、`gofumpt`（可选）、`openapi` 客户端生成漂移检查。
- 用 `oapi-codegen` 从 `docs/api/openapi.yaml` 生成请求/响应类型，替换手写 DTO，彻底消灭"文档对、类型不对"的漂移（现在契约测试只比对路径与方法，不校验 schema）。
- 增加 `make new-module NAME=order` 脚手架脚本，自动生成 migration/sql/repo/service/http 五件套并在 `router.go` 注册——这是把文档承诺变成可执行约束的最有效手段。

---

## 五、后续开发建议（路线图）

### 阶段 0：一周内，低风险高收益
1. README + docs 补 MySQL 说明，或直接删除 MySQL 栈（决策先行）。
2. 邮箱/手机号规范化 + PG 唯一索引 migration（`000005`）。
3. `Config.Validate` 补 TTL/Redis/`key_prefix`/CORS `*` 校验。
4. 请求超时中间件 + DSN `statement_timeout` + Redis 读写超时。
5. 限流内存回退加日志/指标；删除 `postgresReadyCheck`、CORS 死分支。
6. 修正"防响应时间枚举"的实现（dummy bcrypt）或删掉该注释。

### 阶段 1：2–4 周，语义与可观测性
1. 会话模型定稿（绝对/滑动）+ refresh rotation 家族检测 + 测试矩阵。
2. access TTL 降到 30 分钟，文档同步。
3. OTel 全链路 + service 日志带 `request_id`。
4. `/ready` 并发检查；K8s 安全上下文补齐。

### 阶段 2：结构性演进
1. `domainerr` 错误模型改造（一次改一个模块，auth → user → admin）。
2. 模块注册机制替换 `Deps` 上帝结构体。
3. `oapi-codegen` 生成 DTO + `make new-module`。
4. 审计表 + 最后一个 admin 保护 + 基础告警规则。

### 新模块 checklist 建议修订（替换 architecture.md 第六节）
1. 决定归属库（PG only 则跳过 MySQL 双写）；
2. migration（含 down + 索引 + 约束）+ 在 `sqlc.yaml` 注册 schema；
3. 查询进 `sqlc/query`，`sqlc generate`，CI drift 通过；
4. `repo` 接口 + `pg` 实现 + `Store` 暴露；
5. `service` 返回 `domainerr`，不返回 `errcode`；
6. handler 只绑定/调用/返回，路由注册到既有分组；
7. OpenAPI 增补路径与 schema（契约测试会强制）；
8. 至少 1 个 service 单测 + 1 条 repo 集成测试 + 1 条 E2E happy path。

### 何时应该停止扩展这个脚手架
出现以下任一信号，说明单体脚手架已到边界，应拆分：单表超千万行需要分库、需要跨服务事务、需要独立扩缩容的算力模块（`apps/python` 场景）、需要多团队并行开发同一 `Deps`。在此之前，继续单仓分层是成本最低的选择。

---

## 六、快速收益清单（按投入产出排序）

| # | 动作 | 涉及文件 | 预期收益 |
|---|------|----------|----------|
| 1 | 删 MySQL 或补文档+双迁移流程 | README、docs、repo/mysql | 消除最大维护歧义 |
| 2 | email/phone 规范化 + `lower(email)` 唯一索引 | register.go、`000005` migration | 消除重复账号与登录失败 |
| 3 | 请求超时 + statement_timeout | router.go、store/postgres.go | 防止慢查询拖垮全站 |
| 4 | access TTL 30min + 校验规则 | base.yaml、config.go | 缩短权限回收窗口 |
| 5 | 限流回退日志/指标 | rate_limit.go | 让隐性失效可见 |
| 6 | service 日志带 request_id | logging.go、middleware/request_id.go | 排障效率翻倍 |
| 7 | dummy bcrypt 修时序侧信道 | login.go | 兑现注释承诺 |
| 8 | 删除死代码与 CORS 死分支 | app.go、cors.go | 减少误导 |
| 9 | 文档纠正 frontend/apps 与 migration 编号 | architecture.md | 新人少踩坑 |
| 10 | `make new-module` 脚本 | Makefile、scripts/ | 把分层约定变成约束 |

---

## 附：本次未能验证的部分
- 未执行编译、单测、race、lint、govulncheck、集成/E2E（环境无 `go` 命令）。若需要，可在安装 Go 1.26 后执行 `make cover`、`make lint`、`make test-integration` 复核。
- 未做运行时压测与真实 PG/MySQL 行为对比；P1-2 的 MySQL 大小写行为基于 DDL 排序规则推断，建议用一条集成测试固化（`insert a@x.com` 后 `get A@x.com`）。
