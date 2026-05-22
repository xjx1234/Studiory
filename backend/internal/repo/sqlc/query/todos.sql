-- name: CountTodosByUserID :one
SELECT COUNT(*)::bigint
FROM todos
WHERE user_id = $1;

-- name: ListTodosByUserIDPaginated :many
SELECT *
FROM todos
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: GetTodoByIDAndUserID :one
SELECT *
FROM todos
WHERE id = $1 AND user_id = $2
LIMIT 1;

-- name: CreateTodo :one
INSERT INTO todos (user_id, title)
VALUES ($1, $2)
RETURNING *;

-- name: UpdateTodo :one
UPDATE todos
SET title = $3,
    done = $4,
    updated_at = now()
WHERE id = $1 AND user_id = $2
RETURNING *;

-- name: DeleteTodo :exec
DELETE FROM todos
WHERE id = $1 AND user_id = $2;
