package checks

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/vvb13a/goaudit/data"

	"golang.org/x/net/html"
)

type EnforceHTTPSCheck struct {
	PageInsecureSeverity data.Severity
	LinkInsecureSeverity data.Severity
}

func NewEnforceHTTPSCheck() *EnforceHTTPSCheck {
	return &EnforceHTTPSCheck{
		PageInsecureSeverity: data.SeverityError,
		LinkInsecureSeverity: data.SeverityError,
	}
}

func (c *EnforceHTTPSCheck) Name() string {
	return "enforce_https"
}

func (c *EnforceHTTPSCheck) Checklist() string {
	return "security"
}

func (c *EnforceHTTPSCheck) Supports(doc *data.Document) bool {
	return doc.IsHTML()
}

func (c *EnforceHTTPSCheck) Apply(ctx context.Context, doc *data.Document) []data.Issue {
	var issues []data.Issue

	if !strings.HasPrefix(doc.URL, "https://") {
		issues = append(issues, data.NewFailIssue(
			c,
			c.PageInsecureSeverity,
			"The page itself is not served over a secure HTTPS connection.",
			map[string]any{
				"issue_type": "page_insecure",
				"page_url":   doc.URL,
			},
		))
	}

	node, err := html.Parse(bytes.NewReader(doc.Body))
	if err != nil {
		return []data.Issue{
			data.NewFailIssue(
				c,
				data.SeverityError,
				fmt.Sprintf("Error during HTTPS enforcement check: %s", err.Error()),
				nil,
			),
		}
	}

	insecureLinks := c.findInsecureLinks(node)
	for _, href := range insecureLinks {
		issues = append(issues, data.NewFailIssue(
			c,
			c.LinkInsecureSeverity,
			"Navigational link points to an insecure HTTP URL.",
			map[string]any{
				"issue_type": "link_insecure",
				"link_href":  href,
			},
		))
	}

	if len(issues) > 0 {
		return issues
	}

	return []data.Issue{
		data.NewPassIssue(
			c,
			"The page and all its navigational links use secure HTTPS.",
			nil,
		),
	}
}

func (c *EnforceHTTPSCheck) findInsecureLinks(root *html.Node) []string {
	var insecureLinks []string

	var traverse func(n *html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, "a") {
			for _, attr := range n.Attr {
				if strings.EqualFold(attr.Key, "href") {
					href := strings.TrimSpace(attr.Val)
					if strings.HasPrefix(href, "http://") {
						insecureLinks = append(insecureLinks, href)
					}
					break
				}
			}
		}

		for child := n.FirstChild; child != nil; child = child.NextSibling {
			traverse(child)
		}
	}

	traverse(root)
	return insecureLinks
}
