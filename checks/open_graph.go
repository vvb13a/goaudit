package checks

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/vvb13a/goaudit/domain"

	"golang.org/x/net/html"
)

var (
	mimeTypeRegex = regexp.MustCompile(`^[a-z]+/[a-z0-9\-+]+$`)

	dateTimeLayouts = []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05-0700",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
		time.RFC1123,
		time.RFC1123Z,
		time.RFC822,
		time.RFC822Z,
	}

	ogPrefixes = []string{"og:", "article:", "music:", "video:", "book:", "profile:"}

	defaultArrayableProperties = map[string]struct{}{
		"og:image":            {},
		"og:locale:alternate": {},
		"og:audio":            {},
		"og:video":            {},
		"music:album":         {},
		"music:musician":      {},
		"video:actor":         {},
		"video:director":      {},
		"video:writer":        {},
		"video:tag":           {},
		"article:author":      {},
		"article:tag":         {},
		"book:author":         {},
		"book:tag":            {},
	}

	defaultKnownProperties = map[string]string{
		"og:title":                "non_empty_string",
		"og:type":                 "non_empty_string",
		"og:url":                  "absolute_url",
		"og:description":          "non_empty_string",
		"og:locale":               "locale",
		"og:locale:alternate":     "locale",
		"og:site_name":            "non_empty_string",
		"og:image":                "absolute_url",
		"og:image:url":            "absolute_url",
		"og:image:secure_url":     "absolute_url",
		"og:image:type":           "mime_type",
		"og:image:width":          "numeric",
		"og:image:height":         "numeric",
		"og:image:alt":            "non_empty_string",
		"og:video":                "absolute_url",
		"og:video:secure_url":     "absolute_url",
		"og:video:type":           "mime_type",
		"og:video:width":          "numeric",
		"og:video:height":         "numeric",
		"og:audio":                "absolute_url",
		"og:audio:secure_url":     "absolute_url",
		"og:audio:type":           "mime_type",
		"article:published_time":  "datetime",
		"article:modified_time":   "datetime",
		"article:expiration_time": "datetime",
		"article:author":          "absolute_url",
		"article:section":         "non_empty_string",
		"article:tag":             "non_empty_string",
	}
)

type OpenGraphCheck struct {
	MissingRequiredSeverity    domain.Severity
	MissingRecommendedSeverity domain.Severity
	ValidationSeverity         domain.Severity

	RequiredProperties    []string
	RecommendedProperties []string
	ArrayableProperties   map[string]struct{}
	KnownProperties       map[string]string
}

func NewOpenGraphCheck() *OpenGraphCheck {
	return &OpenGraphCheck{
		MissingRequiredSeverity:    domain.SeverityError,
		MissingRecommendedSeverity: domain.SeverityWarning,
		ValidationSeverity:         domain.SeverityWarning,

		RequiredProperties:    []string{"og:title", "og:type", "og:url"},
		RecommendedProperties: []string{"og:description", "og:locale", "og:site_name", "og:image:alt"},
		ArrayableProperties:   defaultArrayableProperties,
		KnownProperties:       defaultKnownProperties,
	}
}

func (c *OpenGraphCheck) Info() domain.CheckInfo {
	return domain.CheckInfo{
		Name:        "open_graph",
		Description: "Validates Open Graph meta tags for presence, format, and required properties.",
		Category:    domain.CategorySEO,
	}
}

func (c *OpenGraphCheck) Supports(doc *domain.Document) bool {
	return doc.IsHTML()
}

type ogTagNode struct {
	Property string
	Content  string
}

func (c *OpenGraphCheck) Apply(ctx context.Context, doc *domain.Document) []domain.Issue {
	root, err := html.Parse(bytes.NewReader(doc.Body))
	if err != nil {
		return []domain.Issue{
			domain.NewFailIssue(
				c,
				domain.SeverityError,
				fmt.Sprintf("Error during Open Graph check: %s", err.Error()),
				nil,
			),
		}
	}

	presentOgTags := c.findOGTags(root)
	presentProperties := make(map[string][]string)
	var detectedIssues []domain.Issue

	for _, tag := range presentOgTags {
		property := tag.Property
		content := tag.Content

		presentProperties[property] = append(presentProperties[property], content)

		if content == "" {
			detectedIssues = append(detectedIssues, domain.NewFailIssue(
				c,
				c.ValidationSeverity,
				fmt.Sprintf("Open Graph property '%s' has empty content.", property),
				map[string]any{
					"issue_type": "empty_content",
					"property":   property,
				},
			))
			continue
		}

		if ruleType, ok := c.KnownProperties[property]; ok {
			c.validateContent(&detectedIssues, property, content, ruleType)
		}
	}

	c.checkForDuplicates(&detectedIssues, presentProperties)

	c.checkForMissing(&detectedIssues, presentProperties)

	if len(detectedIssues) > 0 {
		return detectedIssues
	}

	return []domain.Issue{
		domain.NewPassIssue(
			c,
			"Open Graph tags are well-formed and valid.",
		),
	}
}

