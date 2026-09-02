-- name: GetAudit :one
SELECT * FROM audits
WHERE id = ? LIMIT 1;

-- name: ListAudits :many
SELECT * FROM audits
WHERE 
    (sqlc.narg('plan_id') IS NULL OR plan_id = sqlc.narg('plan_id'))
    AND
    (sqlc.narg('highest_severity') IS NULL OR highest_severity = sqlc.narg('highest_severity'))
ORDER BY started_at DESC
LIMIT ? OFFSET ?;

-- name: CreateAudit :exec
INSERT INTO audits (
    id, plan_id, plan_name, checklist_id, checklist_name,
    started_at, duration_ms, total_endpoints, passed_count,
    failed_count, skipped_count, highest_severity
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: DeleteAudit :exec
DELETE FROM audits
WHERE id = ?;