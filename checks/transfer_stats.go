package checks

import (
	"context"
	"fmt"

	"github.com/vvb13a/goaudit/data"
)

type TransferStatsLogCheck struct {
	Severity data.Severity
}

func NewTransferStatsLogCheck() *TransferStatsLogCheck {
	return &TransferStatsLogCheck{
		Severity: data.SeverityInfo,
	}
}

func (c *TransferStatsLogCheck) Name() string {
	return "transfer_stats_log"
}

func (c *TransferStatsLogCheck) Checklist() string {
	return "performance"
}

func (c *TransferStatsLogCheck) Supports(doc *data.Document) bool {
	return true
}

func (c *TransferStatsLogCheck) Apply(ctx context.Context, doc *data.Document) []data.Issue {
	if doc.TransferStats == nil {
		return []data.Issue{
			data.NewPassIssue(
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

	return []data.Issue{
		data.NewFailIssue(
			c,
			c.Severity,
			fmt.Sprintf("Request completed in %dms.", totalTimeMs),
			details,
		),
	}
}
