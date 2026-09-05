package domain

import (
	"encoding/json"
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

// Audit is a self-contained record of one audit run: it carries the run
// configuration (Name, Targets and CheckNames) inline instead of referencing
// reusable plans or checklists.
type Audit struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Targets     []string        `json:"targets"`
	CheckNames  []string        `json:"check_names"`
	Config      json.RawMessage `json:"config,omitempty"`
	StartedAt   time.Time       `json:"started_at"`
	Duration    time.Duration   `json:"duration"`
	Summary     Summary         `json:"summary"`
	Urls        []*AuditedUrl   `json:"urls"`
}

// CalculateSummary aggregates the per-URL verdicts into the audit summary.
// It requires the per-URL UrlSummary values, which the persistence layer
// computes when the run is stored.
func (a *Audit) CalculateSummary() {
	a.Summary = Summary{
		TotalCount:      len(a.Urls),
		HighestSeverity: SeveritySuccess,
	}

	for _, u := range a.Urls {
		if u.Summary.FailedCount() == 0 {
			a.Summary.PassedCount++
		} else {
			a.Summary.FailedCount++
		}

		if u.Summary.HighestSeverity.IsHigherThan(a.Summary.HighestSeverity) {
			a.Summary.HighestSeverity = u.Summary.HighestSeverity
		}
	}
}

type AuditFilter struct {
	HighestSeverity *Severity
	Limit           int
	Offset          int
}
