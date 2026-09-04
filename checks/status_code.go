package checks

import (
	"context"
	"fmt"

	"github.com/vvb13a/goaudit/domain"
)

type StatusCodeCheck struct {
	RedirectSeverity         domain.Severity
	ClientErrorSeverity      domain.Severity
	ServerErrorSeverity      domain.Severity
	UnexpectedStatusSeverity domain.Severity
}

func NewStatusCodeCheck() *StatusCodeCheck {
	return &StatusCodeCheck{
		RedirectSeverity:         domain.SeverityWarning,
		ClientErrorSeverity:      domain.SeverityError,
		ServerErrorSeverity:      domain.SeverityFatal,
		UnexpectedStatusSeverity: domain.SeverityError,
	}
}

func (c *StatusCodeCheck) Info() domain.CheckInfo {
	return domain.CheckInfo{
		Name:        "status_code",
		Description: "Evaluates the HTTP response status code of the page.",
		Category:    domain.CategoryGeneral,
	}
}

func (c *StatusCodeCheck) Supports(doc *domain.Document) bool {
	return true
}

func (c *StatusCodeCheck) Apply(ctx context.Context, doc *domain.Document) domain.Issue {
	statusCode := doc.StatusCode
	details := map[string]any{
		"status_code": statusCode,
	}

	if statusCode >= 200 && statusCode < 300 {
		return domain.NewPassIssueWithDetails(
			c,
			fmt.Sprintf("Status code (%d) indicates success.", statusCode),
			details,
		)
	}

	if statusCode >= 300 && statusCode < 400 {
		if c.RedirectSeverity == domain.SeveritySuccess {
			return domain.NewPassIssueWithDetails(
				c,
				fmt.Sprintf("Page redirected (%d) as expected.", statusCode),
				details,
			)
		}

		details["redirect_location"] = doc.Headers.Get("Location")

		return domain.NewFailIssue(
			c,
			c.RedirectSeverity,
			fmt.Sprintf("Page redirected (%d).", statusCode),
			details,
		)
	}

	if statusCode >= 400 && statusCode < 500 {
		return domain.NewFailIssue(
			c,
			c.ClientErrorSeverity,
			fmt.Sprintf("Client error response (%d).", statusCode),
			details,
		)
	}

	if statusCode >= 500 && statusCode < 600 {
		return domain.NewFailIssue(
			c,
			c.ServerErrorSeverity,
			fmt.Sprintf("Server error response (%d).", statusCode),
			details,
		)
	}

	return domain.NewFailIssue(
		c,
		c.UnexpectedStatusSeverity,
		fmt.Sprintf("Received unexpected status code: %d.", statusCode),
		details,
	)
}
