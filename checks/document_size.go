package checks

import (
	"context"
	"fmt"
	"math"

	"github.com/vvb13a/goaudit/domain"
)

type DocumentSizeCheck struct {
	NoticeThresholdKb  int
	WarningThresholdKb int
	ErrorThresholdKb   int
}

func NewDocumentSizeCheck() *DocumentSizeCheck {
	return &DocumentSizeCheck{
		NoticeThresholdKb:  250,
		WarningThresholdKb: 500,
		ErrorThresholdKb:   1000,
	}
}

func (c *DocumentSizeCheck) Info() domain.CheckInfo {
	return domain.CheckInfo{
		Name:        "document_size",
		Description: "Flags pages whose uncompressed response body is overly large.",
		Category:    domain.CategoryPerformance,
	}
}

func (c *DocumentSizeCheck) Supports(doc *domain.Document) bool {
	return true
}

func (c *DocumentSizeCheck) Apply(ctx context.Context, doc *domain.Document) domain.Issue {
	actualSizeBytes := len(doc.Body)
	actualSizeKb := math.Round((float64(actualSizeBytes)/1024.0)*100) / 100

	errorThresholdBytes := c.ErrorThresholdKb * 1024
	warningThresholdBytes := c.WarningThresholdKb * 1024
	noticeThresholdBytes := c.NoticeThresholdKb * 1024

	if actualSizeBytes > errorThresholdBytes {
		return domain.NewFailIssue(
			c,
			domain.SeverityError,
			fmt.Sprintf("Uncompressed response size (%.2f KB) is critically large, exceeding the error threshold of %d KB.", actualSizeKb, c.ErrorThresholdKb),
			map[string]any{
				"issue_type":   "critical_size",
				"size_kb":      actualSizeKb,
				"threshold_kb": c.ErrorThresholdKb,
			},
		)
	}

	if actualSizeBytes > warningThresholdBytes {
		return domain.NewFailIssue(
			c,
			domain.SeverityWarning,
			fmt.Sprintf("Uncompressed response size (%.2f KB) is large, exceeding the warning threshold of %d KB.", actualSizeKb, c.WarningThresholdKb),
			map[string]any{
				"issue_type":   "large_size",
				"size_kb":      actualSizeKb,
				"threshold_kb": c.WarningThresholdKb,
			},
		)
	}

	if actualSizeBytes > noticeThresholdBytes {
		return domain.NewFailIssue(
			c,
			domain.SeverityNotice,
			fmt.Sprintf("Uncompressed response size (%.2f KB) is large, exceeding the notice threshold of %d KB.", actualSizeKb, c.NoticeThresholdKb),
			map[string]any{
				"issue_type":   "large_size",
				"size_kb":      actualSizeKb,
				"threshold_kb": c.NoticeThresholdKb,
			},
		)
	}

	return domain.NewPassIssueWithDetails(
		c,
		fmt.Sprintf("Uncompressed response size (%.2f KB) is within acceptable limits.", actualSizeKb),
		map[string]any{
			"size_kb": actualSizeKb,
		},
	)
}
