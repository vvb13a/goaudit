package checks

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/vvb13a/goaudit/domain"

	"golang.org/x/net/html"
)

var initialScaleRegex = regexp.MustCompile(`initial-scale\s*=\s*1(\.0)?`)

type ViewportCheck struct {
	MissingSeverity       domain.Severity
	MisconfiguredSeverity domain.Severity
}

func NewViewportCheck() *ViewportCheck {
	return &ViewportCheck{
		MissingSeverity:       domain.SeverityError,
		MisconfiguredSeverity: domain.SeverityError,
	}
}

func (c *ViewportCheck) Info() domain.CheckInfo {
	return domain.CheckInfo{
		Name:        "viewport",
		Description: "Checks for a correctly configured viewport meta tag for mobile devices.",
		Category:    domain.CategorySEO,
	}
}

func (c *ViewportCheck) Supports(doc *domain.Document) bool {
	return doc.IsHTML()
}

func (c *ViewportCheck) Apply(ctx context.Context, doc *domain.Document) domain.Issue {
	root, err := html.Parse(bytes.NewReader(doc.Body))
	if err != nil {
		return domain.NewFailIssue(
			c,
			domain.SeverityError,
			fmt.Sprintf("Error during viewport check: %s", err.Error()),
			nil,
		)
	}

	viewportContents := c.findViewportMetaTags(root)

	if len(viewportContents) == 0 {
		return domain.NewFailIssue(
			c,
			c.MissingSeverity,
			"The viewport meta tag (<meta name=\"viewport\">) is missing.",
			map[string]any{
				"issue_type": "missing",
			},
		)
	}

	content := strings.TrimSpace(viewportContents[0])

	if content == "" {
		return domain.NewFailIssue(
			c,
			c.MisconfiguredSeverity,
			"The viewport meta tag has an empty content attribute.",
			map[string]any{
				"issue_type": "empty_content",
			},
		)
	}

	builder := domain.NewIssueBuilder()

	if !strings.Contains(content, "width=device-width") {
		builder.Add(domain.Finding{
			Type:     "missing_width",
			Severity: c.MisconfiguredSeverity,
			Message:  "Viewport 'content' attribute is missing the required 'width=device-width' directive.",
			Data: map[string]any{
				"content": content,
			},
		})
	}

	if !initialScaleRegex.MatchString(content) {
		builder.Add(domain.Finding{
			Type:     "missing_initial_scale",
			Severity: c.MisconfiguredSeverity,
			Message:  "Viewport 'content' attribute is missing the required 'initial-scale=1.0' directive.",
			Data: map[string]any{
				"content": content,
			},
		})
	}

	if builder.HasFindings() {
		return domain.NewFailIssue(c, builder.Severity(), c.summary(builder), builder.Details())
	}

	return domain.NewPassIssue(
		c,
		"The viewport meta tag is present and correctly configured.",
	)
}

// summary renders a short message for the aggregated issue from the kinds of
// findings collected.
func (c *ViewportCheck) summary(builder *domain.IssueBuilder) string {
	var parts []string
	if builder.CountByType("missing_width") > 0 {
		parts = append(parts, "The viewport is missing the 'width=device-width' directive.")
	}
	if builder.CountByType("missing_initial_scale") > 0 {
		parts = append(parts, "The viewport is missing the 'initial-scale=1.0' directive.")
	}
	return strings.Join(parts, " ")
}

func (c *ViewportCheck) findViewportMetaTags(root *html.Node) []string {
	var contents []string

	var traverse func(n *html.Node, inHead bool)
	traverse = func(n *html.Node, inHead bool) {
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, "head") {
			inHead = true
		}

		if inHead && n.Type == html.ElementNode && strings.EqualFold(n.Data, "meta") {
			var isViewport bool
			var content string

			for _, attr := range n.Attr {
				key := strings.ToLower(attr.Key)
				val := strings.TrimSpace(attr.Val)

				if key == "name" && strings.EqualFold(val, "viewport") {
					isViewport = true
				}
				if key == "content" {
					content = attr.Val
				}
			}

			if isViewport {
				contents = append(contents, content)
			}
		}

		for child := n.FirstChild; child != nil; child = child.NextSibling {
			traverse(child, inHead)
		}
	}

	traverse(root, false)
	return contents
}
