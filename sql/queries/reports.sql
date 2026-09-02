-- name: CreateReport :exec
INSERT INTO reports (
    id, audit_id, url, final_url, status_code,
    duration_ms, passed_count, failed_count,
    skipped_count, highest_severity, issues
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListReportsByAuditID :many
SELECT * FROM reports
WHERE audit_id = ?
ORDER BY id ASC;