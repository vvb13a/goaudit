-- name: GetChecklist :one
SELECT * FROM checklists
WHERE id = ? LIMIT 1;

-- name: GetActiveChecklist :one
SELECT * FROM checklists
WHERE is_active = 1
LIMIT 1;

-- name: ListChecklists :many
SELECT * FROM checklists
ORDER BY created_at DESC;

-- name: CreateChecklist :exec
INSERT INTO checklists (id, name, description, check_names, is_active, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: UpdateChecklist :exec
UPDATE checklists
SET name = ?, description = ?, check_names = ?, updated_at = ?
WHERE id = ?;

-- name: DeactivateAllChecklists :exec
UPDATE checklists
SET is_active = 0;

-- name: SetActiveChecklist :exec
UPDATE checklists
SET is_active = 1, updated_at = ?
WHERE id = ?;

-- name: DeleteChecklist :exec
DELETE FROM checklists
WHERE id = ?;