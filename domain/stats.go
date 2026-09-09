package domain

import "time"

// URLAggregates is the aggregate state of the audited URLs of one audit: the
// count of URL rows per state plus the average duration and score across all
// of them. It backs the url tab's summary widget without loading the rows.
type URLAggregates struct {
	URLStates   SnapshotURLStates
	AvgDuration time.Duration
	AvgScore    float64
}

// Total returns the number of URL rows the aggregates describe.
func (a URLAggregates) Total() int {
	return a.URLStates.New + a.URLStates.Active + a.URLStates.Missing
}

// AuditMetrics is the aggregate state of an audit for the dashboard: the
// URL state counts and the per-severity and per-lifecycle distributions of
// its stored issues. The overview totals derive from these without loading
// any rows.
type AuditMetrics struct {
	URLStates SnapshotURLStates
	Severity  SeverityCounts
	Lifecycle SnapshotLifecycles
}

// TotalURLs returns how many audited URL rows the metrics describe.
func (m AuditMetrics) TotalURLs() int {
	return m.URLStates.New + m.URLStates.Active + m.URLStates.Missing
}

// TotalIssues returns how many stored issues the metrics describe.
func (m AuditMetrics) TotalIssues() int {
	return m.Severity.Total()
}

// Inc adds one issue of the given severity to the counts. It is the exported
// entry point used to fold aggregated rows into the bucket.
func (c *SeverityCounts) Inc(severity Severity) {
	c.add(severity)
}
