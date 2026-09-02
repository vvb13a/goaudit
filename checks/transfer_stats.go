package checks

import (
	"context"
	"fmt"

	"github.com/vvb13a/goaudit/domain"
)

type TransferStatsLogCheck struct {
	Severity domain.Severity
}

func NewTransferStatsLogCheck() *TransferStatsLogCheck {
	return &TransferStatsLogCheck{
		Severity: domain.SeverityInfo,
	}
}

func (c *TransferStatsLogCheck) Info() domain.CheckInfo {
	return domain.CheckInfo{
		Name:        "transfer_stats_log",
		Description: "Logs transfer-level timing statistics for the request.",
		Category:    domain.CategoryPerformance,
	}
}

func (c *TransferStatsLogCheck) Supports(doc *domain.Document) bool {
	return true
}

func (c *TransferStatsLogCheck) Apply(ctx context.Context, doc *domain.Document) []domain.Issue {
	if doc.TransferStats == nil {
		return []domain.Issue{
			domain.NewPassIssueWithDetails(
				c,
				"Transfer stats were not collected for this request.",
				map[string]any{
					"note": "To enable, ensure httptrace is configured on the Fetcher.",
				},
			),
		}
	}

	stats := doc.TransferStats
	totalTimeMs := stats.TotalTime.Milliseconds()

	details := map[string]any{
		"dns_lookup_ms":        stats.DNSLookup.Milliseconds(),
		"tcp_connection_ms":    stats.TCPConnection.Milliseconds(),
		"tls_handshake_ms":     stats.TLSHandshake.Milliseconds(),
		"server_processing_ms": stats.ServerProcessing.Milliseconds(),
		"total_time_ms":        totalTimeMs,
		"is_https":             stats.IsHTTPS,
	}

	return []domain.Issue{
		domain.NewFailIssue(
			c,
			c.Severity,
			fmt.Sprintf("Request completed in %dms.", totalTimeMs),
			details,
		),
	}
}
