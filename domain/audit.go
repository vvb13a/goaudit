package domain

import (
	"errors"
	"time"
)

var (
	ErrAuditNotFound   = errors.New("audit not found")
	ErrInvalidAudit    = errors.New("invalid audit data")
	ErrAuditInProgress = errors.New("audit is currently running")
)

type Summary struct {
	TotalCount      int      `json:"total_count"`
	PassedCount     int      `json:"passed_count"`
	FailedCount     int      `json:"failed_count"`
	SkippedCount    int      `json:"skipped_count"`
	HighestSeverity Severity `json:"highest_severity"`
}

type Report struct {
	URL        string        `json:"url"`
	FinalURL   string        `json:"final_url"`
	StatusCode int           `json:"status_code"`
	Duration   time.Duration `json:"duration"`
	Summary    Summary       `json:"summary"`
	Issues     []Issue       `json:"issues"`
}

func (r *Report) CalculateSummary() {
	r.Summary = Summary{
		TotalCount:      len(r.Issues),
		HighestSeverity: SeveritySuccess,
	}

	for _, issue := range r.Issues {
		if issue.Passed {
			r.Summary.PassedCount++
		} else {
			r.Summary.FailedCount++
		}

		if issue.Severity.IsHigherThan(r.Summary.HighestSeverity) {
			r.Summary.HighestSeverity = issue.Severity
		}
	}
}

type Audit struct {
	ID            string        `json:"id"`
	PlanID        string        `json:"plan_id,omitempty"`
	PlanName      string        `json:"plan_name"`
	ChecklistID   string        `json:"checklist_id,omitempty"`
	ChecklistName string        `json:"checklist_name"`
	StartedAt     time.Time     `json:"started_at"`
	Duration      time.Duration `json:"duration"`
	Summary       Summary       `json:"summary"`
	Reports       []*Report     `json:"reports"`
}

func (a *Audit) CalculateSummary() {
	a.Summary = Summary{
		TotalCount:      len(a.Reports),
		HighestSeverity: SeveritySuccess,
	}

	for _, r := range a.Reports {
		r.CalculateSummary()

		if r.Summary.FailedCount == 0 {
			a.Summary.PassedCount++
		} else {
			a.Summary.FailedCount++
		}

		if r.Summary.HighestSeverity.IsHigherThan(a.Summary.HighestSeverity) {
			a.Summary.HighestSeverity = r.Summary.HighestSeverity
		}
	}
}

type AuditFilter struct {
	PlanID          *string
	HighestSeverity *Severity
	Limit           int
	Offset          int
}
