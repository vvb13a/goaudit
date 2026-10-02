package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/vvb13a/goaudit/domain"
)

// GraphSnapshot mirrors the "graph_snapshots" table. It stores the graph state
// of an audit after every run that performed graph extraction, so the graph
// timeline is independent of the check timeline. Snapshots are immutable:
// every graph run appends a row.
type GraphSnapshot struct {
	ID            string    `gorm:"column:id;primaryKey;type:text"`
	AuditID       string    `gorm:"column:audit_id;type:text;not null;index:idx_graph_snapshots_audit_id"`
	CreatedAt     time.Time `gorm:"column:created_at"`
	TotalNodes    int       `gorm:"column:total_nodes;not null;default:0"`
	TotalEdges    int       `gorm:"column:total_edges;not null;default:0"`
	InternalNodes int       `gorm:"column:internal_nodes;not null;default:0"`
	ExternalNodes int       `gorm:"column:external_nodes;not null;default:0"`
	RootNodes     int       `gorm:"column:root_nodes;not null;default:0"`
	Filetypes     string    `gorm:"column:filetypes;type:text;not null;default:'[]'"`
	DurationMs    int64     `gorm:"column:duration_ms;not null;default:0"`
}

func (GraphSnapshot) TableName() string { return "graph_snapshots" }

// GraphSnapshotID derives the deterministic row id of a graph snapshot from its
// audit and its creation time.
func GraphSnapshotID(auditID string, at time.Time) string {
	sum := sha256.Sum256([]byte(auditID + "\x00" + at.UTC().Format(time.RFC3339Nano)))
	return "gsp_" + hex.EncodeToString(sum[:16])
}

// GraphSnapshotModel converts a domain graph snapshot into a row model.
func GraphSnapshotModel(s *domain.GraphSnapshot) *GraphSnapshot {
	raw, err := json.Marshal(s.Filetypes)
	if err != nil {
		raw = []byte("[]")
	}
	return &GraphSnapshot{
		ID:            GraphSnapshotID(s.AuditID, s.CreatedAt),
		AuditID:       s.AuditID,
		CreatedAt:     s.CreatedAt,
		TotalNodes:    s.TotalNodes,
		TotalEdges:    s.TotalEdges,
		InternalNodes: s.InternalNodes,
		ExternalNodes: s.ExternalNodes,
		RootNodes:     s.RootNodes,
		Filetypes:     string(raw),
		DurationMs:    s.Duration.Milliseconds(),
	}
}

func (m *GraphSnapshot) ToDomain() *domain.GraphSnapshot {
	snap := &domain.GraphSnapshot{
		ID:            m.ID,
		AuditID:       m.AuditID,
		CreatedAt:     m.CreatedAt,
		TotalNodes:    m.TotalNodes,
		TotalEdges:    m.TotalEdges,
		InternalNodes: m.InternalNodes,
		ExternalNodes: m.ExternalNodes,
		RootNodes:     m.RootNodes,
		Duration:      time.Duration(m.DurationMs) * time.Millisecond,
		Filetypes:     []domain.FiletypeCount{},
	}
	if m.Filetypes != "" && m.Filetypes != "[]" {
		_ = json.Unmarshal([]byte(m.Filetypes), &snap.Filetypes)
	}
	return snap
}
