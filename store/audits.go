package store

import (
	"encoding/json"
	"time"

	"github.com/vvb13a/goaudit/domain"
)

// Audit mirrors the "audits" table. Audits are self-contained: the run
// configuration (Name, Description, Targets and CheckNames) is stored inline;
// Targets and CheckNames are JSON-encoded text columns and Config holds the
// audit's engine configuration as a JSON object.
type Audit struct {
	ID          string    `gorm:"column:id;primaryKey;type:text"`
	Name        string    `gorm:"column:name;type:text;not null"`
	Description string    `gorm:"column:description;type:text;not null;default:''"`
	Targets     []string  `gorm:"column:targets;type:text;not null;default:'[]';serializer:json"`
	CheckNames  []string  `gorm:"column:check_names;type:text;not null;default:'[]';serializer:json"`
	Config      string    `gorm:"column:config;type:text;not null;default:'{}'"`
	StartedAt   time.Time `gorm:"column:started_at;index:idx_audits_started_at,sort:desc"`
	DurationMs  int64     `gorm:"column:duration_ms"`
	Score       float64   `gorm:"column:score;type:real;not null;default:0"`
}

func (Audit) TableName() string { return "audits" }

func AuditModel(a *domain.Audit) *Audit {
	return &Audit{
		ID:          a.ID,
		Name:        a.Name,
		Description: a.Description,
		Targets:     a.Targets,
		CheckNames:  a.CheckNames,
		Config:      auditConfigString(a.Config),
		StartedAt:   a.StartedAt,
		DurationMs:  a.Duration.Milliseconds(),
		Score:       a.Score,
	}
}

func (m *Audit) ToDomain() *domain.Audit {
	return &domain.Audit{
		ID:          m.ID,
		Name:        m.Name,
		Description: m.Description,
		Targets:     m.Targets,
		CheckNames:  m.CheckNames,
		Config:      auditConfigRaw(m.Config),
		StartedAt:   m.StartedAt,
		Duration:    time.Duration(m.DurationMs) * time.Millisecond,
		Score:       m.Score,
	}
}

// auditConfigString converts a raw config object into the column value,
// keeping the canonical "{}" placeholder for empty configs.
func auditConfigString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "{}"
	}
	return string(raw)
}

// auditConfigRaw converts the column value back into a raw config object.
func auditConfigRaw(s string) json.RawMessage {
	if s == "" || s == "null" {
		return nil
	}
	return json.RawMessage(s)
}
