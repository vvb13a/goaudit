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

type MetaDescriptionCheck struct {
	MinDescriptionLength         *int
	MaxDescriptionLength         *int
	LengthWarningOverage         *int
	MinorLengthDeviationSeverity domain.Severity
	MajorLengthDeviationSeverity domain.Severity
	MissingEmptySeverity         domain.Severity
	MultipleSeverity             domain.Severity
}

func NewMetaDescriptionCheck() *MetaDescriptionCheck {
	return &MetaDescriptionCheck{
		MinDescriptionLength:         intPtr(50),
		MaxDescriptionLength:         intPtr(160),
		LengthWarningOverage:         intPtr(20),
		MinorLengthDeviationSeverity: domain.SeverityNotice,
		MajorLengthDeviationSeverity: domain.SeverityWarning,
		MissingEmptySeverity:         domain.SeverityError,
		MultipleSeverity:             domain.SeverityError,
	}
}

func (c *MetaDescriptionCheck) Info() domain.CheckInfo {
	return domain.CheckInfo{
		Name:        "meta_description",
		Description: "Checks for a single meta description tag of appropriate length.",
		Category:    domain.CategorySEO,
	}
}

func (c *MetaDescriptionCheck) Supports(doc *domain.Document) bool {
	return doc.IsHTML()
}

func (c *MetaDescriptionCheck) Apply(ctx context.Context, doc *domain.Document) []domain.Issue {
	var detectedIssues []domain.Issue

	root, err := html.Parse(bytes.NewReader(doc.Body))
	if err != nil {
		return []domain.Issue{
			domain.NewFailIssue(
				c,
				domain.SeverityError,
				fmt.Sprintf("Error during meta description check: %s", err.Error()),
				nil,
			),
		}
	}

	metaDescNodes := c.findMetaDescriptions(root)
	descNodeCount := len(metaDescNodes)

	if descNodeCount == 0 {
		return []domain.Issue{
			domain.NewFailIssue(
				c,
				c.MissingEmptySeverity,
				"Missing <meta name=\"description\"> tag.",
				map[string]any{
					"issue_type": "missing",
				},
			),
		}
	}

	if descNodeCount > 1 {
		detectedIssues = append(detectedIssues, domain.NewFailIssue(
			c,
			c.MultipleSeverity,
			"Multiple <meta name=\"description\"> tags found.",
			map[string]any{
				"issue_type": "multiple",
				"count":      descNodeCount,
			},
		))
	}

	descriptionContent := strings.TrimSpace(metaDescNodes[0])
	if descriptionContent == "" {
		detectedIssues = append(detectedIssues, domain.NewFailIssue(
			c,
			c.MissingEmptySeverity,
			"<meta name=\"description\"> tag content is empty.",
			map[string]any{
				"issue_type": "empty",
			},
		))
	} else {
		c.checkLength(&detectedIssues, descriptionContent)
	}

	if len(detectedIssues) > 0 {
		return detectedIssues
	}

	return []domain.Issue{
		domain.NewPassIssue(
			c,
			"Meta description is present and has appropriate length.",
		),
	}
}

func (c *MetaDescriptionCheck) checkLength(detectedIssues *[]domain.Issue, descriptionContent string) {
	descLength := utf8.RuneCountInString(descriptionContent)

	if c.MaxDescriptionLength != nil && descLength > *c.MaxDescriptionLength {
		overage := descLength - *c.MaxDescriptionLength

		level := c.MinorLengthDeviationSeverity
		if c.LengthWarningOverage != nil && overage >= *c.LengthWarningOverage {
			level = c.MajorLengthDeviationSeverity
		}

		message := fmt.Sprintf("Title length (%d) exceeds the ideal maximum of %d by %d characters.", descLength, *c.MaxDescriptionLength, overage)

		*detectedIssues = append(*detectedIssues, domain.NewFailIssue(
			c,
			level,
			message,
			map[string]any{
				"issue_type": "length_max",
				"title":      descriptionContent,
				"length":     descLength,
				"limit":      *c.MaxDescriptionLength,
				"overage":    overage,
			},
		))
	}

	if c.MinDescriptionLength != nil && descLength < *c.MinDescriptionLength {
		message := fmt.Sprintf("Title length (%d) is less than the recommended minimum of %d.", descLength, *c.MinDescriptionLength)

		*detectedIssues = append(*detectedIssues, domain.NewFailIssue(
			c,
			c.MajorLengthDeviationSeverity,
			message,
			map[string]any{
				"issue_type": "length_min",
				"title":      descriptionContent,
				"length":     descLength,
				"limit":      *c.MinDescriptionLength,
			},
		))
	}
}

func (c *MetaDescriptionCheck) findMetaDescriptions(root *html.Node) []string {
	var descriptions []string

	var traverse func(n *html.Node, inHead bool)
	traverse = func(n *html.Node, inHead bool) {
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, "head") {
			inHead = true
		}

		if inHead && n.Type == html.ElementNode && strings.EqualFold(n.Data, "meta") {
			var isDescription bool
			var content string

			for _, attr := range n.Attr {
				key := strings.ToLower(attr.Key)
				val := strings.TrimSpace(attr.Val)

				if key == "name" && strings.EqualFold(val, "description") {
					isDescription = true
				}
				if key == "content" {
					content = attr.Val
				}
			}

			if isDescription {
				descriptions = append(descriptions, content)
			}
		}

		for child := n.FirstChild; child != nil; child = child.NextSibling {
			traverse(child, inHead)
		}
	}

	traverse(root, false)
	return descriptions
}
