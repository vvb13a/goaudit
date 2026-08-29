package checks

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/vvb13a/goaudit/data"

	"golang.org/x/net/html"
)

type RobotsMetaCheck struct {
	NoindexSeverity  data.Severity
	NofollowSeverity data.Severity
	MultipleSeverity data.Severity
}

func NewRobotsMetaCheck() *RobotsMetaCheck {
	return &RobotsMetaCheck{
		NoindexSeverity:  data.SeverityInfo,
		NofollowSeverity: data.SeverityInfo,
		MultipleSeverity: data.SeverityError,
	}
}

func (c *RobotsMetaCheck) Name() string {
	return "robots_meta"
}

func (c *RobotsMetaCheck) Checklist() string {
	return "seo"
}

func (c *RobotsMetaCheck) Supports(doc *data.Document) bool {
	return doc.IsHTML()
}

func (c *RobotsMetaCheck) Apply(ctx context.Context, doc *data.Document) []data.Issue {
	root, err := html.Parse(bytes.NewReader(doc.Body))
	if err != nil {
		return []data.Issue{
			data.NewFailIssue(
				c,
				data.SeverityError,
				fmt.Sprintf("Error during robots meta check: %s", err.Error()),
				nil,
			),
		}
	}

	robotsNodes := c.findRobotsMetaTags(root)
	robotsNodeCount := len(robotsNodes)

	if robotsNodeCount == 0 {
		return []data.Issue{
			data.NewPassIssue(
				c,
				"No robots meta tag found; crawlers will use default behavior.",
				nil,
			),
		}
	}

	var detectedIssues []data.Issue

	if robotsNodeCount > 1 {
		detectedIssues = append(detectedIssues, data.NewFailIssue(
			c,
			c.MultipleSeverity,
			"Multiple robots meta tags found. Directives should be consolidated into one tag.",
			map[string]any{
				"issue_type": "multiple",
				"count":      robotsNodeCount,
			},
		))
	}

	content := strings.ToLower(strings.TrimSpace(robotsNodes[0]))

	if strings.Contains(content, "noindex") {
		detectedIssues = append(detectedIssues, data.NewFailIssue(
			c,
			c.NoindexSeverity,
			"A 'noindex' directive was found in the robots meta tag, which will prevent this page from being indexed by search engines.",
			map[string]any{
				"issue_type": "noindex_found",
				"content":    content,
			},
		))
	}

	if strings.Contains(content, "nofollow") {
		detectedIssues = append(detectedIssues, data.NewFailIssue(
			c,
			c.NofollowSeverity,
			"A 'nofollow' directive was found in the robots meta tag, which will prevent search engines from following links on this page.",
			map[string]any{
				"issue_type": "nofollow_found",
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
			"The robots meta tag is present and does not contain 'noindex' or 'nofollow'.",
			nil,
		),
	}
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
