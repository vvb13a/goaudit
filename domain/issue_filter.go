package domain

// IssueFilter narrows a stored issue query. Each slice is an OR-set within
// its dimension and the dimensions combine with AND, so e.g. severities
// [error, fatal] and lifecycles [new] match issues that are new and either
// an error or fatal. A nil or empty slice leaves its dimension unfiltered,
// which makes the zero-value filter match every issue.
type IssueFilter struct {
	Severities []Severity
	Lifecycles []IssueLifecycle
	CheckNames []string
	Categories []Category
}
