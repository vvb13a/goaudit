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

type TitleCheck struct {
	MinTitleLength               *int
	MaxTitleLength               *int
	LengthWarningOverage         int
	MinorLengthDeviationSeverity domain.Severity
	MajorLengthDeviationSeverity domain.Severity
	MissingEmptySeverity         domain.Severity
	MultipleSeverity             domain.Severity
}

func NewTitleCheck() *TitleCheck {
	return &TitleCheck{
		MinTitleLength:               new(10),
		MaxTitleLength:               new(60),
		LengthWarningOverage:         15,
		MinorLengthDeviationSeverity: domain.SeverityNotice,
		MajorLengthDeviationSeverity: domain.SeverityWarning,
		MissingEmptySeverity:         domain.SeverityError,
		MultipleSeverity:             domain.SeverityError,
	}
}

func (c *TitleCheck) Info() domain.CheckInfo {
	return domain.CheckInfo{
		Name:        "title",
		Description: "Checks for a single, non-empty <title> tag of appropriate length.",
		Category:    domain.CategorySEO,
	}
}

func (c *TitleCheck) Supports(doc *domain.Document) bool {
	return doc.IsHTML()
}

func (c *TitleCheck) Apply(ctx context.Context, doc *domain.Document) domain.Issue {
	root, err := html.Parse(bytes.NewReader(doc.Body))
	if err != nil {
		return domain.NewFailIssue(
			c,
			domain.SeverityError,
			fmt.Sprintf("Error during title check: %s", err.Error()),
			nil,
		)
	}

	titleNodes := c.findTitleNodes(root)
	titleNodeCount := len(titleNodes)

	if titleNodeCount == 0 {
		return domain.NewFailIssue(
			c,
			c.MissingEmptySeverity,
			"Missing <title> tag.",
			map[string]any{
				"issue_type": "missing",
			},
		)
	}

	builder := domain.NewIssueBuilder()

	if titleNodeCount > 1 {
		builder.Add(domain.Finding{
			Type:     "multiple",
			Severity: c.MultipleSeverity,
			Message:  "Multiple <title> tags found.",
			Data: map[string]any{
				"count": titleNodeCount,
			},
		})
	}

	titleContent := strings.TrimSpace(c.extractText(titleNodes[0]))
	if titleContent == "" {
		builder.Add(domain.Finding{
			Type:     "empty",
			Severity: c.MissingEmptySeverity,
			Message:  "<title> tag is empty or contains only whitespace.",
		})
	} else {
		c.checkLength(builder, titleContent)
	}

	if builder.HasFindings() {
		return domain.NewFailIssue(c, builder.Severity(), c.summary(builder), builder.Details())
	}

	return domain.NewPassIssue(
		c,
		"Title is present and has appropriate length.",
	)
}

// summary renders a short message for the aggregated issue from the kinds of
// findings collected.
func (c *TitleCheck) summary(builder *domain.IssueBuilder) string {
	var parts []string
	if builder.CountByType("multiple") > 0 {
		parts = append(parts, "Multiple <title> tags found.")
	}
	if builder.CountByType("empty") > 0 {
		parts = append(parts, "The <title> tag is empty or contains only whitespace.")
	}
	if builder.CountByType("length_max") > 0 {
		parts = append(parts, "The <title> is longer than the recommended maximum.")
	}
	if builder.CountByType("length_min") > 0 {
		parts = append(parts, "The <title> is shorter than the recommended minimum.")
	}
	return strings.Join(parts, " ")
}

func (c *TitleCheck) checkLength(builder *domain.IssueBuilder, titleContent string) {
	titleLength := utf8.RuneCountInString(titleContent)

	if c.MaxTitleLength != nil && titleLength > *c.MaxTitleLength {
		overage := titleLength - *c.MaxTitleLength

		level := c.MinorLengthDeviationSeverity
		if overage >= c.LengthWarningOverage {
			level = c.MajorLengthDeviationSeverity
		}

		message := fmt.Sprintf("Title length (%d) exceeds the ideal maximum of %d by %d characters.", titleLength, *c.MaxTitleLength, overage)

		builder.Add(domain.Finding{
			Type:     "length_max",
			Severity: level,
			Message:  message,
			Data: map[string]any{
				"title":   titleContent,
				"length":  titleLength,
				"limit":   *c.MaxTitleLength,
				"overage": overage,
			},
		})
	}

	if c.MinTitleLength != nil && titleLength < *c.MinTitleLength {
		message := fmt.Sprintf("Title length (%d) is less than the recommended minimum of %d.", titleLength, *c.MinTitleLength)

		builder.Add(domain.Finding{
			Type:     "length_min",
			Severity: c.MajorLengthDeviationSeverity,
			Message:  message,
			Data: map[string]any{
				"title":  titleContent,
				"length": titleLength,
				"limit":  *c.MinTitleLength,
			},
		})
	}
}

func (c *TitleCheck) findTitleNodes(root *html.Node) []*html.Node {
	var titles []*html.Node

	var traverse func(n *html.Node, inHead bool)
	traverse = func(n *html.Node, inHead bool) {
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, "head") {
			inHead = true
		}

		if inHead && n.Type == html.ElementNode && strings.EqualFold(n.Data, "title") {
			titles = append(titles, n)
		}

		for child := n.FirstChild; child != nil; child = child.NextSibling {
			traverse(child, inHead)
		}
	}

	traverse(root, false)
	return titles
}

func (c *TitleCheck) extractText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var buf strings.Builder
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		buf.WriteString(c.extractText(child))
	}
	return buf.String()
}
