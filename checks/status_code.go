package checks

import (
	"context"
	"fmt"

	"github.com/vvb13a/goaudit/data"
)

type StatusCodeCheck struct {
	RedirectSeverity         data.Severity
	ClientErrorSeverity      data.Severity
	ServerErrorSeverity      data.Severity
	UnexpectedStatusSeverity data.Severity
}

func NewStatusCodeCheck() *StatusCodeCheck {
	return &StatusCodeCheck{
		RedirectSeverity:         data.SeverityWarning,
		ClientErrorSeverity:      data.SeverityError,
		ServerErrorSeverity:      data.SeverityFatal,
		UnexpectedStatusSeverity: data.SeverityError,
	}
}

func (c *StatusCodeCheck) Name() string {
	return "status_code"
}

func (c *StatusCodeCheck) Checklist() string {
	return "resilience"
}

func (c *StatusCodeCheck) Supports(doc *data.Document) bool {
	return true
}

func (c *StatusCodeCheck) Apply(ctx context.Context, doc *data.Document) []data.Issue {
	statusCode := doc.StatusCode
	details := map[string]any{
		"status_code": statusCode,
	}

	if statusCode >= 200 && statusCode < 300 {
		return []data.Issue{
			data.NewPassIssue(
				c,
				fmt.Sprintf("Status code (%d) indicates success.", statusCode),
				details,
			),
		}
	}

	if statusCode >= 300 && statusCode < 400 {
		if c.RedirectSeverity == data.SeveritySuccess {
			return []data.Issue{
				data.NewPassIssue(
					c,
					fmt.Sprintf("Page redirected (%d) as expected.", statusCode),
					details,
				),
			}
		}

		details["redirect_location"] = doc.Headers.Get("Location")

		return []data.Issue{
			data.NewFailIssue(
				c,
				c.RedirectSeverity,
				fmt.Sprintf("Page redirected (%d).", statusCode),
				details,
			),
		}
	}

	if statusCode >= 400 && statusCode < 500 {
		return []data.Issue{
			data.NewFailIssue(
				c,
				c.ClientErrorSeverity,
				fmt.Sprintf("Client error response (%d).", statusCode),
				details,
			),
		}
	}

	if statusCode >= 500 && statusCode < 600 {
		return []data.Issue{
			data.NewFailIssue(
				c,
				c.ServerErrorSeverity,
				fmt.Sprintf("Server error response (%d).", statusCode),
				details,
			),
		}
	}

	return []data.Issue{
		data.NewFailIssue(
			c,
			c.UnexpectedStatusSeverity,
			fmt.Sprintf("Received unexpected status code: %d.", statusCode),
			details,
		),
	}
}
