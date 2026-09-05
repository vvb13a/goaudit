package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/vvb13a/goaudit/domain"
)

// AuditSnapshot mirrors the "audit_snapshots" table. It stores the
// dashboard state of an audit after every run: the run time plus the
// overview totals, URL states, severity and lifecycle distributions and
// timings, each as a JSON column. Snapshots are immutable: every run of an
// audit appends a new row so the audit history is preserved.
type AuditSnapshot struct {
	ID              string    `gorm:"column:id;primaryKey;type:text"`
	AuditID         string    `gorm:"column:audit_id;type:text;not null;index:idx_audit_snapshots_audit_id"`
	CreatedAt       time.Time `gorm:"column:created_at"`
	Overview        string    `gorm:"column:overview;type:text;not null;default:'{}'"`
	URLStates       string    `gorm:"column:url_states;type:text;not null;default:'{}'"`
	SeverityCounts  string    `gorm:"column:severity_counts;type:text;not null;default:'{}'"`
	LifecycleCounts string    `gorm:"column:lifecycle_counts;type:text;not null;default:'{}'"`
	Timings         string    `gorm:"column:timings;type:text;not null;default:'{}'"`
}

func (AuditSnapshot) TableName() string { return "audit_snapshots" }

// SnapshotID derives the deterministic row id of a snapshot from its audit
// and its creation time.
func SnapshotID(auditID string, at time.Time) string {
	sum := sha256.Sum256([]byte(auditID + "\x00" + at.UTC().Format(time.RFC3339Nano)))
	return "snp_" + hex.EncodeToString(sum[:16])
}

// AuditSnapshotModel converts a domain snapshot into a row model.
func AuditSnapshotModel(s *domain.AuditSnapshot) *AuditSnapshot {
	return &AuditSnapshot{
		ID:              SnapshotID(s.AuditID, s.CreatedAt),
		AuditID:         s.AuditID,
		CreatedAt:       s.CreatedAt,
		Overview:        snapshotJSON(s.Overview),
		URLStates:       snapshotJSON(s.URLStates),
		SeverityCounts:  snapshotJSON(s.SeverityCounts),
		LifecycleCounts: snapshotJSON(s.LifecycleCounts),
		Timings:         snapshotJSON(s.Timings),
	}
}

func (m *AuditSnapshot) ToDomain() *domain.AuditSnapshot {
	snap := &domain.AuditSnapshot{
		ID:        m.ID,
		AuditID:   m.AuditID,
		CreatedAt: m.CreatedAt,
	}
	snapshotUnmarshal(m.Overview, &snap.Overview)
	snapshotUnmarshal(m.URLStates, &snap.URLStates)
	snapshotUnmarshal(m.SeverityCounts, &snap.SeverityCounts)
	snapshotUnmarshal(m.LifecycleCounts, &snap.LifecycleCounts)
	snapshotUnmarshal(m.Timings, &snap.Timings)
	return snap
}

// snapshotJSON encodes a snapshot value as its column value.
func snapshotJSON(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

// snapshotUnmarshal decodes a snapshot column value into the target,
// leaving it zeroed for empty or unparsable input.
func snapshotUnmarshal(raw string, target any) {
	if raw == "" || raw == "{}" {
		return
	}
	_ = json.Unmarshal([]byte(raw), target)
}
