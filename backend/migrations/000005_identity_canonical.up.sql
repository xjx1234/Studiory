-- 标识归一化：把「邮箱不区分大小写」这条规则落到数据库层（PostgreSQL）。
--
-- 服务层已统一使用 strutil.NormalizeEmail / NormalizePhone（见 docs/redis-keys.md），
-- 但历史行仍可能是 "Alice@x.com" 这种形式。PG 的 VARCHAR 比较区分大小写，于是同一个人
-- 既能注册出两个账号，也会用归一化后的 "alice@x.com" 登录不到那行旧数据；MySQL 侧
-- 因 migrations/mysql/000001 的 utf8mb4_unicode_ci 一直不区分大小写——同一个输入在两个
-- 驱动上语义相反。这里先清洗历史数据，再用表达式唯一索引兜底。
--
-- 不引入 citext：它需要 CREATE EXTENSION 权限，托管 PG（RDS/Neon/Supabase）上常拿不到，
-- 而表达式索引任何权限都能建，且与 lower() 查询可直接匹配。
--
-- 若历史数据里存在仅大小写/空白不同的重复邮箱，建索引会直接失败并列出冲突行。
-- 这是刻意的：静默合并（或丢数据）比报错更糟，需要人工决定保留哪个账号。

UPDATE users
SET email = lower(btrim(email))
WHERE email IS NOT NULL AND email <> lower(btrim(email));

-- 唯一约束 users_email_key 仍保留：等值查询 WHERE email = ? 走它，归一化后的输入即可命中。
CREATE UNIQUE INDEX IF NOT EXISTS users_email_lower_key ON users (lower(email)) WHERE email IS NOT NULL;

-- phone 无大小写问题，唯一性由 000001 的 UNIQUE 覆盖；书写形式（分隔符、+86）由服务层归一化，
-- 不建表达式索引，避免为了 SQL 旁路写入而牺牲等值查询的索引命中。
