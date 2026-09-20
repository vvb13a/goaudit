package domain

import "time"

// URLFilter narrows a stored audited URL query. States and
// HighestSeverities are OR-sets within their dimension and the dimensions
// combine with AND. The duration and score bounds are inclusive; a zero
// minimum or maximum leaves that side unbounded. The zero-value filter
// matches every URL.
type URLFilter struct {
	States            []UrlState
	HighestSeverities []Severity
	DurationMin       time.Duration
	DurationMax       time.Duration
	ScoreMin          float64
	ScoreMax          float64
}
