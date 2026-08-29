package checks

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/vvb13a/goaudit/data"

	"golang.org/x/net/html"
)

var hreflangRegex = regexp.MustCompile(`^[a-z]{2}(-[A-Z]{2})?$|^x-default$`)

type HreflangCheck struct {
	FormatSeverity               data.Severity
	RelativeURLSeverity          data.Severity
	MissingSelfReferenceSeverity data.Severity
}

func NewHreflangCheck() *HreflangCheck {
	return &HreflangCheck{
		FormatSeverity:               data.SeverityError,
		RelativeURLSeverity:          data.SeverityError,
		MissingSelfReferenceSeverity: data.SeverityWarning,
	}
}

func (c *HreflangCheck) Name() string {
	return "hreflang"
}

func (c *HreflangCheck) Checklist() string {
	return "seo"
}

func (c *HreflangCheck) Supports(doc *data.Document) bool {
	return doc.IsHTML()
}

type hreflangTag struct {
	Hreflang string
	Href     string
}

func (c *HreflangCheck) Apply(ctx context.Context, doc *data.Document) []data.Issue {
	root, err := html.Parse(bytes.NewReader(doc.Body))
	if err != nil {
		return []data.Issue{
			data.NewFailIssue(
				c,
				data.SeverityError,
				fmt.Sprintf("Error during hreflang check: %s", err.Error()),
				nil,
			),
		}
	}

	tags := c.findHreflangTags(root)

	if len(tags) == 0 {
		return []data.Issue{
			data.NewPassIssue(
				c,
				"No hreflang tags found on the page.",
				nil,
			),
		}
	}

	var detectedIssues []data.Issue
	languageCodes := make(map[string]string)
	hasSelfReference := false
	url := doc.URL

	for _, tag := range tags {
		hreflang := tag.Hreflang
		href := tag.Href

		if !hreflangRegex.MatchString(hreflang) {
			detectedIssues = append(detectedIssues, data.NewFailIssue(
				c,
				c.FormatSeverity,
				fmt.Sprintf("Hreflang attribute '%s' has an invalid format.", hreflang),
				map[string]any{
					"issue_type": "invalid_format",
					"hreflang":   hreflang,
					"href":       href,
				},
			))
		}

		if !strings.HasPrefix(href, "http://") && !strings.HasPrefix(href, "https://") {
			detectedIssues = append(detectedIssues, data.NewFailIssue(
				c,
				c.RelativeURLSeverity,
				fmt.Sprintf("Hreflang link for '%s' must use an absolute URL.", hreflang),
				map[string]any{
					"issue_type": "relative_url",
					"hreflang":   hreflang,
					"href":       href,
				},
			))
		}

		if _, exists := languageCodes[hreflang]; exists {
			detectedIssues = append(detectedIssues, data.NewFailIssue(
				c,
				c.FormatSeverity,
				fmt.Sprintf("Duplicate hreflang tag found for language code '%s'.", hreflang),
				map[string]any{
					"issue_type": "duplicate_code",
					"hreflang":   hreflang,
				},
			))
		}
		languageCodes[hreflang] = href

		if href == url {
			hasSelfReference = true
		}
	}

	if !hasSelfReference {
		detectedIssues = append(detectedIssues, data.NewFailIssue(
			c,
			c.MissingSelfReferenceSeverity,
			"Hreflang tags are present, but a self-referencing link pointing to the current URL is missing.",
			map[string]any{
				"issue_type":  "missing_self_reference",
				"current_url": url,
			},
		))
	}

	if len(detectedIssues) > 0 {
		return detectedIssues
	}

	return []data.Issue{
		data.NewPassIssue(
			c,
			"All hreflang tags are present and correctly configured.",
			nil,
		),
	}
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
