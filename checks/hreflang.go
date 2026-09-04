package checks

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/vvb13a/goaudit/domain"

	"golang.org/x/net/html"
)

var hreflangRegex = regexp.MustCompile(`^[a-z]{2}(-[A-Z]{2})?$|^x-default$`)

type HreflangCheck struct {
	FormatSeverity               domain.Severity
	RelativeURLSeverity          domain.Severity
	MissingSelfReferenceSeverity domain.Severity
}

func NewHreflangCheck() *HreflangCheck {
	return &HreflangCheck{
		FormatSeverity:               domain.SeverityError,
		RelativeURLSeverity:          domain.SeverityError,
		MissingSelfReferenceSeverity: domain.SeverityWarning,
	}
}

func (c *HreflangCheck) Info() domain.CheckInfo {
	return domain.CheckInfo{
		Name:        "hreflang",
		Description: "Validates hreflang alternate tags for language format, absolute URLs, and self-references.",
		Category:    domain.CategorySEO,
	}
}

func (c *HreflangCheck) Supports(doc *domain.Document) bool {
	return doc.IsHTML()
}

type hreflangTag struct {
	Hreflang string
	Href     string
}

func (c *HreflangCheck) Apply(ctx context.Context, doc *domain.Document) domain.Issue {
	root, err := html.Parse(bytes.NewReader(doc.Body))
	if err != nil {
		return domain.NewFailIssue(
			c,
			domain.SeverityError,
			fmt.Sprintf("Error during hreflang check: %s", err.Error()),
			nil,
		)
	}

	tags := c.findHreflangTags(root)

	if len(tags) == 0 {
		return domain.NewPassIssue(
			c,
			"No hreflang tags found on the page.",
		)
	}

	builder := domain.NewIssueBuilder()
	languageCodes := make(map[string]string)
	hasSelfReference := false
	url := doc.URL

	for _, tag := range tags {
		hreflang := tag.Hreflang
		href := tag.Href

		if !hreflangRegex.MatchString(hreflang) {
			builder.Add(domain.Finding{
				Type:     "invalid_format",
				Severity: c.FormatSeverity,
				Message:  fmt.Sprintf("Hreflang attribute '%s' has an invalid format.", hreflang),
				Data: map[string]any{
					"hreflang": hreflang,
					"href":     href,
				},
			})
		}

		if !strings.HasPrefix(href, "http://") && !strings.HasPrefix(href, "https://") {
			builder.Add(domain.Finding{
				Type:     "relative_url",
				Severity: c.RelativeURLSeverity,
				Message:  fmt.Sprintf("Hreflang link for '%s' must use an absolute URL.", hreflang),
				Data: map[string]any{
					"hreflang": hreflang,
					"href":     href,
				},
			})
		}

		if _, exists := languageCodes[hreflang]; exists {
			builder.Add(domain.Finding{
				Type:     "duplicate_code",
				Severity: c.FormatSeverity,
				Message:  fmt.Sprintf("Duplicate hreflang tag found for language code '%s'.", hreflang),
				Data: map[string]any{
					"hreflang": hreflang,
				},
			})
		}
		languageCodes[hreflang] = href

		if href == url {
			hasSelfReference = true
		}
	}

	if !hasSelfReference {
		builder.Add(domain.Finding{
			Type:     "missing_self_reference",
			Severity: c.MissingSelfReferenceSeverity,
			Message:  "Hreflang tags are present, but a self-referencing link pointing to the current URL is missing.",
			Data: map[string]any{
				"current_url": url,
			},
		})
	}

	if builder.HasFindings() {
		return domain.NewFailIssue(c, builder.Severity(), c.summary(builder), builder.Details())
	}

	return domain.NewPassIssue(
		c,
		"All hreflang tags are present and correctly configured.",
	)
}

// summary renders a short message for the aggregated issue from the kinds of
// findings collected.
func (c *HreflangCheck) summary(builder *domain.IssueBuilder) string {
	var parts []string
	if count := builder.CountByType("invalid_format"); count > 0 {
		parts = append(parts, fmt.Sprintf("%d hreflang tag(s) have an invalid language format", count))
	}
	if count := builder.CountByType("relative_url"); count > 0 {
		parts = append(parts, fmt.Sprintf("%d hreflang link(s) use a relative URL", count))
	}
	if count := builder.CountByType("duplicate_code"); count > 0 {
		parts = append(parts, fmt.Sprintf("%d language code(s) are declared more than once", count))
	}
	if builder.CountByType("missing_self_reference") > 0 {
		parts = append(parts, "A self-referencing hreflang link to the current URL is missing")
	}
	return strings.Join(parts, "; ") + "."
}

func (c *HreflangCheck) findHreflangTags(root *html.Node) []hreflangTag {
	var tags []hreflangTag

	var traverse func(n *html.Node, inHead bool)
	traverse = func(n *html.Node, inHead bool) {
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, "head") {
			inHead = true
		}

		if inHead && n.Type == html.ElementNode && strings.EqualFold(n.Data, "link") {
			var isAlternate bool
			var hreflang string
			var href string

			for _, attr := range n.Attr {
				key := strings.ToLower(attr.Key)
				val := strings.TrimSpace(attr.Val)

				if key == "rel" && strings.EqualFold(val, "alternate") {
					isAlternate = true
				}
				if key == "hreflang" {
					hreflang = val
				}
				if key == "href" {
					href = val
				}
			}

			if isAlternate && hreflang != "" {
				tags = append(tags, hreflangTag{
					Hreflang: hreflang,
					Href:     href,
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
