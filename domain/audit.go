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

// Audit is a self-contained record of one audit run: it carries the run
// configuration (Name, Targets and CheckNames) inline instead of referencing
// reusable plans or checklists. Score is the overall 0-100 health score of
// the run, derived from the per-URL scores when the run is persisted.
type Audit struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Targets     []string        `json:"targets"`
	CheckNames  []string        `json:"check_names"`
	Config      json.RawMessage `json:"config,omitempty"`
	StartedAt   time.Time       `json:"started_at"`
	Duration    time.Duration   `json:"duration"`
	Score       float64         `json:"score"`
	Urls        []*AuditedUrl   `json:"urls"`
}

// CalculateScore derives the audit score from the per-URL scores. It
// requires the per-URL UrlSummary values, which the persistence layer
// computes when the run is stored.
func (a *Audit) CalculateScore() {
	var sum float64
	for _, u := range a.Urls {
		sum += u.Summary.Score
	}
	if n := len(a.Urls); n > 0 {
		a.Score = sum / float64(n)
	}
}

type AuditFilter struct {
	Limit  int
	Offset int
}
