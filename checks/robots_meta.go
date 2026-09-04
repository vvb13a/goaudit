package checks

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/vvb13a/goaudit/domain"

	"golang.org/x/net/html"
)

type RobotsMetaCheck struct {
	NoindexSeverity  domain.Severity
	NofollowSeverity domain.Severity
	MultipleSeverity domain.Severity
}

func NewRobotsMetaCheck() *RobotsMetaCheck {
	return &RobotsMetaCheck{
		NoindexSeverity:  domain.SeverityInfo,
		NofollowSeverity: domain.SeverityInfo,
		MultipleSeverity: domain.SeverityError,
	}
}

func (c *RobotsMetaCheck) Info() domain.CheckInfo {
	return domain.CheckInfo{
		Name:        "robots_meta",
		Description: "Reports on robots meta directives and duplicate tag issues.",
		Category:    domain.CategorySEO,
	}
}

func (c *RobotsMetaCheck) Supports(doc *domain.Document) bool {
	return doc.IsHTML()
}

func (c *RobotsMetaCheck) Apply(ctx context.Context, doc *domain.Document) domain.Issue {
	root, err := html.Parse(bytes.NewReader(doc.Body))
	if err != nil {
		return domain.NewFailIssue(
			c,
			domain.SeverityError,
			fmt.Sprintf("Error during robots meta check: %s", err.Error()),
			nil,
		)
	}

	robotsNodes := c.findRobotsMetaTags(root)
	robotsNodeCount := len(robotsNodes)

	if robotsNodeCount == 0 {
		return domain.NewPassIssue(
			c,
			"No robots meta tag found; crawlers will use default behavior.",
		)
	}

	builder := domain.NewIssueBuilder()

	if robotsNodeCount > 1 {
		builder.Add(domain.Finding{
			Type:     "multiple",
			Severity: c.MultipleSeverity,
			Message:  "Multiple robots meta tags found. Directives should be consolidated into one tag.",
			Data: map[string]any{
				"count": robotsNodeCount,
			},
		})
	}

	content := strings.ToLower(strings.TrimSpace(robotsNodes[0]))

	if strings.Contains(content, "noindex") {
		builder.Add(domain.Finding{
			Type:     "noindex_found",
			Severity: c.NoindexSeverity,
			Message:  "A 'noindex' directive was found in the robots meta tag, which will prevent this page from being indexed by search engines.",
			Data: map[string]any{
				"content": content,
			},
		})
	}

	if strings.Contains(content, "nofollow") {
		builder.Add(domain.Finding{
			Type:     "nofollow_found",
			Severity: c.NofollowSeverity,
			Message:  "A 'nofollow' directive was found in the robots meta tag, which will prevent search engines from following links on this page.",
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
		"The robots meta tag is present and does not contain 'noindex' or 'nofollow'.",
	)
}

// summary renders a short message for the aggregated issue from the kinds of
// findings collected.
func (c *RobotsMetaCheck) summary(builder *domain.IssueBuilder) string {
	var parts []string
	if builder.CountByType("multiple") > 0 {
		parts = append(parts, "Multiple robots meta tags found.")
	}
	if builder.CountByType("noindex_found") > 0 {
		parts = append(parts, "The page is blocked from indexing with 'noindex'.")
	}
	if builder.CountByType("nofollow_found") > 0 {
		parts = append(parts, "Search engines are told not to follow links with 'nofollow'.")
	}
	return strings.Join(parts, " ")
}

func (c *RobotsMetaCheck) findRobotsMetaTags(root *html.Node) []string {
	var robotsTags []string

	var traverse func(n *html.Node, inHead bool)
	traverse = func(n *html.Node, inHead bool) {
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, "head") {
			inHead = true
		}

		if inHead && n.Type == html.ElementNode && strings.EqualFold(n.Data, "meta") {
			var isRobots bool
			var content string

			for _, attr := range n.Attr {
				key := strings.ToLower(attr.Key)
				val := strings.TrimSpace(attr.Val)

				if key == "name" && strings.EqualFold(val, "robots") {
					isRobots = true
				}
				if key == "content" {
					content = attr.Val
				}
			}

			if isRobots {
				robotsTags = append(robotsTags, content)
			}
		}

		for child := n.FirstChild; child != nil; child = child.NextSibling {
			traverse(child, inHead)
		}
	}

	traverse(root, false)
	return robotsTags
}
