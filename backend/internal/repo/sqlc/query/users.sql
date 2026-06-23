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
UPDATE users
SET role = $2,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateUserStatus :one
UPDATE users
SET status = $2,
    updated_at = now()
WHERE id = $1
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

