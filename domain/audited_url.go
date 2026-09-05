package domain

import "time"

// UrlState describes where an audited URL of an audit stands relative to
// the audit's previous run:
//
//	new     the URL was audited by this audit for the first time
//	active  the URL was audited again by the latest run
//	missing the URL was audited by an earlier run but did not reappear in
//	        the latest one; its row is kept so the run history is not lost
type UrlState string

const (
	UrlStateNew     UrlState = "new"
	UrlStateActive  UrlState = "active"
	UrlStateMissing UrlState = "missing"
)

func (s UrlState) IsValid() bool {
	switch s {
	case UrlStateNew, UrlStateActive, UrlStateMissing:
		return true
	}
	return false
}

func (s UrlState) String() string {
	return string(s)
}

// SeverityCounts tallies the issues of one audited URL by severity. Passed
// checks fall into the lower severities; failures start at warning.
type SeverityCounts struct {
	Success int `json:"success"`
	Info    int `json:"info"`
	Notice  int `json:"notice"`
	Warning int `json:"warning"`
	Error   int `json:"error"`
	Fatal   int `json:"fatal"`
}

// Total returns the number of issues of every severity.
func (c SeverityCounts) Total() int {
	return c.Success + c.Info + c.Notice + c.Warning + c.Error + c.Fatal
}

// Failed returns how many issues did not pass (severity warning and worse).
func (c SeverityCounts) Failed() int {
	return c.Warning + c.Error + c.Fatal
}

// Passed returns how many issues passed.
func (c SeverityCounts) Passed() int {
	return c.Total() - c.Failed()
}

// Count returns the number of issues at the given severity.
func (c SeverityCounts) Count(severity Severity) int {
	switch severity {
	case SeveritySuccess:
		return c.Success
	case SeverityInfo:
		return c.Info
	case SeverityNotice:
		return c.Notice
	case SeverityWarning:
		return c.Warning
	case SeverityError:
		return c.Error
	case SeverityFatal:
		return c.Fatal
	default:
		return 0
	}
}

func (c *SeverityCounts) add(severity Severity) {
	switch severity {
	case SeveritySuccess:
		c.Success++
	case SeverityInfo:
		c.Info++
	case SeverityNotice:
		c.Notice++
	case SeverityWarning:
		c.Warning++
	case SeverityError:
		c.Error++
	case SeverityFatal:
		c.Fatal++
	}
}

// UrlSummary is the verdict of one audited URL. It holds the per-severity
// issue counts, the highest severity observed and a health score between 0
// and 100 (severity-weighted penalty per failing issue, floored at 0). The
// summary is derived from the URL's issues when the run is persisted.
type UrlSummary struct {
	SeverityCounts  SeverityCounts `json:"severity_counts"`
	HighestSeverity Severity       `json:"highest_severity"`
	Score           float64        `json:"score"`
}

// scorePenalty is the health score deduction of one failing issue of the
// given severity.
func scorePenalty(severity Severity) float64 {
	switch severity {
	case SeverityFatal:
		return 25
	case SeverityError:
		return 15
	case SeverityWarning:
		return 8
	case SeverityNotice:
		return 4
	default:
		return 0
	}
}

// Calculate derives the summary from the given issues. Only failing issues
// (severity warning and worse) deduct from the score.
func (s *UrlSummary) Calculate(issues []Issue) {
	counts := SeverityCounts{}
	highest := SeveritySuccess
	score := 100.0

	for i := range issues {
		iss := &issues[i]
		counts.add(iss.Severity)
		if iss.Severity.IsFailure() {
			score -= scorePenalty(iss.Severity)
		}
		if iss.Severity.IsHigherThan(highest) {
			highest = iss.Severity
		}
	}

	if score < 0 {
		score = 0
	}
	s.SeverityCounts = counts
	s.HighestSeverity = highest
	s.Score = score
}

func (s UrlSummary) TotalCount() int  { return s.SeverityCounts.Total() }
func (s UrlSummary) FailedCount() int { return s.SeverityCounts.Failed() }
func (s UrlSummary) PassedCount() int { return s.SeverityCounts.Passed() }

// AuditedUrl is one audited URL of an audit run: the page that was checked,
// its outcome and its lifecycle metadata across reruns. State, the
// timestamps and Summary are filled in by the persistence layer; checks
// only produce the Issues.
type AuditedUrl struct {
	URL         string        `json:"url"`
	FinalURL    string        `json:"final_url"`
	StatusCode  int           `json:"status_code"`
	Duration    time.Duration `json:"duration"`
	State       UrlState      `json:"state"`
	CreatedAt   time.Time     `json:"created_at"`
	LastAudited time.Time     `json:"last_audited_at"`
	Summary     UrlSummary    `json:"summary"`
	Issues      []Issue       `json:"issues"`
}

// CalculateSummary derives the URL summary from its issues. It is computed
// when the run is persisted, not while the checks run.
func (u *AuditedUrl) CalculateSummary() {
	u.Summary.Calculate(u.Issues)
}
