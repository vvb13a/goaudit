package store

import (
	"time"

	"github.com/vvb13a/goaudit/domain"
)

// Report mirrors the "reports" table. It stores per-URL metadata of an
// audit run; the issues themselves live in the dedicated "issues" table and
// are re-attached on read. Reports belong to an Audit and are removed
// explicitly in a transaction with their parent.
type Report struct {
	ID              string `gorm:"column:id;primaryKey;type:text"`
	AuditID         string `gorm:"column:audit_id;type:text;not null;index:idx_reports_audit_id"`
	URL             string `gorm:"column:url;type:text;not null"`
	FinalURL        string `gorm:"column:final_url;type:text;not null"`
	StatusCode      int64  `gorm:"column:status_code"`
	DurationMs      int64  `gorm:"column:duration_ms"`
	PassedCount     int64  `gorm:"column:passed_count"`
	FailedCount     int64  `gorm:"column:failed_count"`
	SkippedCount    int64  `gorm:"column:skipped_count;not null;default:0"`
	HighestSeverity string `gorm:"column:highest_severity;type:text;not null"`
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
	}
}

func (m *Report) ToDomain() *domain.Report {
	return &domain.Report{
		URL:        m.URL,
		FinalURL:   m.FinalURL,
		StatusCode: int(m.StatusCode),
		Duration:   time.Duration(m.DurationMs) * time.Millisecond,
		Summary: domain.Summary{
			PassedCount:     int(m.PassedCount),
			FailedCount:     int(m.FailedCount),
			SkippedCount:    int(m.SkippedCount),
			HighestSeverity: domain.Severity(m.HighestSeverity),
		},
	}
}
