package checks

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/vvb13a/goaudit/domain"

	"golang.org/x/net/html"
)

type MixedContentCheck struct {
	Severity domain.Severity
}

func NewMixedContentCheck() *MixedContentCheck {
	return &MixedContentCheck{
		Severity: domain.SeverityError,
	}
}

func (c *MixedContentCheck) Info() domain.CheckInfo {
	return domain.CheckInfo{
		Name:        "mixed_content",
		Description: "Flags insecure (HTTP) subresources loaded on HTTPS pages.",
		Category:    domain.CategorySecurity,
	}
}

func (c *MixedContentCheck) Supports(doc *domain.Document) bool {
	return doc.IsHTML()
}

type insecureAsset struct {
	Tag      string
	AssetURL string
}

func (c *MixedContentCheck) Apply(ctx context.Context, doc *domain.Document) domain.Issue {
	if strings.HasPrefix(doc.URL, "http://") {
		return domain.NewPassIssue(
			c,
			"Skipped: Page is not loaded over HTTPS.",
		)
	}

	root, err := html.Parse(bytes.NewReader(doc.Body))
	if err != nil {
		return domain.NewFailIssue(
			c,
			domain.SeverityError,
			fmt.Sprintf("Error during mixed content check: %s", err.Error()),
			nil,
		)
	}

	assets := c.findInsecureAssets(root)

	if len(assets) > 0 {
		builder := domain.NewIssueBuilder()
		for _, asset := range assets {
			builder.Add(domain.Finding{
				Type:     "mixed_content",
				Severity: c.Severity,
				Message:  "Insecure asset loaded on a secure page (mixed content).",
				Data: map[string]any{
					"tag":       asset.Tag,
					"asset_url": asset.AssetURL,
				},
			})
		}
		return domain.NewFailIssue(
			c,
			builder.Severity(),
			fmt.Sprintf("%d insecure asset(s) are loaded over HTTP on a secure page (mixed content).", builder.Count()),
			builder.Details(),
		)
	}

	return domain.NewPassIssue(
		c,
		"No mixed content found on the page.",
	)
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
