package checks

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/vvb13a/goaudit/data"

	"golang.org/x/net/html"
)

var initialScaleRegex = regexp.MustCompile(`initial-scale\s*=\s*1(\.0)?`)

type ViewportCheck struct {
	MissingSeverity       data.Severity
	MisconfiguredSeverity data.Severity
}

func NewViewportCheck() *ViewportCheck {
	return &ViewportCheck{
		MissingSeverity:       data.SeverityError,
		MisconfiguredSeverity: data.SeverityError,
	}
}

func (c *ViewportCheck) Name() string {
	return "viewport"
}

func (c *ViewportCheck) Checklist() string {
	return "mobile"
}

func (c *ViewportCheck) Supports(doc *data.Document) bool {
	return doc.IsHTML()
}

func (c *ViewportCheck) Apply(ctx context.Context, doc *data.Document) []data.Issue {
	root, err := html.Parse(bytes.NewReader(doc.Body))
	if err != nil {
		return []data.Issue{
			data.NewFailIssue(
				c,
				data.SeverityError,
				fmt.Sprintf("Error during viewport check: %s", err.Error()),
				nil,
			),
		}
	}

	viewportContents := c.findViewportMetaTags(root)

	if len(viewportContents) == 0 {
		return []data.Issue{
			data.NewFailIssue(
				c,
				c.MissingSeverity,
				"The viewport meta tag (<meta name=\"viewport\">) is missing.",
				map[string]any{
					"issue_type": "missing",
				},
			),
		}
	}

	content := strings.TrimSpace(viewportContents[0])

	if content == "" {
		return []data.Issue{
			data.NewFailIssue(
				c,
				c.MisconfiguredSeverity,
				"The viewport meta tag has an empty content attribute.",
				map[string]any{
					"issue_type": "empty_content",
				},
			),
		}
	}

	var detectedIssues []data.Issue

	if !strings.Contains(content, "width=device-width") {
		detectedIssues = append(detectedIssues, data.NewFailIssue(
			c,
			c.MisconfiguredSeverity,
			"Viewport 'content' attribute is missing the required 'width=device-width' directive.",
			map[string]any{
				"issue_type": "missing_width",
				"content":    content,
			},
		))
	}

	if !initialScaleRegex.MatchString(content) {
		detectedIssues = append(detectedIssues, data.NewFailIssue(
			c,
			c.MisconfiguredSeverity,
			"Viewport 'content' attribute is missing the required 'initial-scale=1.0' directive.",
			map[string]any{
				"issue_type": "missing_initial_scale",
				"content":    content,
			},
		))
	}

	if len(detectedIssues) > 0 {
		return detectedIssues
	}

	return []data.Issue{
		data.NewPassIssue(
			c,
			"The viewport meta tag is present and correctly configured.",
			nil,
		),
	}
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
