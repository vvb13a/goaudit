package checks

import (
	"context"
	"fmt"
	"strings"

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

func (c *PerformanceTimingsCheck) Apply(ctx context.Context, doc *domain.Document) domain.Issue {
	stats := doc.TransferStats

	if stats == nil {
		return domain.NewFailIssue(
			c,
			domain.SeverityInfo,
			"Skipped: Transfer stats were not collected. Enable httptrace to collect phase timings.",
			nil,
		)
	}

	builder := domain.NewIssueBuilder()

	dnsTimeMs := stats.DNSLookup.Milliseconds()
	if dnsTimeMs > c.DNSWarningMs {
		builder.Add(domain.Finding{
			Type:     "slow_dns",
			Severity: c.Severity,
			Message:  fmt.Sprintf("DNS lookup is slow (%dms).", dnsTimeMs),
			Data: map[string]any{
				"time_ms":      dnsTimeMs,
				"threshold_ms": c.DNSWarningMs,
			},
		})
	}

	tcpTimeMs := stats.TCPConnection.Milliseconds()
	if tcpTimeMs > c.TCPConnectionWarningMs {
		builder.Add(domain.Finding{
			Type:     "slow_tcp",
			Severity: c.Severity,
			Message:  fmt.Sprintf("TCP connection is slow (%dms).", tcpTimeMs),
			Data: map[string]any{
				"time_ms":      tcpTimeMs,
				"threshold_ms": c.TCPConnectionWarningMs,
			},
		})
	}

	if stats.IsHTTPS && stats.TLSHandshake > 0 {
		tlsTimeMs := stats.TLSHandshake.Milliseconds()
		if tlsTimeMs > c.TLSHandshakeWarningMs {
			builder.Add(domain.Finding{
				Type:     "slow_tls",
				Severity: c.Severity,
				Message:  fmt.Sprintf("TLS handshake is slow (%dms).", tlsTimeMs),
				Data: map[string]any{
					"time_ms":      tlsTimeMs,
					"threshold_ms": c.TLSHandshakeWarningMs,
				},
			})
		}
	}

	serverTimeMs := stats.ServerProcessing.Milliseconds()
	if serverTimeMs > c.ServerProcessingErrorMs {
		builder.Add(domain.Finding{
			Type:     "slow_server_critical",
			Severity: domain.SeverityError,
			Message:  fmt.Sprintf("Server processing time is critically slow (%dms).", serverTimeMs),
			Data: map[string]any{
				"time_ms":      serverTimeMs,
				"threshold_ms": c.ServerProcessingErrorMs,
			},
		})
	} else if serverTimeMs > c.ServerProcessingWarningMs {
		builder.Add(domain.Finding{
			Type:     "slow_server_warning",
			Severity: domain.SeverityWarning,
			Message:  fmt.Sprintf("Server processing time is slow (%dms).", serverTimeMs),
			Data: map[string]any{
				"time_ms":      serverTimeMs,
				"threshold_ms": c.ServerProcessingWarningMs,
			},
		})
	} else if serverTimeMs > c.ServerProcessingNoticeMs {
		builder.Add(domain.Finding{
			Type:     "slow_server_notice",
			Severity: domain.SeverityNotice,
			Message:  fmt.Sprintf("Server processing time is slow (%dms).", serverTimeMs),
			Data: map[string]any{
				"time_ms":      serverTimeMs,
				"threshold_ms": c.ServerProcessingNoticeMs,
			},
		})
	}

	totalTimeMs := stats.TotalTime.Milliseconds()
	if totalTimeMs > c.TotalTimeErrorMs {
		builder.Add(domain.Finding{
			Type:     "slow_total_critical",
			Severity: domain.SeverityError,
			Message:  fmt.Sprintf("Total request time is critically slow (%dms).", totalTimeMs),
			Data: map[string]any{
				"time_ms":      totalTimeMs,
				"threshold_ms": c.TotalTimeErrorMs,
			},
		})
	} else if totalTimeMs > c.TotalTimeWarningMs {
		builder.Add(domain.Finding{
			Type:     "slow_total_warning",
			Severity: c.Severity,
			Message:  fmt.Sprintf("Total request time is slow (%dms).", totalTimeMs),
			Data: map[string]any{
				"time_ms":      totalTimeMs,
				"threshold_ms": c.TotalTimeWarningMs,
			},
		})
	}

	if builder.HasFindings() {
		return domain.NewFailIssue(c, builder.Severity(), c.summary(builder), builder.Details())
	}

	return domain.NewPassIssueWithDetails(
		c,
		fmt.Sprintf("Performance timings are good (Server: %dms, Total: %dms).", serverTimeMs, totalTimeMs),
		map[string]any{
			"server_ms": serverTimeMs,
			"total_ms":  totalTimeMs,
		},
	)
}

// summary renders a short message for the aggregated issue from the phases
// that were found slow.
func (c *PerformanceTimingsCheck) summary(builder *domain.IssueBuilder) string {
	var parts []string
	if builder.CountByType("slow_dns") > 0 {
		parts = append(parts, "DNS lookup is slow.")
	}
	if builder.CountByType("slow_tcp") > 0 {
		parts = append(parts, "TCP connection is slow.")
	}
	if builder.CountByType("slow_tls") > 0 {
		parts = append(parts, "TLS handshake is slow.")
	}
	switch {
	case builder.CountByType("slow_server_critical") > 0:
		parts = append(parts, "Server processing is critically slow.")
	case builder.CountByType("slow_server_warning") > 0 || builder.CountByType("slow_server_notice") > 0:
		parts = append(parts, "Server processing is slow.")
	}
	switch {
	case builder.CountByType("slow_total_critical") > 0:
		parts = append(parts, "Total request time is critically slow.")
	case builder.CountByType("slow_total_warning") > 0:
		parts = append(parts, "Total request time is slow.")
	}
	return strings.Join(parts, " ")
}
