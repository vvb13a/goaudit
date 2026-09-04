package checks

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/vvb13a/goaudit/domain"

	"golang.org/x/net/html"
)

var allowedTwitterCardTypes = map[string]struct{}{
	"summary":             {},
	"summary_large_image": {},
	"app":                 {},
	"player":              {},
}

type TwitterCardCheck struct {
	MissingRequiredSeverity domain.Severity
	ValidationSeverity      domain.Severity
	RequiredProperties      []string
}

func NewTwitterCardCheck() *TwitterCardCheck {
	return &TwitterCardCheck{
		MissingRequiredSeverity: domain.SeverityWarning,
		ValidationSeverity:      domain.SeverityWarning,
		RequiredProperties: []string{
			"twitter:card",
			"twitter:title",
			"twitter:description",
			"twitter:image",
		},
	}
}

func (c *TwitterCardCheck) Info() domain.CheckInfo {
	return domain.CheckInfo{
		Name:        "twitter_card",
		Description: "Validates Twitter Card meta tags for required properties and values.",
		Category:    domain.CategorySEO,
	}
}

func (c *TwitterCardCheck) Supports(doc *domain.Document) bool {
	return doc.IsHTML()
}

type twitterTagNode struct {
	Property string
	Content  string
}

func (c *TwitterCardCheck) Apply(ctx context.Context, doc *domain.Document) domain.Issue {
	root, err := html.Parse(bytes.NewReader(doc.Body))
	if err != nil {
		return domain.NewFailIssue(
			c,
			domain.SeverityError,
			fmt.Sprintf("Error during Twitter Card check: %s", err.Error()),
			nil,
		)
	}

	presentTwitterTags := c.findTwitterTags(root)
	presentProperties := make(map[string]string)
	builder := domain.NewIssueBuilder()

	for _, tag := range presentTwitterTags {
		property := tag.Property
		content := tag.Content

		if _, exists := presentProperties[property]; exists {
			builder.Add(domain.Finding{
				Type:     "multiple",
				Severity: c.ValidationSeverity,
				Message:  fmt.Sprintf("Multiple Twitter Card tags found for property '%s'.", property),
				Data: map[string]any{
					"property": property,
				},
			})
		}
		presentProperties[property] = content

		if content == "" {
			builder.Add(domain.Finding{
				Type:     "empty_content",
				Severity: c.ValidationSeverity,
				Message:  fmt.Sprintf("Twitter Card property '%s' has empty content.", property),
				Data: map[string]any{
					"property": property,
				},
			})
		}
	}

	for _, property := range c.RequiredProperties {
		if _, exists := presentProperties[property]; !exists {
			builder.Add(domain.Finding{
				Type:     "missing_required",
				Severity: c.MissingRequiredSeverity,
				Message:  fmt.Sprintf("Required Twitter Card property '%s' is missing.", property),
				Data: map[string]any{
					"property": property,
				},
			})
		}
	}

	if cardType, ok := presentProperties["twitter:card"]; ok {
		if _, isValid := allowedTwitterCardTypes[cardType]; !isValid {
			builder.Add(domain.Finding{
				Type:     "invalid_card_type",
				Severity: c.ValidationSeverity,
				Message:  fmt.Sprintf("Twitter Card property 'twitter:card' has an invalid value '%s'.", cardType),
				Data: map[string]any{
					"content": cardType,
				},
			})
		}
	}

	if builder.HasFindings() {
		return domain.NewFailIssue(c, builder.Severity(), c.summary(builder), builder.Details())
	}

	return domain.NewPassIssue(
		c,
		"All required Twitter Card tags are present and valid.",
	)
}

// summary renders a short message for the aggregated issue from the kinds of
// findings collected.
func (c *TwitterCardCheck) summary(builder *domain.IssueBuilder) string {
	var parts []string
	if count := builder.CountByType("multiple"); count > 0 {
		parts = append(parts, fmt.Sprintf("%d property(ies) are declared more than once", count))
	}
	if count := builder.CountByType("empty_content"); count > 0 {
		parts = append(parts, fmt.Sprintf("%d tag(s) have empty content", count))
	}
	if count := builder.CountByType("missing_required"); count > 0 {
		parts = append(parts, fmt.Sprintf("%d required property(ies) are missing", count))
	}
	if builder.CountByType("invalid_card_type") > 0 {
		parts = append(parts, "the twitter:card value is invalid")
	}
	return strings.Join(parts, "; ") + "."
}

func (c *TwitterCardCheck) findTwitterTags(root *html.Node) []twitterTagNode {
	var tags []twitterTagNode

	var traverse func(n *html.Node, inHead bool)
	traverse = func(n *html.Node, inHead bool) {
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, "head") {
			inHead = true
		}

		if inHead && n.Type == html.ElementNode && strings.EqualFold(n.Data, "meta") {
			var property, content string

			for _, attr := range n.Attr {
				key := strings.ToLower(attr.Key)
				if key == "name" {
					property = strings.TrimSpace(attr.Val)
				}
				if key == "content" {
					content = strings.TrimSpace(attr.Val)
				}
			}

			if strings.HasPrefix(strings.ToLower(property), "twitter:") {
				tags = append(tags, twitterTagNode{
					Property: property,
					Content:  content,
				})
			}
		}

		for child := n.FirstChild; child != nil; child = child.NextSibling {
			traverse(child, inHead)
		}
	}

	traverse(root, false)
	return tags
}
