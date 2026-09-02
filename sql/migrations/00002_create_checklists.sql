-- +goose Up
CREATE TABLE checklists (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    check_names TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

CREATE INDEX idx_checklists_is_active ON checklists(is_active);

-- +goose Down
DROP TABLE IF EXISTS checklists;