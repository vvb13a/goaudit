package checks

import (
	"context"
	"fmt"

	"github.com/vvb13a/goaudit/domain"
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
	Severity                  domain.Severity
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
		Severity:                  domain.SeverityWarning,
	}
}

func (c *PerformanceTimingsCheck) Info() domain.CheckInfo {
	return domain.CheckInfo{
		Name:        "performance_timings",
		Description: "Flags slow DNS, connection, TLS, server processing, and total request timings.",
		Category:    domain.CategoryPerformance,
	}
}

func (c *PerformanceTimingsCheck) Supports(doc *domain.Document) bool {
	return true
}

func (c *PerformanceTimingsCheck) Apply(ctx context.Context, doc *domain.Document) []domain.Issue {
	stats := doc.TransferStats

	if stats == nil {
		return []domain.Issue{
			domain.NewFailIssue(
				c,
				domain.SeverityInfo,
				"Skipped: Transfer stats were not collected. Enable httptrace to collect phase timings.",
				nil,
			),
		}
	}

	var detectedIssues []domain.Issue

	dnsTimeMs := stats.DNSLookup.Milliseconds()
	if dnsTimeMs > c.DNSWarningMs {
		detectedIssues = append(detectedIssues, domain.NewFailIssue(
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
		detectedIssues = append(detectedIssues, domain.NewFailIssue(
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
			detectedIssues = append(detectedIssues, domain.NewFailIssue(
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
		detectedIssues = append(detectedIssues, domain.NewFailIssue(
			c,
			domain.SeverityError,
			fmt.Sprintf("Server processing time is critically slow (%dms).", serverTimeMs),
			map[string]any{
				"issue_type":   "slow_server_critical",
				"time_ms":      serverTimeMs,
				"threshold_ms": c.ServerProcessingErrorMs,
			},
		))
	} else if serverTimeMs > c.ServerProcessingWarningMs {
		detectedIssues = append(detectedIssues, domain.NewFailIssue(
			c,
			domain.SeverityWarning,
			fmt.Sprintf("Server processing time is slow (%dms).", serverTimeMs),
			map[string]any{
				"issue_type":   "slow_server_warning",
				"time_ms":      serverTimeMs,
				"threshold_ms": c.ServerProcessingWarningMs,
			},
		))
	} else if serverTimeMs > c.ServerProcessingNoticeMs {
		detectedIssues = append(detectedIssues, domain.NewFailIssue(
			c,
			domain.SeverityNotice,
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
		detectedIssues = append(detectedIssues, domain.NewFailIssue(
			c,
			domain.SeverityError,
			fmt.Sprintf("Total request time is critically slow (%dms).", totalTimeMs),
			map[string]any{
				"issue_type":   "slow_total_critical",
				"time_ms":      totalTimeMs,
				"threshold_ms": c.TotalTimeErrorMs,
			},
		))
	} else if totalTimeMs > c.TotalTimeWarningMs {
		detectedIssues = append(detectedIssues, domain.NewFailIssue(
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

	return []domain.Issue{
		domain.NewPassIssueWithDetails(
			c,
			fmt.Sprintf("Performance timings are good (Server: %dms, Total: %dms).", serverTimeMs, totalTimeMs),
			map[string]any{
				"server_ms": serverTimeMs,
				"total_ms":  totalTimeMs,
			},
		),
	}
}
