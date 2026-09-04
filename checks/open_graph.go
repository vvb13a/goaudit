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

func (c *OpenGraphCheck) Apply(ctx context.Context, doc *domain.Document) domain.Issue {
	root, err := html.Parse(bytes.NewReader(doc.Body))
	if err != nil {
		return domain.NewFailIssue(
			c,
			domain.SeverityError,
			fmt.Sprintf("Error during Open Graph check: %s", err.Error()),
			nil,
		)
	}

	presentOgTags := c.findOGTags(root)
	presentProperties := make(map[string][]string)
	builder := domain.NewIssueBuilder()

	for _, tag := range presentOgTags {
		property := tag.Property
		content := tag.Content

		presentProperties[property] = append(presentProperties[property], content)

		if content == "" {
			builder.Add(domain.Finding{
				Type:     "empty_content",
				Severity: c.ValidationSeverity,
				Message:  fmt.Sprintf("Open Graph property '%s' has empty content.", property),
				Data: map[string]any{
					"property": property,
				},
			})
			continue
		}

		if ruleType, ok := c.KnownProperties[property]; ok {
			c.validateContent(builder, property, content, ruleType)
		}
	}

	c.checkForDuplicates(builder, presentProperties)

	c.checkForMissing(builder, presentProperties)

	if builder.HasFindings() {
		return domain.NewFailIssue(c, builder.Severity(), c.summary(builder), builder.Details())
	}

	return domain.NewPassIssue(
		c,
		"Open Graph tags are well-formed and valid.",
	)
}

// summary renders a short message for the aggregated issue from the kinds of
// findings collected.
func (c *OpenGraphCheck) summary(builder *domain.IssueBuilder) string {
	var parts []string
	if count := builder.CountByType("missing_required"); count > 0 {
		parts = append(parts, fmt.Sprintf("%d required property(ies) are missing", count))
	}
	if builder.CountByType("missing_image") > 0 {
		parts = append(parts, "an og:image property is missing")
	}
	if count := builder.CountByType("missing_recommended"); count > 0 {
		parts = append(parts, fmt.Sprintf("%d recommended property(ies) are missing", count))
	}
	if count := builder.CountByType("empty_content"); count > 0 {
		parts = append(parts, fmt.Sprintf("%d tag(s) have empty content", count))
	}
	if count := builder.CountByType("multiple"); count > 0 {
		parts = append(parts, fmt.Sprintf("%d non-arrayable property(ies) are declared more than once", count))
	}
	if count := builder.CountByType("relative_url"); count > 0 {
		parts = append(parts, fmt.Sprintf("%d value(s) are not absolute URLs", count))
	}
	if count := builder.CountByType("not_numeric"); count > 0 {
		parts = append(parts, fmt.Sprintf("%d value(s) are not numeric", count))
	}
	if count := builder.CountByType("invalid_mime_type"); count > 0 {
		parts = append(parts, fmt.Sprintf("%d value(s) have an invalid MIME type", count))
	}
	if count := builder.CountByType("invalid_datetime"); count > 0 {
		parts = append(parts, fmt.Sprintf("%d value(s) have an invalid datetime format", count))
	}
	return strings.Join(parts, "; ") + "."
}

func (c *OpenGraphCheck) validateContent(builder *domain.IssueBuilder, property, content, ruleType string) {
	switch ruleType {
	case "absolute_url":
		if !strings.HasPrefix(content, "http://") && !strings.HasPrefix(content, "https://") {
			builder.Add(domain.Finding{
				Type:     "relative_url",
				Severity: c.ValidationSeverity,
				Message:  fmt.Sprintf("Open Graph property '%s' must be an absolute URL.", property),
				Data: map[string]any{
					"property": property,
					"content":  content,
				},
			})
		}
	case "numeric":
		if _, err := strconv.ParseFloat(content, 64); err != nil {
			builder.Add(domain.Finding{
				Type:     "not_numeric",
				Severity: c.ValidationSeverity,
				Message:  fmt.Sprintf("Open Graph property '%s' must have a numeric value.", property),
				Data: map[string]any{
					"property": property,
					"content":  content,
				},
			})
		}
	case "mime_type":
		if !mimeTypeRegex.MatchString(content) {
			builder.Add(domain.Finding{
				Type:     "invalid_mime_type",
				Severity: c.ValidationSeverity,
				Message:  fmt.Sprintf("Open Graph property '%s' has an invalid MIME type format.", property),
				Data: map[string]any{
					"property": property,
					"content":  content,
				},
			})
		}
	case "datetime":
		if !c.isValidDateTime(content) {
			builder.Add(domain.Finding{
				Type:     "invalid_datetime",
				Severity: c.ValidationSeverity,
				Message:  fmt.Sprintf("Open Graph property '%s' has an invalid datetime format.", property),
				Data: map[string]any{
					"property": property,
					"content":  content,
				},
			})
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

func (c *OpenGraphCheck) checkForDuplicates(builder *domain.IssueBuilder, presentProperties map[string][]string) {
	for property, values := range presentProperties {
		if len(values) > 1 {
			if _, isArrayable := c.ArrayableProperties[property]; !isArrayable {
				builder.Add(domain.Finding{
					Type:     "multiple",
					Severity: c.ValidationSeverity,
					Message:  fmt.Sprintf("Multiple Open Graph tags found for non-arrayable property '%s'.", property),
					Data: map[string]any{
						"property": property,
						"count":    len(values),
					},
				})
			}
		}
	}
}

func (c *OpenGraphCheck) checkForMissing(builder *domain.IssueBuilder, presentProperties map[string][]string) {
	for _, property := range c.RequiredProperties {
		if _, ok := presentProperties[property]; !ok {
			builder.Add(domain.Finding{
				Type:     "missing_required",
				Severity: c.MissingRequiredSeverity,
				Message:  fmt.Sprintf("Required Open Graph property '%s' is missing.", property),
				Data: map[string]any{
					"property": property,
				},
			})
		}
	}

	_, hasImage := presentProperties["og:image"]
	_, hasImageURL := presentProperties["og:image:url"]
	if !hasImage && !hasImageURL {
		builder.Add(domain.Finding{
			Type:     "missing_image",
			Severity: c.MissingRequiredSeverity,
			Message:  "Required Open Graph image ('og:image' or 'og:image:url') is missing.",
			Data: map[string]any{
				"property": "og:image",
			},
		})
	}

	for _, property := range c.RecommendedProperties {
		if _, ok := presentProperties[property]; !ok {
			builder.Add(domain.Finding{
				Type:     "missing_recommended",
				Severity: c.MissingRecommendedSeverity,
				Message:  fmt.Sprintf("Recommended Open Graph property '%s' is missing.", property),
				Data: map[string]any{
					"property": property,
				},
			})
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
