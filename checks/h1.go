package checks

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/vvb13a/goaudit/domain"

	"golang.org/x/net/html"
)

type H1Check struct {
	MaxHeadingLength             *int
	MinHeadingLength             *int
	LengthWarningOverage         *int
	MinorLengthDeviationSeverity domain.Severity
	MajorLengthDeviationSeverity domain.Severity
	MultipleSeverity             domain.Severity
	MissingEmptySeverity         domain.Severity
}

func NewH1Check() *H1Check {
	return &H1Check{
		MaxHeadingLength:             new(70),
		MinHeadingLength:             new(20),
		LengthWarningOverage:         new(15),
		MinorLengthDeviationSeverity: domain.SeverityNotice,
		MajorLengthDeviationSeverity: domain.SeverityWarning,
		MultipleSeverity:             domain.SeverityError,
		MissingEmptySeverity:         domain.SeverityError,
	}
}

func (c *H1Check) Info() domain.CheckInfo {
	return domain.CheckInfo{
		Name:        "h1",
		Description: "Ensures the page has exactly one non-empty, reasonably sized <h1> heading.",
		Category:    domain.CategorySEO,
	}
}

func (c *H1Check) Supports(doc *domain.Document) bool {
	return doc.IsHTML()
}

func (c *H1Check) Apply(ctx context.Context, doc *domain.Document) []domain.Issue {
	var detectedIssues []domain.Issue

	root, err := html.Parse(bytes.NewReader(doc.Body))
	if err != nil {
		return []domain.Issue{
			domain.NewFailIssue(
				c,
				domain.SeverityError,
				fmt.Sprintf("Error during heading check: %s", err.Error()),
				nil,
			),
		}
	}

	h1Nodes := c.findH1Nodes(root)
	headingNodeCount := len(h1Nodes)

	if headingNodeCount == 0 {
		return []domain.Issue{
			domain.NewFailIssue(
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
		detectedIssues = append(detectedIssues, domain.NewFailIssue(
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
		detectedIssues = append(detectedIssues, domain.NewFailIssue(
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

	return []domain.Issue{
		domain.NewPassIssue(
			c,
			"Heading is present and has appropriate length.",
		),
	}
}

func (c *H1Check) checkLength(detectedIssues *[]domain.Issue, headingContent string) {
	headingLength := utf8.RuneCountInString(headingContent)

	if c.MaxHeadingLength != nil && headingLength > *c.MaxHeadingLength {
		overage := headingLength - *c.MaxHeadingLength

		level := c.MinorLengthDeviationSeverity
		if c.LengthWarningOverage != nil && overage >= *c.LengthWarningOverage {
			level = c.MajorLengthDeviationSeverity
		}

		message := fmt.Sprintf("Title length (%d) exceeds the ideal maximum of %d by %d characters.", headingLength, *c.MaxHeadingLength, overage)

		*detectedIssues = append(*detectedIssues, domain.NewFailIssue(
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

		*detectedIssues = append(*detectedIssues, domain.NewFailIssue(
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
