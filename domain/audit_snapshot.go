package domain

import "time"

// SnapshotOverview holds the hero totals of a snapshot: audited URLs,
// issues and checks, plus the overall audit score.
type SnapshotOverview struct {
	TotalURLs   int     `json:"total_urls"`
	TotalIssues int     `json:"total_issues"`
	TotalChecks int     `json:"total_checks"`
	Score       float64 `json:"score"`
}

// SnapshotURLStates counts the audited URLs by their lifecycle state.
type SnapshotURLStates struct {
	New     int `json:"new"`
	Active  int `json:"active"`
	Missing int `json:"missing"`
}

// SnapshotLifecycles counts the issues of the snapshot by lifecycle.
type SnapshotLifecycles struct {
	New        int `json:"new"`
	Open       int `json:"open"`
	Resurfaced int `json:"resurfaced"`
	Resolved   int `json:"resolved"`
	Improved   int `json:"improved"`
	Degraded   int `json:"degraded"`
	Passed     int `json:"passed"`
}

// SnapshotTimings reports the wall-clock duration of the run and its
// amortized share per audited URL and per issue. Durations are zero when
// they cannot be derived (e.g. no URLs or issues).
type SnapshotTimings struct {
	Total    time.Duration `json:"total_ns"`
	PerURL   time.Duration `json:"per_url_ns"`
	PerIssue time.Duration `json:"per_issue_ns"`
}

// AuditSnapshot is the dashboard state of an audit after one run: its ID and
// creation time together with every metric shown on the dashboard (overview
// totals, URL states, severity and lifecycle distributions, timings).
type AuditSnapshot struct {
	ID              string             `json:"id"`
	AuditID         string             `json:"audit_id"`
	CreatedAt       time.Time          `json:"created_at"`
	Overview        SnapshotOverview   `json:"overview"`
	URLStates       SnapshotURLStates  `json:"url_states"`
	SeverityCounts  SeverityCounts     `json:"severity_counts"`
	LifecycleCounts SnapshotLifecycles `json:"lifecycle_counts"`
	Timings         SnapshotTimings    `json:"timings"`
}

// NewAuditSnapshot captures the dashboard state of the audit as it stands
// after a run. The audit must carry its stored URL rows (states, summaries
// and re-attached issues) for the snapshot to match what the dashboard
// shows.
func NewAuditSnapshot(a *Audit, at time.Time) AuditSnapshot {
	snap := AuditSnapshot{
		AuditID:   a.ID,
		CreatedAt: at,
	}

	// Overview: URL rows, issues and configured checks.
	issues := 0
	for _, u := range a.Urls {
		issues += len(u.Issues)
	}
	snap.Overview = SnapshotOverview{
		TotalURLs:   len(a.Urls),
		TotalIssues: issues,
		TotalChecks: snapshotCheckCount(a),
		Score:       a.Score,
	}

	// URL states.
	for _, u := range a.Urls {
		switch u.State {
		case UrlStateNew:
			snap.URLStates.New++
		case UrlStateActive:
			snap.URLStates.Active++
		case UrlStateMissing:
			snap.URLStates.Missing++
		}
	}

	// Severity and lifecycle distributions over every stored issue.
	var severity SeverityCounts
	for _, u := range a.Urls {
		for _, iss := range u.Issues {
			severity.add(iss.Severity)
			switch iss.Lifecycle {
			case LifecycleNew:
				snap.LifecycleCounts.New++
			case LifecycleOpen:
				snap.LifecycleCounts.Open++
			case LifecycleResurfaced:
				snap.LifecycleCounts.Resurfaced++
			case LifecycleResolved:
				snap.LifecycleCounts.Resolved++
			case LifecycleImproved:
				snap.LifecycleCounts.Improved++
			case LifecycleDegraded:
				snap.LifecycleCounts.Degraded++
			case LifecyclePassed:
				snap.LifecycleCounts.Passed++
			}
		}
	}
	snap.SeverityCounts = severity

	// Timings: the run duration amortized over the listed URLs and issues.
	snap.Timings.Total = a.Duration
	if n := len(a.Urls); n > 0 {
		snap.Timings.PerURL = a.Duration / time.Duration(n)
	}
	if issues > 0 {
		snap.Timings.PerIssue = a.Duration / time.Duration(issues)
	}

	return snap
}

// snapshotCheckCount returns the number of checks that ran, falling back to
// the distinct checks observed in the issues for audits without stored check
// names.
func snapshotCheckCount(a *Audit) int {
	if n := len(a.CheckNames); n > 0 {
		return n
	}
	seen := make(map[string]struct{})
	for _, u := range a.Urls {
		for _, iss := range u.Issues {
			seen[iss.CheckName] = struct{}{}
		}
	}
	return len(seen)
}
