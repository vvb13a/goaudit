package domain

import "context"

type CheckInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Category    Category `json:"category"`
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
