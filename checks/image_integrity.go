package checks

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/vvb13a/goaudit/domain"

	"golang.org/x/net/html"
)

type ImageIntegrityCheck struct {
	ExcludeJSSrc            bool
	FlagEmptyAlt            bool
	EmptyMissingSrcSeverity domain.Severity
	MissingAltSeverity      domain.Severity
	EmptyAltSeverity        domain.Severity
}

func NewImageIntegrityCheck() *ImageIntegrityCheck {
	return &ImageIntegrityCheck{
		ExcludeJSSrc:            true,
		FlagEmptyAlt:            true,
		EmptyMissingSrcSeverity: domain.SeverityError,
		MissingAltSeverity:      domain.SeverityWarning,
		EmptyAltSeverity:        domain.SeverityWarning,
	}
}

func (c *ImageIntegrityCheck) Info() domain.CheckInfo {
	return domain.CheckInfo{
		Name:        "image_integrity",
		Description: "Checks that images have a src attribute and appropriate alt text.",
		Category:    domain.CategoryAccessibility,
	}
}

func (c *ImageIntegrityCheck) Supports(doc *domain.Document) bool {
	return doc.IsHTML()
}

func (c *ImageIntegrityCheck) Apply(ctx context.Context, doc *domain.Document) domain.Issue {
	root, err := html.Parse(bytes.NewReader(doc.Body))
	if err != nil {
		return domain.NewFailIssue(
			c,
			domain.SeverityError,
			fmt.Sprintf("Error processing images: %s", err.Error()),
			nil,
		)
	}

	images := c.findImages(root)

	if len(images) == 0 {
		return domain.NewPassIssue(
			c,
			"No images found on the page to check.",
		)
	}

	builder := domain.NewIssueBuilder()

	for _, img := range images {
		src, hasSrc := c.getAttribute(img, "src")
		src = strings.TrimSpace(src)

		if !hasSrc || src == "" {
			builder.Add(domain.Finding{
				Type:     "missing_src",
				Severity: c.EmptyMissingSrcSeverity,
				Message:  "Image tag is missing the \"src\" attribute or it is empty.",
			})
			continue
		}

		altText, hasAlt := c.getAttribute(img, "alt")
		if !hasAlt {
			builder.Add(domain.Finding{
				Type:     "missing_alt",
				Severity: c.MissingAltSeverity,
				Message:  "Image is missing the alt attribute.",
				Data: map[string]any{
					"image_src": src,
				},
			})
			continue
		}

		altText = strings.TrimSpace(altText)
		if c.FlagEmptyAlt && altText == "" {
			builder.Add(domain.Finding{
				Type:     "empty_alt",
				Severity: c.EmptyAltSeverity,
				Message:  "Image has an empty alt attribute (alt=\"\"). This may be intentional for decorative images.",
				Data: map[string]any{
					"image_src": src,
				},
			})
		}
	}

	if builder.HasFindings() {
		return domain.NewFailIssue(c, builder.Severity(), c.summary(builder), builder.Details())
	}

	return domain.NewPassIssue(
		c,
		"All images have valid src and appropriate alt attributes.",
	)
}

// summary renders a short message for the aggregated issue from the kinds of
// findings collected.
func (c *ImageIntegrityCheck) summary(builder *domain.IssueBuilder) string {
	var parts []string
	if count := builder.CountByType("missing_src"); count > 0 {
		parts = append(parts, fmt.Sprintf("%d image(s) are missing a src attribute", count))
	}
	if count := builder.CountByType("missing_alt"); count > 0 {
		parts = append(parts, fmt.Sprintf("%d image(s) are missing an alt attribute", count))
	}
	if count := builder.CountByType("empty_alt"); count > 0 {
		parts = append(parts, fmt.Sprintf("%d image(s) have an empty alt attribute", count))
	}
	return strings.Join(parts, "; ") + "."
}

func (c *ImageIntegrityCheck) findImages(root *html.Node) []*html.Node {
	var images []*html.Node

	var traverse func(n *html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, "img") {
			if c.ExcludeJSSrc && c.hasAttribute(n, ":src") {
				// Exclude dynamic JS src binding
			} else {
				images = append(images, n)
			}
		}

		for child := n.FirstChild; child != nil; child = child.NextSibling {
			traverse(child)
		}
	}

	traverse(root)
	return images
}

func (c *ImageIntegrityCheck) getAttribute(n *html.Node, attrName string) (string, bool) {
	for _, attr := range n.Attr {
		if strings.EqualFold(attr.Key, attrName) {
			return attr.Val, true
		}
	}
	return "", false
}

func (c *ImageIntegrityCheck) hasAttribute(n *html.Node, attrName string) bool {
	for _, attr := range n.Attr {
		if strings.EqualFold(attr.Key, attrName) {
			return true
		}
	}
	return false
}
