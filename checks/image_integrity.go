package checks

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/vvb13a/goaudit/data"

	"golang.org/x/net/html"
)

type ImageIntegrityCheck struct {
	ExcludeJSSrc            bool
	FlagEmptyAlt            bool
	EmptyMissingSrcSeverity data.Severity
	MissingAltSeverity      data.Severity
	EmptyAltSeverity        data.Severity
}

func NewImageIntegrityCheck() *ImageIntegrityCheck {
	return &ImageIntegrityCheck{
		ExcludeJSSrc:            true,
		FlagEmptyAlt:            true,
		EmptyMissingSrcSeverity: data.SeverityError,
		MissingAltSeverity:      data.SeverityWarning,
		EmptyAltSeverity:        data.SeverityWarning,
	}
}

func (c *ImageIntegrityCheck) Name() string {
	return "image_integrity"
}

func (c *ImageIntegrityCheck) Checklist() string {
	return "accessibility"
}

func (c *ImageIntegrityCheck) Supports(doc *data.Document) bool {
	return doc.IsHTML()
}

func (c *ImageIntegrityCheck) Apply(ctx context.Context, doc *data.Document) []data.Issue {
	root, err := html.Parse(bytes.NewReader(doc.Body))
	if err != nil {
		return []data.Issue{
			data.NewFailIssue(
				c,
				data.SeverityError,
				fmt.Sprintf("Error processing images: %s", err.Error()),
				nil,
			),
		}
	}

	images := c.findImages(root)

	if len(images) == 0 {
		return []data.Issue{
			data.NewPassIssue(
				c,
				"No images found on the page to check.",
				nil,
			),
		}
	}

	var detectedIssues []data.Issue

	for _, img := range images {
		src, hasSrc := c.getAttribute(img, "src")
		src = strings.TrimSpace(src)

		if !hasSrc || src == "" {
			detectedIssues = append(detectedIssues, data.NewFailIssue(
				c,
				c.EmptyMissingSrcSeverity,
				"Image tag is missing the \"src\" attribute or it is empty.",
				map[string]any{
					"issue_type": "missing_src",
				},
			))
			continue
		}

		altText, hasAlt := c.getAttribute(img, "alt")
		if !hasAlt {
			detectedIssues = append(detectedIssues, data.NewFailIssue(
				c,
				c.MissingAltSeverity,
				"Image is missing the alt attribute.",
				map[string]any{
					"issue_type": "missing_alt",
					"image_src":  src,
				},
			))
			continue
		}

		altText = strings.TrimSpace(altText)
		if c.FlagEmptyAlt && altText == "" {
			detectedIssues = append(detectedIssues, data.NewFailIssue(
				c,
				c.EmptyAltSeverity,
				"Image has an empty alt attribute (alt=\"\"). This may be intentional for decorative images.",
				map[string]any{
					"issue_type": "empty_alt",
					"image_src":  src,
				},
			))
		}
	}

	if len(detectedIssues) > 0 {
		return detectedIssues
	}

	return []data.Issue{
		data.NewPassIssue(
			c,
			"All images have valid src and appropriate alt attributes.",
			nil,
		),
	}
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
