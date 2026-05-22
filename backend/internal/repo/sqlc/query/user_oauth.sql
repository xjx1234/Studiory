-- name: GetUserOAuthByProviderOpenID :one
SELECT *
FROM user_oauth
WHERE provider = $1 AND open_id = $2
LIMIT 1;

-- name: CreateUserOAuth :one
INSERT INTO user_oauth (user_id, provider, open_id)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetUserByOAuth :one
SELECT u.*
FROM users u
JOIN user_oauth o ON o.user_id = u.id
WHERE o.provider = $1 AND o.open_id = $2
LIMIT 1;

