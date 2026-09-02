package checks

import (
	"bytes"
	"context"
	"fmt"

	"github.com/vvb13a/goaudit/domain"

	"golang.org/x/net/html"
)

type DomSizeCheck struct {
	NoticeThreshold  int
	WarningThreshold int
	ErrorThreshold   int
}

func NewDomSizeCheck() *DomSizeCheck {
	return &DomSizeCheck{
		NoticeThreshold:  1000,
		WarningThreshold: 2000,
		ErrorThreshold:   3000,
	}
}

func (c *DomSizeCheck) Info() domain.CheckInfo {
	return domain.CheckInfo{
		Name:        "dom_size",
		Description: "Flags pages with excessively large DOM node counts.",
		Category:    domain.CategoryPerformance,
	}
}

func (c *DomSizeCheck) Supports(doc *domain.Document) bool {
	return doc.IsHTML()
}

func (c *DomSizeCheck) Apply(ctx context.Context, doc *domain.Document) []domain.Issue {
	node, err := html.Parse(bytes.NewReader(doc.Body))
	if err != nil {
		return []domain.Issue{
			domain.NewFailIssue(
				c,
				domain.SeverityError,
				fmt.Sprintf("Error during DOM size check: %s", err.Error()),
				nil,
			),
		}
	}

	domNodeCount := c.countElementNodes(node)

	// Rule 1: Error Threshold
	if domNodeCount > c.ErrorThreshold {
		return []domain.Issue{
			domain.NewFailIssue(
				c,
				domain.SeverityError,
				fmt.Sprintf("DOM size is critically large (%d elements), exceeding the error threshold of %d.", domNodeCount, c.ErrorThreshold),
				map[string]any{
					"issue_type": "critical_size",
					"node_count": domNodeCount,
					"threshold":  c.ErrorThreshold,
				},
			),
		}
	}

	// Rule 2: Warning Threshold
	if domNodeCount > c.WarningThreshold {
		return []domain.Issue{
			domain.NewFailIssue(
				c,
				domain.SeverityWarning,
				fmt.Sprintf("DOM size is large (%d elements), exceeding the warning threshold of %d.", domNodeCount, c.WarningThreshold),
				map[string]any{
					"issue_type": "large_size",
					"node_count": domNodeCount,
					"threshold":  c.WarningThreshold,
				},
			),
		}
	}

	// Rule 3: Notice Threshold
	if domNodeCount > c.NoticeThreshold {
		return []domain.Issue{
			domain.NewFailIssue(
				c,
				domain.SeverityNotice,
				fmt.Sprintf("DOM size is large (%d elements), exceeding the notice threshold of %d.", domNodeCount, c.NoticeThreshold),
				map[string]any{
					"issue_type": "large_size",
					"node_count": domNodeCount,
					"threshold":  c.NoticeThreshold,
				},
			),
		}
	}

	return []domain.Issue{
		domain.NewPassIssueWithDetails(
			c,
			fmt.Sprintf("DOM size (%d elements) is within acceptable limits.", domNodeCount),
			map[string]any{
				"node_count": domNodeCount,
			},
		),
	}
}

func (c *DomSizeCheck) countElementNodes(n *html.Node) int {
	count := 0
	if n.Type == html.ElementNode {
		count++
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		count += c.countElementNodes(child)
	}
	return count
}
