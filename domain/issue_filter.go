package domain

// IssueFilter narrows a stored issue query. Each slice is an OR-set within
// its dimension and the dimensions combine with AND, so e.g. severities
// [error, fatal] and lifecycles [new] match issues that are new and either
// an error or fatal. A nil or empty slice leaves its dimension unfiltered,
// which makes the zero-value filter match every issue. The two text fields
// match issues whose URL or message contains the given substring.
type IssueFilter struct {
	Severities []Severity
	Lifecycles []IssueLifecycle
	CheckNames []string
	Categories []Category

	// URLContains and MessageContains are case-insensitive substring matches
	// over the issue's page URL and message. Empty disables the match.
	URLContains     string
	MessageContains string
}
