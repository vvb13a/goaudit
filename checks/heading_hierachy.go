package checks

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/vvb13a/goaudit/data"

	"golang.org/x/net/html"
)

type HeadingHierarchyCheck struct {
	Severity data.Severity
}

func NewHeadingHierarchyCheck() *HeadingHierarchyCheck {
	return &HeadingHierarchyCheck{
		Severity: data.SeverityWarning,
	}
}

func (c *HeadingHierarchyCheck) Name() string {
	return "heading_hierarchy"
}

func (c *HeadingHierarchyCheck) Checklist() string {
	return "seo"
}

func (c *HeadingHierarchyCheck) Supports(doc *data.Document) bool {
	return doc.IsHTML()
}

type headingNodeInfo struct {
	tag   string
	level int
	node  *html.Node
}

func (c *HeadingHierarchyCheck) Apply(ctx context.Context, doc *data.Document) []data.Issue {
	root, err := html.Parse(bytes.NewReader(doc.Body))
	if err != nil {
		return []data.Issue{
			data.NewFailIssue(
				c,
				data.SeverityError,
				fmt.Sprintf("Error during heading hierarchy check: %s", err.Error()),
				nil,
			),
		}
	}

	headings := c.collectHeadings(root)

	if len(headings) == 0 {
		return []data.Issue{
			data.NewPassIssue(
				c,
				"No headings present to check.",
				nil,
			),
		}
	}

	firstHeading := headings[0]
	if firstHeading.tag != "h1" {
		return []data.Issue{
			data.NewFailIssue(
				c,
				c.Severity,
				fmt.Sprintf("Heading hierarchy error: The first heading on the page should be an <h1> but found a <%s>.", firstHeading.tag),
				map[string]any{
					"issue_type":   "incorrect_first_heading",
					"found_tag":    firstHeading.tag,
					"expected_tag": "h1",
				},
			),
		}
	}

	lastLevel := 1
	for i := 1; i < len(headings); i++ {
		currentHeading := headings[i]
		currentLevel := currentHeading.level

		if currentLevel > (lastLevel + 1) {
			violatingTag := fmt.Sprintf("h%d", currentLevel)
			previousTag := fmt.Sprintf("h%d", lastLevel)

			return []data.Issue{
				data.NewFailIssue(
					c,
					c.Severity,
					fmt.Sprintf("Heading hierarchy error: A <%s> was found following a <%s>, skipping a level.", violatingTag, previousTag),
					map[string]any{
						"issue_type":     "skipped_level",
						"violating_tag":  violatingTag,
						"violating_text": strings.TrimSpace(c.extractText(currentHeading.node)),
						"previous_tag":   previousTag,
					},
				),
			}
		}

		lastLevel = currentLevel
	}

	return []data.Issue{
		data.NewPassIssue(
			c,
			"Heading hierarchy is valid.",
			nil,
		),
	}
}

func (c *HeadingHierarchyCheck) collectHeadings(root *html.Node) []headingNodeInfo {
	var headings []headingNodeInfo

	var traverse func(n *html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode {
			tag := strings.ToLower(n.Data)
			if len(tag) == 2 && tag[0] == 'h' && tag[1] >= '1' && tag[1] <= '6' {
				headings = append(headings, headingNodeInfo{
					tag:   tag,
					level: int(tag[1] - '0'),
					node:  n,
				})
			}
		}

		for child := n.FirstChild; child != nil; child = child.NextSibling {
			traverse(child)
		}
	}

	traverse(root)
	return headings
}

func (c *HeadingHierarchyCheck) extractText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var buf strings.Builder
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		buf.WriteString(c.extractText(child))
	}
	return buf.String()
}
