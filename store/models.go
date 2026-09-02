package store

import (
	"time"

	"github.com/vvb13a/goaudit/domain"
)

// Plan mirrors the "plans" table. URLs are stored as a JSON-encoded text column.
type Plan struct {
	ID        string    `gorm:"column:id;primaryKey;type:text"`
	Name      string    `gorm:"column:name;type:text;not null"`
	URLs      []string  `gorm:"column:urls;type:text;not null;serializer:json"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (Plan) TableName() string { return "plans" }

func PlanModel(p *domain.Plan) *Plan {
	return &Plan{
		ID:        p.ID,
		Name:      p.Name,
		URLs:      p.URLs,
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
	}
}

func (m *Plan) ToDomain() *domain.Plan {
	return &domain.Plan{
		ID:        m.ID,
		Name:      m.Name,
		URLs:      m.URLs,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}

// Checklist mirrors the "checklists" table. CheckNames are stored as a
// JSON-encoded text column.
type Checklist struct {
	ID          string    `gorm:"column:id;primaryKey;type:text"`
	Name        string    `gorm:"column:name;type:text;not null"`
	Description string    `gorm:"column:description;type:text;not null;default:''"`
	CheckNames  []string  `gorm:"column:check_names;type:text;not null;serializer:json"`
	IsActive    bool      `gorm:"column:is_active;index:idx_checklists_is_active;not null;default:false"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

func (Checklist) TableName() string { return "checklists" }

func ChecklistModel(c *domain.Checklist) *Checklist {
	return &Checklist{
		ID:          c.ID,
		Name:        c.Name,
		Description: c.Description,
		CheckNames:  c.CheckNames,
		IsActive:    c.IsActive,
		CreatedAt:   c.CreatedAt,
		UpdatedAt:   c.UpdatedAt,
	}
}

func (m *Checklist) ToDomain() *domain.Checklist {
	return &domain.Checklist{
		ID:          m.ID,
		Name:        m.Name,
		Description: m.Description,
		CheckNames:  m.CheckNames,
		IsActive:    m.IsActive,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

// Audit mirrors the "audits" table. PlanID and ChecklistID are nullable and
// reference no FK constraint; names are denormalized snapshots.
type Audit struct {
	ID              string    `gorm:"column:id;primaryKey;type:text"`
	PlanID          *string   `gorm:"column:plan_id;index:idx_audits_plan_id"`
	PlanName        string    `gorm:"column:plan_name;type:text;not null"`
	ChecklistID     *string   `gorm:"column:checklist_id"`
	ChecklistName   string    `gorm:"column:checklist_name;type:text;not null"`
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
		PlanID:          optionalString(a.PlanID),
		PlanName:        a.PlanName,
		ChecklistID:     optionalString(a.ChecklistID),
		ChecklistName:   a.ChecklistName,
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
		ID:            m.ID,
		PlanID:        valueOrEmpty(m.PlanID),
		PlanName:      m.PlanName,
		ChecklistID:   valueOrEmpty(m.ChecklistID),
		ChecklistName: m.ChecklistName,
		StartedAt:     m.StartedAt,
		Duration:      time.Duration(m.DurationMs) * time.Millisecond,
		Summary: domain.Summary{
			TotalCount:      int(m.TotalEndpoints),
			PassedCount:     int(m.PassedCount),
			FailedCount:     int(m.FailedCount),
			SkippedCount:    int(m.SkippedCount),
			HighestSeverity: domain.Severity(m.HighestSeverity),
		},
	}
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

func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func valueOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
