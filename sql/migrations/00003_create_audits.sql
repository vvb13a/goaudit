-- +goose Up
CREATE TABLE audits (
    id TEXT PRIMARY KEY,
    plan_id TEXT,
    plan_name TEXT NOT NULL,
    checklist_id TEXT,
    checklist_name TEXT NOT NULL,
    started_at DATETIME NOT NULL,
    duration_ms INTEGER NOT NULL,
    total_endpoints INTEGER NOT NULL,
    passed_count INTEGER NOT NULL,
    failed_count INTEGER NOT NULL,
    skipped_count INTEGER NOT NULL DEFAULT 0,
    highest_severity TEXT NOT NULL
);

CREATE INDEX idx_audits_started_at ON audits(started_at DESC);
CREATE INDEX idx_audits_plan_id ON audits(plan_id);

-- +goose Down
DROP TABLE IF EXISTS audits;