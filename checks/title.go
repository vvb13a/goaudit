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
		MinTitleLength:               intPtr(10),
		MaxTitleLength:               intPtr(60),
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

func (c *TitleCheck) Apply(ctx context.Context, doc *domain.Document) []domain.Issue {
	var detectedIssues []domain.Issue

	root, err := html.Parse(bytes.NewReader(doc.Body))
	if err != nil {
		return []domain.Issue{
			domain.NewFailIssue(
				c,
				domain.SeverityError,
				fmt.Sprintf("Error during title check: %s", err.Error()),
				nil,
			),
		}
	}

	titleNodes := c.findTitleNodes(root)
	titleNodeCount := len(titleNodes)

	if titleNodeCount == 0 {
		return []domain.Issue{
			domain.NewFailIssue(
				c,
				c.MissingEmptySeverity,
				"Missing <title> tag.",
				map[string]any{
					"issue_type": "missing",
				},
			),
		}
	}

	if titleNodeCount > 1 {
		detectedIssues = append(detectedIssues, domain.NewFailIssue(
			c,
			c.MultipleSeverity,
			"Multiple <title> tags found.",
			map[string]any{
				"issue_type": "multiple",
				"count":      titleNodeCount,
			},
		))
	}

	titleContent := strings.TrimSpace(c.extractText(titleNodes[0]))
	if titleContent == "" {
		detectedIssues = append(detectedIssues, domain.NewFailIssue(
			c,
			c.MissingEmptySeverity,
			"<title> tag is empty or contains only whitespace.",
			map[string]any{
				"issue_type": "empty",
			},
		))
	} else {
		c.checkLength(&detectedIssues, titleContent)
	}

	if len(detectedIssues) > 0 {
		return detectedIssues
	}

	return []domain.Issue{
		domain.NewPassIssue(
			c,
			"Title is present and has appropriate length.",
		),
	}
}

func (c *TitleCheck) checkLength(detectedIssues *[]domain.Issue, titleContent string) {
	titleLength := utf8.RuneCountInString(titleContent)

	if c.MaxTitleLength != nil && titleLength > *c.MaxTitleLength {
		overage := titleLength - *c.MaxTitleLength

		level := c.MinorLengthDeviationSeverity
		if overage >= c.LengthWarningOverage {
			level = c.MajorLengthDeviationSeverity
		}

		message := fmt.Sprintf("Title length (%d) exceeds the ideal maximum of %d by %d characters.", titleLength, *c.MaxTitleLength, overage)

		*detectedIssues = append(*detectedIssues, domain.NewFailIssue(
			c,
			level,
			message,
			map[string]any{
				"issue_type": "length_max",
				"title":      titleContent,
				"length":     titleLength,
				"limit":      *c.MaxTitleLength,
				"overage":    overage,
			},
		))
	}

	if c.MinTitleLength != nil && titleLength < *c.MinTitleLength {
		message := fmt.Sprintf("Title length (%d) is less than the recommended minimum of %d.", titleLength, *c.MinTitleLength)

		*detectedIssues = append(*detectedIssues, domain.NewFailIssue(
			c,
			c.MajorLengthDeviationSeverity,
			message,
			map[string]any{
				"issue_type": "length_min",
				"title":      titleContent,
				"length":     titleLength,
				"limit":      *c.MinTitleLength,
			},
		))
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
