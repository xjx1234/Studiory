-- 只读检查：找出「清洗后会撞号」的手机号。
--
-- 清洗口径与 000007_identity_canonical_phone.up.sql、服务层 strutil.NormalizePhone 一致：
--   1) 去掉所有非数字字符；
--   2) 剩余部分匹配 ^0086(1[3-9][0-9]{9})$ 时剥掉 0086；
--   3) 剩余部分匹配 ^86(1[3-9][0-9]{9})$ 时剥掉 86。
--
-- 口径由 backend/pkg/strutil/testdata/phone_normalization_golden.tsv 锁定：
-- migration_upgrade_test.sh 的 5b 会用同一批样例断言本脚本与 Go / 000007 结果一致。
--
-- 用法（存在存量库时必须先跑）：
--   psql "$DATABASE_URL" -f scripts/check_phone_collisions.sql
--
-- 有输出 = 000007 会中止。先人工决定每组保留哪个账号（合并或停用），再跑迁移。
WITH digits AS (
    SELECT id,
           created_at,
           regexp_replace(phone, '[^0-9]', '', 'g') AS d
    FROM users
    WHERE phone IS NOT NULL
), cleaned AS (
    SELECT id,
           created_at,
           CASE
               WHEN d ~ '^0086(1[3-9][0-9]{9})$' THEN substring(d FROM 5)
               WHEN d ~ '^86(1[3-9][0-9]{9})$' THEN substring(d FROM 3)
               ELSE d
           END AS normalized
    FROM digits
)
SELECT normalized,
       count(*) AS rows,
       string_agg(id::text, ',' ORDER BY created_at, id) AS ids
FROM cleaned
GROUP BY normalized
HAVING count(*) > 1
ORDER BY normalized;
