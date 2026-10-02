package domain

import "context"

// CheckScope marks when a check runs. Document checks run per fetched page
// during the crawl; graph checks run once at the end of a run, against the
// run's link graph and its persisted validation results.
type CheckScope string

const (
	ScopeDocument CheckScope = "document"
	ScopeGraph    CheckScope = "graph"
)

type CheckInfo struct {
	Name        string     `json:"name"`
	Label       string     `json:"label"`
	Description string     `json:"description"`
	Category    Category   `json:"category"`
	Scope       CheckScope `json:"scope"`
}

// LinkRef is one reference from an audited page to a target URL, as seen by a
// graph check.
type LinkRef struct {
	SourceURL string
	TargetURL string
	Type      string
}

// TargetStatus is the validation state of a link target: its classification
// (filetype/external/audited) and the HTTP result. Validated is false when the
// target has no fetched or validated status.
type TargetStatus struct {
	URL        string
	Filetype   string
	External   bool
	Audited    bool
	StatusCode int
	Error      string
	FinalURL   string
	Validated  bool
}

// GraphView is the read-only view of a run's link graph that graph checks
// evaluate.
type GraphView interface {
	Edges() []LinkRef
	Target(url string) (TargetStatus, bool)
	Sources() []string
}

// PageIssue is a graph-check result attributed to the page that produced it.
type PageIssue struct {
	URL   string
	Issue Issue
}

// GraphCheck is a check that evaluates the run's link graph instead of a single
// fetched document. Apply is never called (Supports returns false); Evaluate is
// invoked once after link validation.
type GraphCheck interface {
	Check
	Evaluate(ctx context.Context, view GraphView) []PageIssue
}

// CheckSummary is the aggregate state of one check over an audit: the check's
// category and how many issues it produced, grouped by severity.
type CheckSummary struct {
	Name     string         `json:"name"`
	Category Category       `json:"category"`
	Total    int            `json:"total"`
	Severity SeverityCounts `json:"severity"`
}

type Check interface {
	Info() CheckInfo
	Supports(doc *Document) bool
	Apply(ctx context.Context, doc *Document) Issue
}

func NewPassIssue(check Check, message string) Issue {
	info := check.Info()
	return Issue{
		CheckName: info.Name,
		Category:  info.Category,
		Severity:  SeveritySuccess,
		Message:   message,
	}
}

func NewPassIssueWithDetails(check Check, message string, details map[string]any) Issue {
	issue := NewPassIssue(check, message)
	issue.Details = details
	return issue
}

func NewFailIssue(check Check, severity Severity, message string, details map[string]any) Issue {
	info := check.Info()
	return Issue{
		CheckName: info.Name,
		Category:  info.Category,
		Severity:  severity,
		Message:   message,
		Details:   details,
	}
}
