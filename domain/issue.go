package domain

import "time"

// IssueLifecycle describes how an issue changed between the previous run
// state (PriorSeverity) and its current state (Severity). Severities compare
// by weight, so e.g. fatal->error is "improved" just like error->warning.
type IssueLifecycle string

const (
	LifecycleNew        IssueLifecycle = "new"
	LifecyclePassed     IssueLifecycle = "passed"
	LifecycleResurfaced IssueLifecycle = "resurfaced"
	LifecycleResolved   IssueLifecycle = "resolved"
	LifecycleOpen       IssueLifecycle = "open"
	LifecycleImproved   IssueLifecycle = "improved"
	LifecycleDegraded   IssueLifecycle = "degraded"
)

// ComputeLifecycle derives the lifecycle of an issue from its prior severity
// and its current severity:
//
//	no prior            -> new
//	same severity       -> passed (success) or open (any real issue)
//	prior success       -> resurfaced
//	current success     -> resolved
//	current worse       -> degraded
//	current better      -> improved
func ComputeLifecycle(prior Severity, hasPrior bool, current Severity) IssueLifecycle {
	if !hasPrior {
		return LifecycleNew
	}
	if prior == current {
		if current == SeveritySuccess {
			return LifecyclePassed
		}
		return LifecycleOpen
	}
	if prior == SeveritySuccess {
		return LifecycleResurfaced
	}
	if current == SeveritySuccess {
		return LifecycleResolved
	}
	if current.Weight() > prior.Weight() {
		return LifecycleDegraded
	}
	return LifecycleImproved
}

// Issue is a single check result. Checks now emit exactly one issue per
// (audit, url, check) combination, so a persisted issue is uniquely
// identified by those three values. The row metadata (ID, URL, timestamps,
// PriorSeverity, Lifecycle) is filled in by the persistence layer; checks
// only build the check result fields.
type Issue struct {
	ID        string         `json:"id,omitempty"`
	URL       string         `json:"url,omitempty"`
	CheckName string         `json:"check_name"`
	Category  Category       `json:"category"`
	Severity  Severity       `json:"severity"`
	Passed    bool           `json:"passed"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`

	// PriorSeverity is the severity of the same issue in the previous run
	// state (empty when the issue has no prior state). Lifecycle describes
	// how the issue changed from PriorSeverity to Severity.
	PriorSeverity Severity       `json:"prior_severity,omitempty"`
	Lifecycle     IssueLifecycle `json:"lifecycle,omitempty"`
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
