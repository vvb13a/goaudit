package checks

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/vvb13a/goaudit/domain"

	"golang.org/x/net/html"
)

type EnforceHTTPSCheck struct {
	PageInsecureSeverity domain.Severity
	LinkInsecureSeverity domain.Severity
}

func NewEnforceHTTPSCheck() *EnforceHTTPSCheck {
	return &EnforceHTTPSCheck{
		PageInsecureSeverity: domain.SeverityError,
		LinkInsecureSeverity: domain.SeverityError,
	}
}

func (c *EnforceHTTPSCheck) Info() domain.CheckInfo {
	return domain.CheckInfo{
		Name:        "enforce_https",
		Description: "Ensures the page and its navigational links are served over HTTPS.",
		Category:    domain.CategorySecurity,
	}
}

func (c *EnforceHTTPSCheck) Supports(doc *domain.Document) bool {
	return doc.IsHTML()
}

func (c *EnforceHTTPSCheck) Apply(ctx context.Context, doc *domain.Document) []domain.Issue {
	var issues []domain.Issue

	if !strings.HasPrefix(doc.URL, "https://") {
		issues = append(issues, domain.NewFailIssue(
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
		return []domain.Issue{
			domain.NewFailIssue(
				c,
				domain.SeverityError,
				fmt.Sprintf("Error during HTTPS enforcement check: %s", err.Error()),
				nil,
			),
		}
	}

	insecureLinks := c.findInsecureLinks(node)
	for _, href := range insecureLinks {
		issues = append(issues, domain.NewFailIssue(
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

	return []domain.Issue{
		domain.NewPassIssue(
			c,
			"The page and all its navigational links use secure HTTPS.",
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
