package checks

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/vvb13a/goaudit/data"

	"golang.org/x/net/html"
)

type CanonicalURLCheck struct {
	MissingSeverity    data.Severity
	MultipleSeverity   data.Severity
	InvalidURLSeverity data.Severity
}

func NewCanonicalURLCheck() *CanonicalURLCheck {
	return &CanonicalURLCheck{
		MissingSeverity:    data.SeverityWarning,
		MultipleSeverity:   data.SeverityError,
		InvalidURLSeverity: data.SeverityError,
	}
}

func (c *CanonicalURLCheck) Name() string {
	return "canonical_url"
}

func (c *CanonicalURLCheck) Checklist() string {
	return "seo"
}

func (c *CanonicalURLCheck) Supports(doc *data.Document) bool {
	return doc.IsHTML()
}

func (c *CanonicalURLCheck) Apply(ctx context.Context, doc *data.Document) []data.Issue {
	node, err := html.Parse(bytes.NewReader(doc.Body))
	if err != nil {
		return []data.Issue{
			data.NewFailIssue(
				c,
				data.SeverityError,
				fmt.Sprintf("Error during canonical URL check: %s", err.Error()),
				nil,
			),
		}
	}

	canonicalNodes := c.findCanonicalTags(node)
	nodeCount := len(canonicalNodes)

	if nodeCount == 0 {
		return []data.Issue{
			data.NewFailIssue(
				c,
				c.MissingSeverity,
				"The canonical link tag (<link rel=\"canonical\">) is missing.",
				map[string]any{
					"issue_type": "missing",
				},
			),
		}
	}

	if nodeCount > 1 {
		return []data.Issue{
			data.NewFailIssue(
				c,
				c.MultipleSeverity,
				fmt.Sprintf("Multiple canonical link tags found (%d). There must be exactly one.", nodeCount),
				map[string]any{
					"issue_type": "multiple",
					"count":      nodeCount,
				},
			),
		}
	}

	href := strings.TrimSpace(canonicalNodes[0])

	if href == "" {
		return []data.Issue{
			data.NewFailIssue(
				c,
				c.InvalidURLSeverity,
				"The canonical link tag has an empty href attribute.",
				map[string]any{
					"issue_type": "empty_href",
				},
			),
		}
	}

	if !strings.HasPrefix(href, "http://") && !strings.HasPrefix(href, "https://") {
		return []data.Issue{
			data.NewFailIssue(
				c,
				c.InvalidURLSeverity,
				"The canonical link's href attribute must be an absolute URL.",
				map[string]any{
					"issue_type": "relative_url",
					"href":       href,
				},
			),
		}
	}

	parsedURL, err := url.ParseRequestURI(href)
	if err != nil || parsedURL.Host == "" {
		return []data.Issue{
			data.NewFailIssue(
				c,
				c.InvalidURLSeverity,
				"The canonical link has a malformed URL in its href attribute.",
				map[string]any{
					"issue_type": "malformed_url",
					"href":       href,
				},
			),
		}
	}

	return []data.Issue{
		data.NewPassIssue(
			c,
			"The canonical link tag is present and valid.",
			map[string]any{
				"href": href,
			},
		),
	}
}

func (c *CanonicalURLCheck) findCanonicalTags(root *html.Node) []string {
	var canonicalHrefs []string

	var traverse func(n *html.Node, inHead bool)
	traverse = func(n *html.Node, inHead bool) {
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, "head") {
			inHead = true
		}

		if inHead && n.Type == html.ElementNode && strings.EqualFold(n.Data, "link") {
			var isCanonical bool
			var href string

			for _, attr := range n.Attr {
				if strings.EqualFold(attr.Key, "rel") && strings.EqualFold(strings.TrimSpace(attr.Val), "canonical") {
					isCanonical = true
				}
				if strings.EqualFold(attr.Key, "href") {
					href = attr.Val
				}
			}

			if isCanonical {
				canonicalHrefs = append(canonicalHrefs, href)
			}
		}

		for child := n.FirstChild; child != nil; child = child.NextSibling {
			traverse(child, inHead)
		}
	}

	traverse(root, false)
	return canonicalHrefs
}
