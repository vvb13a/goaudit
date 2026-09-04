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

func (c *H1Check) Apply(ctx context.Context, doc *domain.Document) domain.Issue {
	root, err := html.Parse(bytes.NewReader(doc.Body))
	if err != nil {
		return domain.NewFailIssue(
			c,
			domain.SeverityError,
			fmt.Sprintf("Error during heading check: %s", err.Error()),
			nil,
		)
	}

	h1Nodes := c.findH1Nodes(root)
	headingNodeCount := len(h1Nodes)

	if headingNodeCount == 0 {
		return domain.NewFailIssue(
			c,
			c.MissingEmptySeverity,
			"Missing <h1> tag.",
			map[string]any{
				"issue_type": "missing",
			},
		)
	}

	builder := domain.NewIssueBuilder()

	if headingNodeCount > 1 {
		builder.Add(domain.Finding{
			Type:     "multiple",
			Severity: c.MultipleSeverity,
			Message:  "Multiple <h1> tags found.",
			Data: map[string]any{
				"count": headingNodeCount,
			},
		})
	}

	headingContent := strings.TrimSpace(c.extractText(h1Nodes[0]))
	if headingContent == "" {
		builder.Add(domain.Finding{
			Type:     "empty",
			Severity: c.MissingEmptySeverity,
			Message:  "<h1> tag is empty or contains only whitespace.",
		})
	} else {
		c.checkLength(builder, headingContent)
	}

	if builder.HasFindings() {
		return domain.NewFailIssue(c, builder.Severity(), c.summary(builder), builder.Details())
	}

	return domain.NewPassIssue(
		c,
		"Heading is present and has appropriate length.",
	)
}

// summary renders a short message for the aggregated issue from the kinds of
// findings collected.
func (c *H1Check) summary(builder *domain.IssueBuilder) string {
	var parts []string
	if builder.CountByType("multiple") > 0 {
		parts = append(parts, "Multiple <h1> tags found.")
	}
	if builder.CountByType("empty") > 0 {
		parts = append(parts, "The <h1> tag is empty or contains only whitespace.")
	}
	if builder.CountByType("length_max") > 0 {
		parts = append(parts, "The <h1> heading is longer than the recommended maximum.")
	}
	if builder.CountByType("length_min") > 0 {
		parts = append(parts, "The <h1> heading is shorter than the recommended minimum.")
	}
	return strings.Join(parts, " ")
}

func (c *H1Check) checkLength(builder *domain.IssueBuilder, headingContent string) {
	headingLength := utf8.RuneCountInString(headingContent)

	if c.MaxHeadingLength != nil && headingLength > *c.MaxHeadingLength {
		overage := headingLength - *c.MaxHeadingLength

		level := c.MinorLengthDeviationSeverity
		if c.LengthWarningOverage != nil && overage >= *c.LengthWarningOverage {
			level = c.MajorLengthDeviationSeverity
		}

		message := fmt.Sprintf("Heading length (%d) exceeds the ideal maximum of %d by %d characters.", headingLength, *c.MaxHeadingLength, overage)

		builder.Add(domain.Finding{
			Type:     "length_max",
			Severity: level,
			Message:  message,
			Data: map[string]any{
				"heading": headingContent,
				"length":  headingLength,
				"limit":   *c.MaxHeadingLength,
				"overage": overage,
			},
		})
	}

	if c.MinHeadingLength != nil && headingLength < *c.MinHeadingLength {
		message := fmt.Sprintf("Heading length (%d) is less than the recommended minimum of %d.", headingLength, *c.MinHeadingLength)

		builder.Add(domain.Finding{
			Type:     "length_min",
			Severity: c.MajorLengthDeviationSeverity,
			Message:  message,
			Data: map[string]any{
				"heading": headingContent,
				"length":  headingLength,
				"limit":   *c.MinHeadingLength,
			},
		})
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
