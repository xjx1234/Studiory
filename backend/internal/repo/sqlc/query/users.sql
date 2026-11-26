-- name: GetUserByID :one
SELECT *
FROM users
WHERE id = $1
LIMIT 1;

-- name: GetUserByPhone :one
SELECT *
FROM users
WHERE phone = $1
LIMIT 1;

-- name: GetUserByEmail :one
SELECT *
FROM users
WHERE email = $1
LIMIT 1;

-- name: CreateUser :one
INSERT INTO users (phone, email, password_hash, nickname, avatar, role)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: UpdateUserPassword :one
UPDATE users
SET password_hash = $2,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateUserProfile :one
UPDATE users
SET nickname = $2,
    avatar = $3,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateUserRole :one
-- “最后一个管理员”保护：只有「目标当前是 active admin，且目标是唯一的 active admin」
-- 这一种情况才拦截。目标是普通用户 / 已禁用账号时不受限制——否则库里一个 admin 都没有时，
-- 连普通用户的改角色请求都会被拒（集成测试踩到过这个回归）。
UPDATE users AS target
SET role = $2,
    updated_at = now()
WHERE target.id = $1
  AND ($2 = 'admin'
       OR target.role <> 'admin'
       OR target.status <> 'active'
       OR EXISTS (SELECT 1
                  FROM users AS other
                  WHERE other.id <> target.id
                    AND other.role = 'admin'
                    AND other.status = 'active'))
RETURNING *;

-- name: UpdateUserStatus :one
-- 同样只拦截「禁用唯一 active admin」，其余（含禁用普通用户）照常放行。
UPDATE users AS target
SET status = $2,
    updated_at = now()
WHERE target.id = $1
  AND ($2 = 'active'
       OR target.role <> 'admin'
       OR target.status <> 'active'
       OR EXISTS (SELECT 1
                  FROM users AS other
                  WHERE other.id <> target.id
                    AND other.role = 'admin'
                    AND other.status = 'active'))
RETURNING *;

-- name: ListUsers :many
SELECT *
FROM users
WHERE (@keyword::text = ''
       OR phone ILIKE '%' || @keyword || '%'
       OR email ILIKE '%' || @keyword || '%'
       OR nickname ILIKE '%' || @keyword || '%')
  AND (@status::text = '' OR status = @status::text)
ORDER BY created_at DESC
LIMIT @limit_count OFFSET @offset_count;

-- name: CountUsers :one
SELECT COUNT(*)::bigint
FROM users
WHERE (@keyword::text = ''
       OR phone ILIKE '%' || @keyword || '%'
       OR email ILIKE '%' || @keyword || '%'
       OR nickname ILIKE '%' || @keyword || '%')
  AND (@status::text = '' OR status = @status::text);

