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
		MinDescriptionLength:         new(50),
		MaxDescriptionLength:         new(160),
		LengthWarningOverage:         new(20),
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

func (c *MetaDescriptionCheck) Apply(ctx context.Context, doc *domain.Document) domain.Issue {
	root, err := html.Parse(bytes.NewReader(doc.Body))
	if err != nil {
		return domain.NewFailIssue(
			c,
			domain.SeverityError,
			fmt.Sprintf("Error during meta description check: %s", err.Error()),
			nil,
		)
	}

	metaDescNodes := c.findMetaDescriptions(root)
	descNodeCount := len(metaDescNodes)

	if descNodeCount == 0 {
		return domain.NewFailIssue(
			c,
			c.MissingEmptySeverity,
			"Missing <meta name=\"description\"> tag.",
			map[string]any{
				"issue_type": "missing",
			},
		)
	}

	builder := domain.NewIssueBuilder()

	if descNodeCount > 1 {
		builder.Add(domain.Finding{
			Type:     "multiple",
			Severity: c.MultipleSeverity,
			Message:  "Multiple <meta name=\"description\"> tags found.",
			Data: map[string]any{
				"count": descNodeCount,
			},
		})
	}

	descriptionContent := strings.TrimSpace(metaDescNodes[0])
	if descriptionContent == "" {
		builder.Add(domain.Finding{
			Type:     "empty",
			Severity: c.MissingEmptySeverity,
			Message:  "<meta name=\"description\"> tag content is empty.",
		})
	} else {
		c.checkLength(builder, descriptionContent)
	}

	if builder.HasFindings() {
		return domain.NewFailIssue(c, builder.Severity(), c.summary(builder), builder.Details())
	}

	return domain.NewPassIssue(
		c,
		"Meta description is present and has appropriate length.",
	)
}

// summary renders a short message for the aggregated issue from the kinds of
// findings collected.
func (c *MetaDescriptionCheck) summary(builder *domain.IssueBuilder) string {
	var parts []string
	if builder.CountByType("multiple") > 0 {
		parts = append(parts, "Multiple meta description tags found.")
	}
	if builder.CountByType("empty") > 0 {
		parts = append(parts, "The meta description content is empty.")
	}
	if builder.CountByType("length_max") > 0 {
		parts = append(parts, "The meta description is longer than the recommended maximum.")
	}
	if builder.CountByType("length_min") > 0 {
		parts = append(parts, "The meta description is shorter than the recommended minimum.")
	}
	return strings.Join(parts, " ")
}

func (c *MetaDescriptionCheck) checkLength(builder *domain.IssueBuilder, descriptionContent string) {
	descLength := utf8.RuneCountInString(descriptionContent)

	if c.MaxDescriptionLength != nil && descLength > *c.MaxDescriptionLength {
		overage := descLength - *c.MaxDescriptionLength

		level := c.MinorLengthDeviationSeverity
		if c.LengthWarningOverage != nil && overage >= *c.LengthWarningOverage {
			level = c.MajorLengthDeviationSeverity
		}

		message := fmt.Sprintf("Meta description length (%d) exceeds the ideal maximum of %d by %d characters.", descLength, *c.MaxDescriptionLength, overage)

		builder.Add(domain.Finding{
			Type:     "length_max",
			Severity: level,
			Message:  message,
			Data: map[string]any{
				"description": descriptionContent,
				"length":      descLength,
				"limit":       *c.MaxDescriptionLength,
				"overage":     overage,
			},
		})
	}

	if c.MinDescriptionLength != nil && descLength < *c.MinDescriptionLength {
		message := fmt.Sprintf("Meta description length (%d) is less than the recommended minimum of %d.", descLength, *c.MinDescriptionLength)

		builder.Add(domain.Finding{
			Type:     "length_min",
			Severity: c.MajorLengthDeviationSeverity,
			Message:  message,
			Data: map[string]any{
				"description": descriptionContent,
				"length":      descLength,
				"limit":       *c.MinDescriptionLength,
			},
		})
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
