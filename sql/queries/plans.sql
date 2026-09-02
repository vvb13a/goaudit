-- name: GetPlan :one
SELECT * FROM plans
WHERE id = ? LIMIT 1;

-- name: ListPlans :many
SELECT * FROM plans
ORDER BY created_at DESC;

-- name: CreatePlan :exec
INSERT INTO plans (id, name, urls, created_at, updated_at)
VALUES (?, ?, ?, ?, ?);

-- name: UpdatePlan :exec
UPDATE plans
SET name = ?, urls = ?, updated_at = ?
WHERE id = ?;

-- name: DeletePlan :exec
DELETE FROM plans
WHERE id = ?;