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

// Highest returns the most severe severity present in the counts, or the
// empty severity when there are no counts.
func (c SeverityCounts) Highest() Severity {
	for i := len(AllSeverities) - 1; i >= 0; i-- {
		if c.Count(AllSeverities[i]) > 0 {
			return AllSeverities[i]
		}
	}
	return ""
}

// Add adds n issues of the given severity to the counts. It folds a grouped
// count row into the bucket without iterating over every issue.
func (c *SeverityCounts) Add(severity Severity, n int) {
	switch severity {
	case SeveritySuccess:
		c.Success += n
	case SeverityInfo:
		c.Info += n
	case SeverityNotice:
		c.Notice += n
	case SeverityWarning:
		c.Warning += n
	case SeverityError:
		c.Error += n
	case SeverityFatal:
		c.Fatal += n
	}
}
