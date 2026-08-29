package data

import "time"

type Summary struct {
	TotalIssues     int      `json:"total_issues"`
	PassedCount     int      `json:"passed_count"`
	SkippedCount    int      `json:"skipped_count"`
	FailedCount     int      `json:"failed_count"`
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

type Inspection struct {
	ID              string        `json:"id"`
	PlanID          string        `json:"plan_id,omitempty"`
	PlanName        string        `json:"plan_name"`
	ChecklistID     string        `json:"checklist_id,omitempty"`
	ChecklistName   string        `json:"checklist_name"`
	StartedAt       time.Time     `json:"started_at"`
	Duration        time.Duration `json:"duration"`
	TotalEndpoints  int           `json:"total_endpoints"`
	PassedCount     int           `json:"passed_count"`
	FailedCount     int           `json:"failed_count"`
	HighestSeverity Severity      `json:"highest_severity"`
	Reports         []*Report     `json:"reports"`
}
