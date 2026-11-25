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
-- “最后一个管理员”保护：目标降为非 admin 时，必须还存在另一个 active admin，
-- 否则本次更新不匹配任何行（repo 层据此返回 ErrLastAdmin）。
UPDATE users AS target
SET role = $2,
    updated_at = now()
WHERE target.id = $1
  AND ($2 = 'admin'
       OR EXISTS (SELECT 1
                  FROM users AS other
                  WHERE other.id <> target.id
                    AND other.role = 'admin'
                    AND other.status = 'active'))
RETURNING *;

-- name: UpdateUserStatus :one
-- 禁用（status <> 'active'）时同样要求存在另一个 active admin。
UPDATE users AS target
SET status = $2,
    updated_at = now()
WHERE target.id = $1
  AND ($2 = 'active'
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

