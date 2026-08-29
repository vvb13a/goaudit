package checks

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/vvb13a/goaudit/data"

	"golang.org/x/net/html"
)

type H1Check struct {
	MaxHeadingLength             *int
	MinHeadingLength             *int
	LengthWarningOverage         *int
	MinorLengthDeviationSeverity data.Severity
	MajorLengthDeviationSeverity data.Severity
	MultipleSeverity             data.Severity
	MissingEmptySeverity         data.Severity
}

func intPtr(i int) *int {
	return &i
}

func NewH1Check() *H1Check {
	return &H1Check{
		MaxHeadingLength:             intPtr(70),
		MinHeadingLength:             intPtr(20),
		LengthWarningOverage:         intPtr(15),
		MinorLengthDeviationSeverity: data.SeverityNotice,
		MajorLengthDeviationSeverity: data.SeverityWarning,
		MultipleSeverity:             data.SeverityError,
		MissingEmptySeverity:         data.SeverityError,
	}
}

func (c *H1Check) Name() string {
	return "h1"
}

func (c *H1Check) Checklist() string {
	return "seo"
}

func (c *H1Check) Supports(doc *data.Document) bool {
	return doc.IsHTML()
}

func (c *H1Check) Apply(ctx context.Context, doc *data.Document) []data.Issue {
	var detectedIssues []data.Issue

	root, err := html.Parse(bytes.NewReader(doc.Body))
	if err != nil {
		return []data.Issue{
			data.NewFailIssue(
				c,
				data.SeverityError,
				fmt.Sprintf("Error during heading check: %s", err.Error()),
				nil,
			),
		}
	}

	h1Nodes := c.findH1Nodes(root)
	headingNodeCount := len(h1Nodes)

	if headingNodeCount == 0 {
		return []data.Issue{
			data.NewFailIssue(
				c,
				c.MissingEmptySeverity,
				"Missing <h1> tag.",
				map[string]any{
					"issue_type": "missing",
				},
			),
		}
	}

	if headingNodeCount > 1 {
		detectedIssues = append(detectedIssues, data.NewFailIssue(
			c,
			c.MultipleSeverity,
			"Multiple <h1> tags found.",
			map[string]any{
				"issue_type": "multiple",
				"count":      headingNodeCount,
			},
		))
	}

	headingContent := strings.TrimSpace(c.extractText(h1Nodes[0]))
	if headingContent == "" {
		detectedIssues = append(detectedIssues, data.NewFailIssue(
			c,
			c.MissingEmptySeverity,
			"<h1> tag is empty or contains only whitespace.",
			map[string]any{
				"issue_type": "empty",
			},
		))
	} else {
		c.checkLength(&detectedIssues, headingContent)
	}

	if len(detectedIssues) > 0 {
		return detectedIssues
	}

	return []data.Issue{
		data.NewPassIssue(
			c,
			"Heading is present and has appropriate length.",
			nil,
		),
	}
}

func (c *H1Check) checkLength(detectedIssues *[]data.Issue, headingContent string) {
	headingLength := utf8.RuneCountInString(headingContent)

	if c.MaxHeadingLength != nil && headingLength > *c.MaxHeadingLength {
		overage := headingLength - *c.MaxHeadingLength

		level := c.MinorLengthDeviationSeverity
		if c.LengthWarningOverage != nil && overage >= *c.LengthWarningOverage {
			level = c.MajorLengthDeviationSeverity
		}

		message := fmt.Sprintf("Title length (%d) exceeds the ideal maximum of %d by %d characters.", headingLength, *c.MaxHeadingLength, overage)

		*detectedIssues = append(*detectedIssues, data.NewFailIssue(
			c,
			level,
			message,
			map[string]any{
				"issue_type": "length_max",
				"title":      headingContent,
				"length":     headingLength,
				"limit":      *c.MaxHeadingLength,
				"overage":    overage,
			},
		))
	}

	if c.MinHeadingLength != nil && headingLength < *c.MinHeadingLength {
		message := fmt.Sprintf("Title length (%d) is less than the recommended minimum of %d.", headingLength, *c.MinHeadingLength)

		*detectedIssues = append(*detectedIssues, data.NewFailIssue(
			c,
			c.MajorLengthDeviationSeverity,
			message,
			map[string]any{
				"issue_type": "length_min",
				"title":      headingContent,
				"length":     headingLength,
				"limit":      *c.MinHeadingLength,
			},
		))
	}
}

func (c *H1Check) findH1Nodes(root *html.Node) []*html.Node {
	var nodes []*html.Node

	var traverse func(n *html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, "h1") {
			nodes = append(nodes, n)
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			traverse(child)
		}
	}

	traverse(root)
	return nodes
}

func (c *H1Check) extractText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var buf strings.Builder
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		buf.WriteString(c.extractText(child))
	}
	return buf.String()
}
