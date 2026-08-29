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

type MetaDescriptionCheck struct {
	MinDescriptionLength         *int
	MaxDescriptionLength         *int
	LengthWarningOverage         *int
	MinorLengthDeviationSeverity data.Severity
	MajorLengthDeviationSeverity data.Severity
	MissingEmptySeverity         data.Severity
	MultipleSeverity             data.Severity
}

func NewMetaDescriptionCheck() *MetaDescriptionCheck {
	return &MetaDescriptionCheck{
		MinDescriptionLength:         intPtr(50),
		MaxDescriptionLength:         intPtr(160),
		LengthWarningOverage:         intPtr(20),
		MinorLengthDeviationSeverity: data.SeverityNotice,
		MajorLengthDeviationSeverity: data.SeverityWarning,
		MissingEmptySeverity:         data.SeverityError,
		MultipleSeverity:             data.SeverityError,
	}
}

func (c *MetaDescriptionCheck) Name() string {
	return "meta_description"
}

func (c *MetaDescriptionCheck) Checklist() string {
	return "seo"
}

func (c *MetaDescriptionCheck) Supports(doc *data.Document) bool {
	return doc.IsHTML()
}

func (c *MetaDescriptionCheck) Apply(ctx context.Context, doc *data.Document) []data.Issue {
	var detectedIssues []data.Issue

	root, err := html.Parse(bytes.NewReader(doc.Body))
	if err != nil {
		return []data.Issue{
			data.NewFailIssue(
				c,
				data.SeverityError,
				fmt.Sprintf("Error during meta description check: %s", err.Error()),
				nil,
			),
		}
	}

	metaDescNodes := c.findMetaDescriptions(root)
	descNodeCount := len(metaDescNodes)

	if descNodeCount == 0 {
		return []data.Issue{
			data.NewFailIssue(
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
		detectedIssues = append(detectedIssues, data.NewFailIssue(
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
		detectedIssues = append(detectedIssues, data.NewFailIssue(
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

	return []data.Issue{
		data.NewPassIssue(
			c,
			"Meta description is present and has appropriate length.",
			nil,
		),
	}
}

func (c *MetaDescriptionCheck) checkLength(detectedIssues *[]data.Issue, descriptionContent string) {
	descLength := utf8.RuneCountInString(descriptionContent)

	if c.MaxDescriptionLength != nil && descLength > *c.MaxDescriptionLength {
		overage := descLength - *c.MaxDescriptionLength

		level := c.MinorLengthDeviationSeverity
		if c.LengthWarningOverage != nil && overage >= *c.LengthWarningOverage {
			level = c.MajorLengthDeviationSeverity
		}

		message := fmt.Sprintf("Title length (%d) exceeds the ideal maximum of %d by %d characters.", descLength, *c.MaxDescriptionLength, overage)

		*detectedIssues = append(*detectedIssues, data.NewFailIssue(
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

		*detectedIssues = append(*detectedIssues, data.NewFailIssue(
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
