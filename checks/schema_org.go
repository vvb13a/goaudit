package checks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vvb13a/goaudit/domain"

	"golang.org/x/net/html"
)

type SchemaCheck struct {
	MissingSeverity     domain.Severity
	InvalidJSONSeverity domain.Severity
}

func NewSchemaCheck() *SchemaCheck {
	return &SchemaCheck{
		MissingSeverity:     domain.SeverityError,
		InvalidJSONSeverity: domain.SeverityError,
	}
}

func (c *SchemaCheck) Info() domain.CheckInfo {
	return domain.CheckInfo{
		Name:        "schema",
		Description: "Checks for a Schema.org JSON-LD script with valid JSON content.",
		Category:    domain.CategorySEO,
	}
}

func (c *SchemaCheck) Supports(doc *domain.Document) bool {
	return doc.IsHTML()
}

func (c *SchemaCheck) Apply(ctx context.Context, doc *domain.Document) []domain.Issue {
	root, err := html.Parse(bytes.NewReader(doc.Body))
	if err != nil {
		return []domain.Issue{
			domain.NewFailIssue(
				c,
				domain.SeverityError,
				fmt.Sprintf("Error during Schema.org check: %s", err.Error()),
				nil,
			),
		}
	}

	schemaScripts := c.findJSONLDScripts(root)

	if len(schemaScripts) == 0 {
		return []domain.Issue{
			domain.NewFailIssue(
				c,
				c.MissingSeverity,
				"No Schema.org script tag (script[type=\"application/ld+json\"]) was found on the page.",
				map[string]any{
					"issue_type": "missing",
				},
			),
		}
	}

	jsonContent := strings.TrimSpace(schemaScripts[0])

	if jsonContent == "" {
		return []domain.Issue{
			domain.NewFailIssue(
				c,
				c.InvalidJSONSeverity,
				"A Schema.org script tag was found, but its content is empty.",
				map[string]any{
					"issue_type": "empty_content",
				},
			),
		}
	}

	var rawJSON any
	if err := json.Unmarshal([]byte(jsonContent), &rawJSON); err != nil {
		return []domain.Issue{
			domain.NewFailIssue(
				c,
				c.InvalidJSONSeverity,
				"The content of the Schema.org script tag is not valid JSON.",
				map[string]any{
					"issue_type": "invalid_json",
					"error":      err.Error(),
				},
			),
		}
	}

	return []domain.Issue{
		domain.NewPassIssue(
			c,
			"Schema (JSON-LD) is present and contains valid JSON.",
		),
	}
}

func (c *SchemaCheck) findJSONLDScripts(root *html.Node) []string {
	var scripts []string

	var traverse func(n *html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, "script") {
			for _, attr := range n.Attr {
				if strings.EqualFold(attr.Key, "type") && strings.EqualFold(strings.TrimSpace(attr.Val), "application/ld+json") {
					content := c.extractText(n)
					scripts = append(scripts, content)
					break
				}
			}
		}

		for child := n.FirstChild; child != nil; child = child.NextSibling {
			traverse(child)
		}
	}

	traverse(root)
	return scripts
}

func (c *SchemaCheck) extractText(n *html.Node) string {
	var buf strings.Builder
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.TextNode {
			buf.WriteString(child.Data)
		}
	}
	return buf.String()
}
