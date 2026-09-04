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
	ID              string    `gorm:"column:id;primaryKey;type:text"`
	Name            string    `gorm:"column:name;type:text;not null"`
	Description     string    `gorm:"column:description;type:text;not null;default:''"`
	Targets         []string  `gorm:"column:targets;type:text;not null;default:'[]';serializer:json"`
	CheckNames      []string  `gorm:"column:check_names;type:text;not null;default:'[]';serializer:json"`
	Config          string    `gorm:"column:config;type:text;not null;default:'{}'"`
	StartedAt       time.Time `gorm:"column:started_at;index:idx_audits_started_at,sort:desc"`
	DurationMs      int64     `gorm:"column:duration_ms"`
	TotalEndpoints  int64     `gorm:"column:total_endpoints"`
	PassedCount     int64     `gorm:"column:passed_count"`
	FailedCount     int64     `gorm:"column:failed_count"`
	SkippedCount    int64     `gorm:"column:skipped_count;not null;default:0"`
	HighestSeverity string    `gorm:"column:highest_severity;type:text;not null"`
}

func (Audit) TableName() string { return "audits" }

func AuditModel(a *domain.Audit) *Audit {
	return &Audit{
		ID:              a.ID,
		Name:            a.Name,
		Description:     a.Description,
		Targets:         a.Targets,
		CheckNames:      a.CheckNames,
		Config:          auditConfigString(a.Config),
		StartedAt:       a.StartedAt,
		DurationMs:      a.Duration.Milliseconds(),
		TotalEndpoints:  int64(a.Summary.TotalCount),
		PassedCount:     int64(a.Summary.PassedCount),
		FailedCount:     int64(a.Summary.FailedCount),
		SkippedCount:    int64(a.Summary.SkippedCount),
		HighestSeverity: string(a.Summary.HighestSeverity),
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
		Summary: domain.Summary{
			TotalCount:      int(m.TotalEndpoints),
			PassedCount:     int(m.PassedCount),
			FailedCount:     int(m.FailedCount),
			SkippedCount:    int(m.SkippedCount),
			HighestSeverity: domain.Severity(m.HighestSeverity),
		},
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

// Report mirrors the "reports" table. Issues are stored as a JSON-encoded
// text column. Reports belong to an Audit and are removed explicitly in a
// transaction with their parent.
type Report struct {
	ID              string         `gorm:"column:id;primaryKey;type:text"`
	AuditID         string         `gorm:"column:audit_id;type:text;not null;index:idx_reports_audit_id"`
	URL             string         `gorm:"column:url;type:text;not null"`
	FinalURL        string         `gorm:"column:final_url;type:text;not null"`
	StatusCode      int64          `gorm:"column:status_code"`
	DurationMs      int64          `gorm:"column:duration_ms"`
	PassedCount     int64          `gorm:"column:passed_count"`
	FailedCount     int64          `gorm:"column:failed_count"`
	SkippedCount    int64          `gorm:"column:skipped_count;not null;default:0"`
	HighestSeverity string         `gorm:"column:highest_severity;type:text;not null"`
	Issues          []domain.Issue `gorm:"column:issues;type:text;not null;serializer:json"`
}

func (Report) TableName() string { return "reports" }

// ReportModel converts a domain report into a row model. The report row ID is
// derived from the parent audit ID and the report's position within it.
func ReportModel(auditID string, id string, r *domain.Report) *Report {
	return &Report{
		ID:              id,
		AuditID:         auditID,
		URL:             r.URL,
		FinalURL:        r.FinalURL,
		StatusCode:      int64(r.StatusCode),
		DurationMs:      r.Duration.Milliseconds(),
		PassedCount:     int64(r.Summary.PassedCount),
		FailedCount:     int64(r.Summary.FailedCount),
		SkippedCount:    int64(r.Summary.SkippedCount),
		HighestSeverity: string(r.Summary.HighestSeverity),
		Issues:          r.Issues,
	}
}

func (m *Report) ToDomain() *domain.Report {
	return &domain.Report{
		URL:        m.URL,
		FinalURL:   m.FinalURL,
		StatusCode: int(m.StatusCode),
		Duration:   time.Duration(m.DurationMs) * time.Millisecond,
		Summary: domain.Summary{
			TotalCount:      len(m.Issues),
			PassedCount:     int(m.PassedCount),
			FailedCount:     int(m.FailedCount),
			SkippedCount:    int(m.SkippedCount),
			HighestSeverity: domain.Severity(m.HighestSeverity),
		},
		Issues: m.Issues,
	}
}
