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

func (c *EnforceHTTPSCheck) Apply(ctx context.Context, doc *domain.Document) domain.Issue {
	node, err := html.Parse(bytes.NewReader(doc.Body))
	if err != nil {
		return domain.NewFailIssue(
			c,
			domain.SeverityError,
			fmt.Sprintf("Error during HTTPS enforcement check: %s", err.Error()),
			nil,
		)
	}

	builder := domain.NewIssueBuilder()

	if !strings.HasPrefix(doc.URL, "https://") {
		builder.Add(domain.Finding{
			Type:     "page_insecure",
			Severity: c.PageInsecureSeverity,
			Message:  "The page itself is not served over a secure HTTPS connection.",
			Data: map[string]any{
				"page_url": doc.URL,
			},
		})
	}

	insecureLinks := c.findInsecureLinks(node)
	for _, href := range insecureLinks {
		builder.Add(domain.Finding{
			Type:     "link_insecure",
			Severity: c.LinkInsecureSeverity,
			Message:  "Navigational link points to an insecure HTTP URL.",
			Data: map[string]any{
				"link_href": href,
			},
		})
	}

	if builder.HasFindings() {
		return domain.NewFailIssue(c, builder.Severity(), c.summary(builder), builder.Details())
	}

	return domain.NewPassIssue(
		c,
		"The page and all its navigational links use secure HTTPS.",
	)
}

// summary renders a short message for the aggregated issue from the kinds of
// findings collected.
func (c *EnforceHTTPSCheck) summary(builder *domain.IssueBuilder) string {
	var parts []string
	if builder.CountByType("page_insecure") > 0 {
		parts = append(parts, "The page is not served over HTTPS")
	}
	if count := builder.CountByType("link_insecure"); count > 0 {
		parts = append(parts, fmt.Sprintf("%d navigational link(s) point to an insecure HTTP URL", count))
	}
	return strings.Join(parts, "; ") + "."
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
