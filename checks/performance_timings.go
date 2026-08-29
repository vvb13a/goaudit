package checks

import (
	"context"
	"fmt"

	"github.com/vvb13a/goaudit/data"
)

type PerformanceTimingsCheck struct {
	DNSWarningMs              int64
	TCPConnectionWarningMs    int64
	TLSHandshakeWarningMs     int64
	ServerProcessingNoticeMs  int64
	ServerProcessingWarningMs int64
	ServerProcessingErrorMs   int64
	TotalTimeWarningMs        int64
	TotalTimeErrorMs          int64
	Severity                  data.Severity
}

func NewPerformanceTimingsCheck() *PerformanceTimingsCheck {
	return &PerformanceTimingsCheck{
		DNSWarningMs:              100,
		TCPConnectionWarningMs:    100,
		TLSHandshakeWarningMs:     200,
		ServerProcessingNoticeMs:  100,
		ServerProcessingWarningMs: 200,
		ServerProcessingErrorMs:   400,
		TotalTimeWarningMs:        1000,
		TotalTimeErrorMs:          2000,
		Severity:                  data.SeverityWarning,
	}
}

func (c *PerformanceTimingsCheck) Name() string {
	return "performance_timings"
}

func (c *PerformanceTimingsCheck) Checklist() string {
	return "performance"
}

func (c *PerformanceTimingsCheck) Supports(doc *data.Document) bool {
	return true
}

func (c *PerformanceTimingsCheck) Apply(ctx context.Context, doc *data.Document) []data.Issue {
	stats := doc.TransferStats

	if stats == nil {
		return []data.Issue{
			data.NewFailIssue(
				c,
				data.SeverityInfo,
				"Skipped: Transfer stats were not collected. Enable httptrace to collect phase timings.",
				nil,
			),
		}
	}

	var detectedIssues []data.Issue

	dnsTimeMs := stats.DNSLookup.Milliseconds()
	if dnsTimeMs > c.DNSWarningMs {
		detectedIssues = append(detectedIssues, data.NewFailIssue(
			c,
			c.Severity,
			fmt.Sprintf("DNS lookup is slow (%dms).", dnsTimeMs),
			map[string]any{
				"issue_type":   "slow_dns",
				"time_ms":      dnsTimeMs,
				"threshold_ms": c.DNSWarningMs,
			},
		))
	}

	tcpTimeMs := stats.TCPConnection.Milliseconds()
	if tcpTimeMs > c.TCPConnectionWarningMs {
		detectedIssues = append(detectedIssues, data.NewFailIssue(
			c,
			c.Severity,
			fmt.Sprintf("TCP connection is slow (%dms).", tcpTimeMs),
			map[string]any{
				"issue_type":   "slow_tcp",
				"time_ms":      tcpTimeMs,
				"threshold_ms": c.TCPConnectionWarningMs,
			},
		))
	}

	if stats.IsHTTPS && stats.TLSHandshake > 0 {
		tlsTimeMs := stats.TLSHandshake.Milliseconds()
		if tlsTimeMs > c.TLSHandshakeWarningMs {
			detectedIssues = append(detectedIssues, data.NewFailIssue(
				c,
				c.Severity,
				fmt.Sprintf("TLS handshake is slow (%dms).", tlsTimeMs),
				map[string]any{
					"issue_type":   "slow_tls",
					"time_ms":      tlsTimeMs,
					"threshold_ms": c.TLSHandshakeWarningMs,
				},
			))
		}
	}

	serverTimeMs := stats.ServerProcessing.Milliseconds()
	if serverTimeMs > c.ServerProcessingErrorMs {
		detectedIssues = append(detectedIssues, data.NewFailIssue(
			c,
			data.SeverityError,
			fmt.Sprintf("Server processing time is critically slow (%dms).", serverTimeMs),
			map[string]any{
				"issue_type":   "slow_server_critical",
				"time_ms":      serverTimeMs,
				"threshold_ms": c.ServerProcessingErrorMs,
			},
		))
	} else if serverTimeMs > c.ServerProcessingWarningMs {
		detectedIssues = append(detectedIssues, data.NewFailIssue(
			c,
			data.SeverityWarning,
			fmt.Sprintf("Server processing time is slow (%dms).", serverTimeMs),
			map[string]any{
				"issue_type":   "slow_server_warning",
				"time_ms":      serverTimeMs,
				"threshold_ms": c.ServerProcessingWarningMs,
			},
		))
	} else if serverTimeMs > c.ServerProcessingNoticeMs {
		detectedIssues = append(detectedIssues, data.NewFailIssue(
			c,
			data.SeverityNotice,
			fmt.Sprintf("Server processing time is slow (%dms).", serverTimeMs),
			map[string]any{
				"issue_type":   "slow_server_notice",
				"time_ms":      serverTimeMs,
				"threshold_ms": c.ServerProcessingNoticeMs,
			},
		))
	}

	totalTimeMs := stats.TotalTime.Milliseconds()
	if totalTimeMs > c.TotalTimeErrorMs {
		detectedIssues = append(detectedIssues, data.NewFailIssue(
			c,
			data.SeverityError,
			fmt.Sprintf("Total request time is critically slow (%dms).", totalTimeMs),
			map[string]any{
				"issue_type":   "slow_total_critical",
				"time_ms":      totalTimeMs,
				"threshold_ms": c.TotalTimeErrorMs,
			},
		))
	} else if totalTimeMs > c.TotalTimeWarningMs {
		detectedIssues = append(detectedIssues, data.NewFailIssue(
			c,
			c.Severity,
			fmt.Sprintf("Total request time is slow (%dms).", totalTimeMs),
			map[string]any{
				"issue_type":   "slow_total_warning",
				"time_ms":      totalTimeMs,
				"threshold_ms": c.TotalTimeWarningMs,
			},
		))
	}

	if len(detectedIssues) > 0 {
		return detectedIssues
	}

	return []data.Issue{
		data.NewPassIssue(
			c,
			fmt.Sprintf("Performance timings are good (Server: %dms, Total: %dms).", serverTimeMs, totalTimeMs),
			map[string]any{
				"server_ms": serverTimeMs,
				"total_ms":  totalTimeMs,
			},
		),
	}
}
