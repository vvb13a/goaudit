-- +goose Up
CREATE TABLE reports (
    id TEXT PRIMARY KEY,
    audit_id TEXT NOT NULL REFERENCES audits(id) ON DELETE CASCADE,
    url TEXT NOT NULL,
    final_url TEXT NOT NULL,
    status_code INTEGER NOT NULL,
    duration_ms INTEGER NOT NULL,
    passed_count INTEGER NOT NULL,
    failed_count INTEGER NOT NULL,
    skipped_count INTEGER NOT NULL DEFAULT 0,
    highest_severity TEXT NOT NULL,
    issues TEXT NOT NULL
);

CREATE INDEX idx_reports_audit_id ON reports(audit_id);

-- +goose Down
DROP TABLE IF EXISTS reports;