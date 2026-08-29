package data

import "context"

type Check interface {
	Name() string
	Checklist() string
	Supports(doc *Document) bool
	Apply(ctx context.Context, doc *Document) []Issue
}

func NewPassIssue(check Check, message string, details map[string]any) Issue {
	return Issue{
		CheckName: check.Name(),
		Checklist: check.Checklist(),
		Severity:  SeveritySuccess,
		Passed:    true,
		Message:   message,
		Details:   details,
	}
}

func NewFailIssue(check Check, severity Severity, message string, details map[string]any) Issue {
	return Issue{
		CheckName: check.Name(),
		Checklist: check.Checklist(),
		Severity:  severity,
		Passed:    false,
		Message:   message,
		Details:   details,
	}
}
