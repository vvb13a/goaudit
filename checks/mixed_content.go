package checks

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/vvb13a/goaudit/data"

	"golang.org/x/net/html"
)

type MixedContentCheck struct {
	Severity data.Severity
}

func NewMixedContentCheck() *MixedContentCheck {
	return &MixedContentCheck{
		Severity: data.SeverityError,
	}
}

func (c *MixedContentCheck) Name() string {
	return "mixed_content"
}

func (c *MixedContentCheck) Checklist() string {
	return "security"
}

func (c *MixedContentCheck) Supports(doc *data.Document) bool {
	return doc.IsHTML()
}

type insecureAsset struct {
	Tag      string
	AssetURL string
}

func (c *MixedContentCheck) Apply(ctx context.Context, doc *data.Document) []data.Issue {
	if strings.HasPrefix(doc.URL, "http://") {
		return []data.Issue{
			data.NewPassIssue(
				c,
				"Skipped: Page is not loaded over HTTPS.",
				nil,
			),
		}
	}

	root, err := html.Parse(bytes.NewReader(doc.Body))
	if err != nil {
		return []data.Issue{
			data.NewFailIssue(
				c,
				data.SeverityError,
				fmt.Sprintf("Error during mixed content check: %s", err.Error()),
				nil,
			),
		}
	}

	assets := c.findInsecureAssets(root)

	if len(assets) > 0 {
		var detectedIssues []data.Issue
		for _, asset := range assets {
			detectedIssues = append(detectedIssues, data.NewFailIssue(
				c,
				c.Severity,
				"Insecure asset loaded on a secure page (mixed content).",
				map[string]any{
					"issue_type": "mixed_content",
					"tag":        asset.Tag,
					"asset_url":  asset.AssetURL,
				},
			))
		}
		return detectedIssues
	}

	return []data.Issue{
		data.NewPassIssue(
			c,
			"No mixed content found on the page.",
			nil,
		),
	}
}

func (c *MixedContentCheck) findInsecureAssets(root *html.Node) []insecureAsset {
	var assets []insecureAsset

	var traverse func(n *html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode {
			tag := strings.ToLower(n.Data)

			switch tag {
			case "img", "script", "video", "audio", "source", "iframe":
				if src, ok := c.getAttribute(n, "src"); ok {
					src = strings.TrimSpace(src)
					if strings.HasPrefix(src, "http://") {
						assets = append(assets, insecureAsset{
							Tag:      tag,
							AssetURL: src,
						})
					}
				}

			case "link":
				rel, _ := c.getAttribute(n, "rel")
				if strings.EqualFold(strings.TrimSpace(rel), "stylesheet") {
					if href, ok := c.getAttribute(n, "href"); ok {
						href = strings.TrimSpace(href)
						if strings.HasPrefix(href, "http://") {
							assets = append(assets, insecureAsset{
								Tag:      tag,
								AssetURL: href,
							})
						}
					}
				}
			}
		}

		for child := n.FirstChild; child != nil; child = child.NextSibling {
			traverse(child)
		}
	}

	traverse(root)
	return assets
}

func (c *MixedContentCheck) getAttribute(n *html.Node, attrName string) (string, bool) {
	for _, attr := range n.Attr {
		if strings.EqualFold(attr.Key, attrName) {
			return attr.Val, true
		}
	}
	return "", false
}