func (c *OpenGraphCheck) validateContent(detectedIssues *[]domain.Issue, property, content, ruleType string) {
	switch ruleType {
	case "absolute_url":
		if !strings.HasPrefix(content, "http://") && !strings.HasPrefix(content, "https://") {
			*detectedIssues = append(*detectedIssues, domain.NewFailIssue(
				c,
				c.ValidationSeverity,
				fmt.Sprintf("Open Graph property '%s' must be an absolute URL.", property),
				map[string]any{
					"issue_type": "relative_url",
					"property":   property,
					"content":    content,
				},
			))
		}
	case "numeric":
		if _, err := strconv.ParseFloat(content, 64); err != nil {
			*detectedIssues = append(*detectedIssues, domain.NewFailIssue(
				c,
				c.ValidationSeverity,
				fmt.Sprintf("Open Graph property '%s' must have a numeric value.", property),
				map[string]any{
					"issue_type": "not_numeric",
					"property":   property,
					"content":    content,
				},
			))
		}
	case "mime_type":
		if !mimeTypeRegex.MatchString(content) {
			*detectedIssues = append(*detectedIssues, domain.NewFailIssue(
				c,
				c.ValidationSeverity,
				fmt.Sprintf("Open Graph property '%s' has an invalid MIME type format.", property),
				map[string]any{
					"issue_type": "invalid_mime_type",
					"property":   property,
					"content":    content,
				},
			))
		}
	case "datetime":
		if !c.isValidDateTime(content) {
			*detectedIssues = append(*detectedIssues, domain.NewFailIssue(
				c,
				c.ValidationSeverity,
				fmt.Sprintf("Open Graph property '%s' has an invalid datetime format.", property),
				map[string]any{
					"issue_type": "invalid_datetime",
					"property":   property,
					"content":    content,
				},
			))
		}
	}
}

func (c *OpenGraphCheck) isValidDateTime(val string) bool {
	for _, layout := range dateTimeLayouts {
		if _, err := time.Parse(layout, val); err == nil {
			return true
		}
	}
	return false
}

func (c *OpenGraphCheck) checkForDuplicates(detectedIssues *[]domain.Issue, presentProperties map[string][]string) {
	for property, values := range presentProperties {
		if len(values) > 1 {
			if _, isArrayable := c.ArrayableProperties[property]; !isArrayable {
				*detectedIssues = append(*detectedIssues, domain.NewFailIssue(
					c,
					c.ValidationSeverity,
					fmt.Sprintf("Multiple Open Graph tags found for non-arrayable property '%s'.", property),
					map[string]any{
						"issue_type": "multiple",
						"property":   property,
						"count":      len(values),
					},
				))
			}
		}
	}
}

func (c *OpenGraphCheck) checkForMissing(detectedIssues *[]domain.Issue, presentProperties map[string][]string) {
	for _, property := range c.RequiredProperties {
		if _, ok := presentProperties[property]; !ok {
			*detectedIssues = append(*detectedIssues, domain.NewFailIssue(
				c,
				c.MissingRequiredSeverity,
				fmt.Sprintf("Required Open Graph property '%s' is missing.", property),
				map[string]any{
					"issue_type": "missing_required",
					"property":   property,
				},
			))
		}
	}

	_, hasImage := presentProperties["og:image"]
	_, hasImageURL := presentProperties["og:image:url"]
	if !hasImage && !hasImageURL {
		*detectedIssues = append(*detectedIssues, domain.NewFailIssue(
			c,
			c.MissingRequiredSeverity,
			"Required Open Graph image ('og:image' or 'og:image:url') is missing.",
			map[string]any{
				"issue_type": "missing_image",
				"property":   "og:image",
			},
		))
	}

	for _, property := range c.RecommendedProperties {
		if _, ok := presentProperties[property]; !ok {
			*detectedIssues = append(*detectedIssues, domain.NewFailIssue(
				c,
				c.MissingRecommendedSeverity,
				fmt.Sprintf("Recommended Open Graph property '%s' is missing.", property),
				map[string]any{
					"issue_type": "missing_recommended",
					"property":   property,
				},
			))
		}
	}
}

func (c *OpenGraphCheck) findOGTags(root *html.Node) []ogTagNode {
	var tags []ogTagNode

	var traverse func(n *html.Node, inHead bool)
	traverse = func(n *html.Node, inHead bool) {
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, "head") {
			inHead = true
		}

		if inHead && n.Type == html.ElementNode && strings.EqualFold(n.Data, "meta") {
			var property, content string

			for _, attr := range n.Attr {
				key := strings.ToLower(attr.Key)
				if key == "property" {
					property = strings.TrimSpace(attr.Val)
				}
				if key == "content" {
					content = strings.TrimSpace(attr.Val)
				}
			}

			if c.hasMatchingPrefix(property) {
				tags = append(tags, ogTagNode{
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

func (c *OpenGraphCheck) hasMatchingPrefix(prop string) bool {
	for _, prefix := range ogPrefixes {
		if strings.HasPrefix(prop, prefix) {
			return true
		}
	}
	return false
}
