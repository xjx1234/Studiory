-- 只读检查：找出「仅大小写/空白不同」的重复邮箱。
--
-- 口径与 000005_identity_canonical.up.sql、服务层 strutil.NormalizeEmail 一致：
--   lower(btrim(email))
--
-- 用法（存在存量库时必须先跑）：
--   psql "$DATABASE_URL" -f scripts/check_email_collisions.sql
--
-- 有输出 = 000005 建 users_email_lower_key 会失败。先人工决定每组保留哪个账号再跑迁移。
SELECT lower(btrim(email)) AS normalized,
       count(*) AS rows,
       string_agg(id::text, ',' ORDER BY created_at, id) AS ids
FROM users
WHERE email IS NOT NULL
GROUP BY 1
HAVING count(*) > 1
ORDER BY normalized;
