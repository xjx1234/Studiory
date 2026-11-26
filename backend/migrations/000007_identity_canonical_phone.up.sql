-- 000007：把历史手机号清洗成裸 11 位，与服务层 strutil.NormalizePhone 对齐。
--
-- 为什么单独成版本：000005 曾在发布后被原地追加过这段清洗（已恢复为原始发布内容，
-- 见 54038cd），执行过旧版 000005 的库停在 version 5 不会重跑，导致清洗静默失效。
-- 单独占一个版本号，才能保证所有存量库都会执行到。
--
-- 执行顺序：
--   1. 只读预检（DO 块）：复刻完整清洗链，撞号时直接报出归一化后的 phone 与冲突 id，
--      替代难以定位的 23505（users_phone_key 冲突）；
--   2. 三条幂等 UPDATE：去非数字 → 剥 0086 → 剥 86（仅当剩余部分匹配 1[3-9]\d{9}）。
--
-- 预检失败时迁移会停在 dirty 状态，处理方式见 docs/deploy.md
-- 「阻塞性迁移的前置检查」与「迁移失败处置」。

DO $$
DECLARE
    conflicts text;
BEGIN
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
    ), dup AS (
        SELECT normalized,
               string_agg(id::text, ',' ORDER BY created_at, id) AS ids
        FROM cleaned
        GROUP BY normalized
        HAVING count(*) > 1
    )
    SELECT string_agg(format('%s (ids: %s)', normalized, ids), '; ' ORDER BY normalized)
    INTO conflicts
    FROM dup;

    IF conflicts IS NOT NULL THEN
        RAISE EXCEPTION 'migration 000007 aborted: phone collision after normalization -> %', conflicts;
    END IF;
END
$$;

-- 1) 去掉分隔符/空格等非数字字符（幂等：只剩纯数字时不再匹配）。
UPDATE users
SET phone = regexp_replace(phone, '[^0-9]', '', 'g')
WHERE phone IS NOT NULL AND phone ~ '[^0-9]';

-- 2) 剥掉 0086 国家码（仅当剩余部分仍是中国大陆手机号，避免误改其他国家号码）。
UPDATE users
SET phone = substring(phone FROM 5)
WHERE phone ~ '^0086(1[3-9][0-9]{9})$';

-- 3) 剥掉 86 国家码（同上）。
UPDATE users
SET phone = substring(phone FROM 3)
WHERE phone ~ '^86(1[3-9][0-9]{9})$';
