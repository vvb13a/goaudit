package domain

import "time"

// Issue is a single check result. Checks now emit exactly one issue per
// (audit, url, check) combination, so a persisted issue is uniquely
// identified by those three values. The row metadata (ID, URL, timestamps,
// PriorSeverity) is filled in by the persistence layer; checks only build the
// check result fields.
type Issue struct {
	ID            string         `json:"id,omitempty"`
	URL           string         `json:"url,omitempty"`
	CheckName     string         `json:"check_name"`
	Category      Category       `json:"category"`
	Severity      Severity       `json:"severity"`
	Passed        bool           `json:"passed"`
	Message       string         `json:"message"`
	Details       map[string]any `json:"details,omitempty"`
	PriorSeverity Severity       `json:"prior_severity,omitempty"`
	CreatedAt     time.Time      `json:"created_at,omitempty"`
	UpdatedAt     time.Time      `json:"updated_at,omitempty"`
}

func (i Issue) IsFailure() bool {
	return !i.Passed && i.Severity.IsFailure()
}

func NewRawIssue(checkName string, category Category, severity Severity, message string, details map[string]any) Issue {
	return Issue{
		CheckName: checkName,
		Category:  category,
		Severity:  severity,
		Passed:    !severity.IsFailure(),
		Message:   message,
		Details:   details,
	}
}
