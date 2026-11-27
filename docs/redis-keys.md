# Redis 键与用途说明

脚手架后端使用 Redis 做验证码、Token 黑名单等。默认键前缀为 `app:`，可通过 `REDIS_KEY_PREFIX` 或 `config/*.yaml` 的 `redis.key_prefix` 修改。

---

## 1. 验证码（登录/注册）

| 键格式 | 类型 | TTL | 说明 |
|--------|------|-----|------|
| `app:sms:{phone}` | string | 5 分钟 | 手机号对应的短信验证码 |
| `app:email:{email}` | string | 5 分钟 | 邮箱对应的邮件验证码 |
| `app:sms:cooldown:{phone}` | string | 60 秒 | 短信发送冷却（同一号码 60 秒内不可重发） |
| `app:email:cooldown:{email}` | string | 60 秒 | 邮件发送冷却 |
| `app:sms:quota:{phone}` / `app:email:quota:{email}` | string（计数） | 滚动 24 小时 | 按目标的下发次数配额（默认 10 次），首次下发时设置过期 |
| `app:code:quota:ip:{ip}` | string（计数） | 滚动 24 小时 | 按客户端 IP 的下发次数配额（默认 100 次，所有渠道共用） |

- `{phone}` / `{email}` 是**归一化后的标识**（大小写、分隔符、国家码前缀的处理见第 4 节），
  由 `service/auth.canonicalTarget` 在入口与三个键构造函数里统一完成：
  `Alice@x.com`、`alice@x.com`、`+86 138-0013-8000`、`13800138000` 都必须命中同一把冷却锁与同一个配额计数器。
- 下发时：用一段 Lua 脚本原子完成「目标配额检查 → IP 配额检查 → SETNX 冷却 → INCR 两个计数器」。
  这样并发请求不会同时通过检查；冷却中或配额已满的请求不会消耗计数。
- 下发失败（生成/写入/发送失败）会删除冷却与验证码，允许立即重试；配额计数保留，
  防止用「失败重试」绕过上限。
- 冷却 key 仍带 60 秒 TTL；配额计数在首次写入时设置 24 小时 TTL（`PEXPIRE`），不做滑动续期。
- 登录校验时：用一段 Lua 脚本在同一原子操作里完成「GET → 比对 → DEL」：键不存在返回 `-1`，不匹配返回 `0`（不删），匹配则 `DEL` 并返回 `1`。把比对和删除放进同一脚本，避免「校验通过但删除前被并发再次使用」的窗口。

---

## 2. Access Token 用户级吊销（改密 / 禁用）

| 键格式 | 类型 | TTL | 说明 |
|--------|------|-----|------|
| `app:revoke:uid:{user_id}` | string（Unix 时间戳） | 与 Access Token 有效期一致 | 改密或禁用账号后，吊销该用户此前签发的 access token |

- 写入当前时间戳；`Auth` 中间件校验 token 的 `iat` 是否早于该时间戳。
- `{user_id}` 一律是 canonical UUID 字符串（小写 + 连字符）。写入方是管理员请求里的路径参数，读取方是 JWT `uid` claim，
  两者形式不同（大写、`{...}`、`urn:uuid:`）时键会对不上：接口照样返回成功，旧 token 却一个都没吊销。
  归一化在 `service.CanonicalUserID` 一处完成，任何新增的写入/读取方都应先过它。
- **不再用于普通登出**（登出改为按 session 吊销，见第 7 节），避免多设备模式下误伤其他设备。
- 改密、后台禁用账号仍会写入该键，作为 access token 的兜底吊销。
- Redis 故障时按 `auth.redis_fail_open` 策略降级（默认开发 `true`=放行，生产 `false`=拒绝），打 Warn 日志。

---

## 7. 登录会话（多设备 / 单设备）

由 `auth.multi_device_enabled` 控制（环境变量 `AUTH_MULTI_DEVICE_ENABLED`）。

| 键格式 | 类型 | TTL | 说明 |
|--------|------|-----|------|
| `app:session:{session_id}` | string（user_id） | 与 Refresh Token 一致（登录时写入，**绝对过期**） | 单个会话是否存在（唯一的有效性依据） |
| `app:sessions:uid:{user_id}` | SET（session_id…） | 滚动设为「最新一次登录 + Refresh TTL」 | **多设备模式**：`RevokeAll` 的枚举索引，不参与有效性判断 |
| `app:active_session:uid:{user_id}` | string（session_id） | 与 Refresh Token 一致 | **单设备模式**：当前唯一有效 session |

- JWT 的 `sid` 声明与 Redis 会话一一对应。
- `{user_id}` 与第 2 节同样按 canonical UUID 拼键（`session` 包在键构造点统一归一化），
  所以「用 canonical 形式登录、用大写形式吊销」仍然命中同一把键；`app:session:{id}` 的 value 也存归一化后的 ID。
- **多设备模式**（`multi_device_enabled=true`，默认）：每次登录新建 session，互不影响；登出仅 `Revoke` 当前 session。
- **单设备模式**（`multi_device_enabled=false`）：新登录前 `RevokeAll` 并写入新的 `active_session`，旧设备 token 立即失效。
- **绝对过期**：`Refresh` 不续期 `session:{sid}`；`sessions:uid` 集合会随新登录续期，但它只是 `RevokeAll` 的枚举索引，
  有效性判断只看 `session:{sid}`，所以旧 sid 的键到期后即使仍在集合里也会被拒绝。
