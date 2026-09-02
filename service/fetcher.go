package service

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptrace"
	"strings"
	"time"

	"github.com/vvb13a/goaudit/domain"
)

type Fetcher struct {
	client    *http.Client
	userAgent string
	headers   map[string]string
}

func NewFetcher() *Fetcher {
	return &Fetcher{
		client: &http.Client{
			Timeout: 15 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return http.ErrUseLastResponse
				}
				return nil
			},
		},
		userAgent: "GoAuditEngine/1.0",
		headers:   make(map[string]string),
	}
}

func (f *Fetcher) WithTimeout(timeout time.Duration) *Fetcher {
	f.client.Timeout = timeout
	return f
}

func (f *Fetcher) WithUserAgent(userAgent string) *Fetcher {
	f.userAgent = userAgent
	return f
}

func (f *Fetcher) WithHeader(key, value string) *Fetcher {
	f.headers[key] = value
	return f
}

func (f *Fetcher) WithHeaders(headers map[string]string) *Fetcher {
	maps.Copy(f.headers, headers)
	return f
}

func (f *Fetcher) Fetch(ctx context.Context, targetURL string) (*domain.Document, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", f.userAgent)
	for k, v := range f.headers {
		req.Header.Set(k, v)
	}

	var (
		dnsStart, dnsDone      time.Time
		connStart, connDone    time.Time
		tlsStart, tlsDone      time.Time
		wroteReq, gotFirstByte time.Time
	)

	trace := &httptrace.ClientTrace{
		DNSStart:             func(_ httptrace.DNSStartInfo) { dnsStart = time.Now() },
		DNSDone:              func(_ httptrace.DNSDoneInfo) { dnsDone = time.Now() },
		ConnectStart:         func(_, _ string) { connStart = time.Now() },
		ConnectDone:          func(_, _ string, _ error) { connDone = time.Now() },
		TLSHandshakeStart:    func() { tlsStart = time.Now() },
		TLSHandshakeDone:     func(_ tls.ConnectionState, _ error) { tlsDone = time.Now() },
		WroteRequest:         func(_ httptrace.WroteRequestInfo) { wroteReq = time.Now() },
		GotFirstResponseByte: func() { gotFirstByte = time.Now() },
	}

	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))

	start := time.Now()
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("network error: %w", err)
	}
	defer resp.Body.Close()

	// Read body capped at 10MB
	const maxBodySize = 10 * 1024 * 1024
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	totalTime := time.Since(start)

	stats := &domain.TransferStats{
		TotalTime: totalTime,
		IsHTTPS:   strings.EqualFold(resp.Request.URL.Scheme, "https"),
	}

	if !dnsStart.IsZero() && !dnsDone.IsZero() {
		stats.DNSLookup = dnsDone.Sub(dnsStart)
	}
	if !connStart.IsZero() && !connDone.IsZero() {
		stats.TCPConnection = connDone.Sub(connStart)
	}
	if !tlsStart.IsZero() && !tlsDone.IsZero() {
		stats.TLSHandshake = tlsDone.Sub(tlsStart)
	}
	if !wroteReq.IsZero() && !gotFirstByte.IsZero() {
		stats.ServerProcessing = gotFirstByte.Sub(wroteReq)
	}

	return &domain.Document{
		URL:           targetURL,
		FinalURL:      resp.Request.URL.String(),
		StatusCode:    resp.StatusCode,
		Headers:       resp.Header.Clone(),
		Body:          body,
		Duration:      totalTime,
		FetchedAt:     start,
		TransferStats: stats,
	}, nil
}
