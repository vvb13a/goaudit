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

func (c *TwitterCardCheck) Apply(ctx context.Context, doc *domain.Document) []domain.Issue {
	root, err := html.Parse(bytes.NewReader(doc.Body))
	if err != nil {
		return []domain.Issue{
			domain.NewFailIssue(
				c,
				domain.SeverityError,
				fmt.Sprintf("Error during Twitter Card check: %s", err.Error()),
				nil,
			),
		}
	}

	presentTwitterTags := c.findTwitterTags(root)
	presentProperties := make(map[string]string)
	var detectedIssues []domain.Issue

	for _, tag := range presentTwitterTags {
		property := tag.Property
		content := tag.Content

		if _, exists := presentProperties[property]; exists {
			detectedIssues = append(detectedIssues, domain.NewFailIssue(
				c,
				c.ValidationSeverity,
				fmt.Sprintf("Multiple Twitter Card tags found for property '%s'.", property),
				map[string]any{
					"issue_type": "multiple",
					"property":   property,
				},
			))
		}
		presentProperties[property] = content

		if content == "" {
			detectedIssues = append(detectedIssues, domain.NewFailIssue(
				c,
				c.ValidationSeverity,
				fmt.Sprintf("Twitter Card property '%s' has empty content.", property),
				map[string]any{
					"issue_type": "empty_content",
					"property":   property,
				},
			))
		}
	}

	for _, property := range c.RequiredProperties {
		if _, exists := presentProperties[property]; !exists {
			detectedIssues = append(detectedIssues, domain.NewFailIssue(
				c,
				c.MissingRequiredSeverity,
				fmt.Sprintf("Required Twitter Card property '%s' is missing.", property),
				map[string]any{
					"issue_type": "missing_required",
					"property":   property,
				},
			))
		}
	}

	if cardType, ok := presentProperties["twitter:card"]; ok {
		if _, isValid := allowedTwitterCardTypes[cardType]; !isValid {
			detectedIssues = append(detectedIssues, domain.NewFailIssue(
				c,
				c.ValidationSeverity,
				fmt.Sprintf("Twitter Card property 'twitter:card' has an invalid value '%s'.", cardType),
				map[string]any{
					"issue_type": "invalid_card_type",
					"content":    cardType,
				},
			))
		}
	}

	if len(detectedIssues) > 0 {
		return detectedIssues
	}

	return []domain.Issue{
		domain.NewPassIssue(
			c,
			"All required Twitter Card tags are present and valid.",
		),
	}
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