- `Auth` 中间件除用户级 `revoke:uid` 外，会校验 `sid` 对应的 `session:{sid}` 键是否存在且 userID 匹配。
  这两个键用一次 `MGET` 读取（`session.Store.CheckAccess`），每个已鉴权请求 1 次往返；
  Redis 故障同样按 `auth.redis_fail_open` 策略降级。

---

## 3. JWT Refresh Token 黑名单（登出 / 刷新轮换）

| 键格式 | 类型 | TTL | 说明 |
|--------|------|-----|------|
| `app:blacklist:refresh:{sha256_hex}` | string | 该 token 的剩余有效期（自然过期即回收） | 已失效的 refresh token（登出或轮换后） |

- 键后缀为 refresh token 的 SHA-256 十六进制摘要，避免 key 过长与截断冲突。
- 登出时：将 token 写入黑名单。
- 刷新 token 时：先查黑名单；签发新 token 后，将旧 refresh token 加入黑名单（轮换）。
- TTL 取自 token 自身的 `exp`（并以配置的 `jwt.refresh_ttl` 为上限），不再硬编码 7 天：
  票一旦自然过期，JWT 解析就会拒绝，黑名单 key 继续留着没有防护价值，只会白占内存。
- 解析失败（伪造/乱码）或已过期的 token **不写黑名单**：这类票本来就换不到新 token，
  写进去等于给 `/auth/logout` 一个「随便发垃圾字符串就能堆 key」的内存放大器。

---

## 4. 登录暴力破解防护（密码登录）

| 键格式 | 类型 | TTL | 说明 |
|--------|------|-----|------|
| `app:login:fail:{sha256_hex}` | string（计数器） | 10 分钟 | 账号维度的连续登录失败计数 |
| `app:login:lock:{sha256_hex}` | string | 15 分钟 | 账号被锁定标记，存在即拒绝密码登录 |

- 键后缀为「账号标识（手机号/邮箱/account，统一小写去空格）」的 SHA-256 十六进制摘要，不存明文账号。
  这里的**归一化形式是唯一口径**：邮箱 = `lower(trim(x))`；手机号 = 删除所有非 ASCII 数字
  （等价 `regexp_replace(phone, '[^0-9]', '', 'g')`，与 000007 迁移同口径），剩余部分匹配
  `1[3-9]\d{9}` 时再去掉 `0086`/`86` 前缀（`strutil.NormalizeEmail` / `NormalizePhone` / `NormalizeAccount`）。
  规则由 `backend/pkg/strutil/testdata/phone_normalization_golden.tsv` 的样例表锁定，Go 单测与迁移升级测试共用。
  注册写库、登录查人、验证码键、锁定键必须由同一个函数产出，否则 `Alice@x.com` 与 `alice@x.com`
  会拆进不同桶——冷却/配额/锁定全部可被换写法绕过。
- 作用于**密码登录、验证码登录与注册校验**三条路径：失败（含「用户不存在」）时原子递增 `fail` 计数；达到 **5 次/10 分钟** 阈值时写入 `lock` 键锁定 **15 分钟**。计数与锁定写入由同一段 Redis Lua 脚本原子完成。不存在的账号同样计数并被锁定，429 因此不泄露账号是否存在；响应耗时的等时性由密码登录路径的 dummy bcrypt 保证，计数本身不改变单次耗时。
- 登录或注册成功立即删除 `fail` 计数（`lock` 键自然过期）。
- Redis 故障时按 `auth.redis_fail_open` 策略降级（默认开发 `true`=放行，生产 `false`=拒绝），打 Warn 日志。
- 阈值/窗口/锁定时长为 `service/auth/service.go` 中常量（`loginMaxFailAttempts`、`loginFailWindow`、`loginLockDuration`），命中锁定返回错误码 `10010`（`err_account_locked`，HTTP 429）。

---

## 5. 限流

使用 Redis 分布式 store（`ulule/limiter`）。键前缀由 `REDIS_KEY_PREFIX` 控制，按维度分 scope：

| 维度 | Redis 前缀 | 适用路由 | 配置项 |
|------|------------|----------|--------|
| 客户端 IP | `${REDIS_KEY_PREFIX}:limiter:ip*` | `/api/v1/auth/*` 等公开业务路由（**不含** `/user`、`/admin`，也不含 `/health`、`/ready`、`/metrics`） | `rate_limit.per_minute` / `RATE_LIMIT_PER_MINUTE` |
| 用户 ID | `${REDIS_KEY_PREFIX}:limiter:uid*` | `/api/v1/user/*`、`/api/v1/admin/*`（Auth 之后按 JWT `uid`） | `rate_limit.user_per_minute` / `RATE_LIMIT_USER_PER_MINUTE` |

- 已鉴权路由跳过 IP 限流，避免同 NAT 下多用户共享配额；未配置 `user_per_minute` 时默认与 `per_minute` 相同。
- 库内部会在上述前缀下生成计数与时间窗口相关键（具体后缀随实现变化）。
- Redis 不可用时回退到进程内 memory store（多实例限流不一致）。
- store 在运行期报错（如 Redis 中途断连）时返回 `503` + `code 50002` 的统一 JSON 信封，而不是 panic 后被 Recovery 兜成"500 + 空 body"；`/health`、`/ready`、`/metrics` 不参与限流，探针始终反映真实依赖状态。

---

## 6. 业务扩展

按模块自行约定，建议继续带项目前缀，例如：

- `app:cache:{resource}:{id}` — 热点数据缓存

会话相关键见第 7 节，不再使用笼统的 `app:user:{user_id}:session` 约定。

---

## 环境变量

- `REDIS_URL`：例如 `redis://localhost:6379/0`
- `REDIS_KEY_PREFIX`：默认 `app`
