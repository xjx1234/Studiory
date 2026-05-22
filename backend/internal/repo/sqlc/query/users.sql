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

