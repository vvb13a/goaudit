package domain

import (
	"mime"
	"net/http"
	"strings"
	"time"
)

type TransferStats struct {
	DNSLookup        time.Duration `json:"dns_lookup"`
	TCPConnection    time.Duration `json:"tcp_connection"`
	TLSHandshake     time.Duration `json:"tls_handshake"`
	ServerProcessing time.Duration `json:"server_processing"`
	TotalTime        time.Duration `json:"total_time"`
	IsHTTPS          bool          `json:"is_https"`
}

type Document struct {
	URL           string         `json:"url"`
	FinalURL      string         `json:"final_url"`
	StatusCode    int            `json:"status_code"`
	Headers       http.Header    `json:"headers"`
	Body          []byte         `json:"-"`
	Duration      time.Duration  `json:"duration"`
	FetchedAt     time.Time      `json:"fetched_at"`
	TransferStats *TransferStats `json:"transfer_stats,omitempty"`
}

func (d *Document) ContentType() string {
	mediaType, _, _ := mime.ParseMediaType(d.Headers.Get("Content-Type"))
	return mediaType
}

func (d *Document) IsHTML() bool {
	return d.ContentType() == "text/html"
}

func (d *Document) IsJSON() bool {
	ct := d.ContentType()
	return ct == "application/json" || strings.HasSuffix(ct, "+json")
}

func (d *Document) IsSuccess() bool {
	return d.StatusCode >= 200 && d.StatusCode < 300
}

func (d *Document) IsRedirect() bool {
	return d.StatusCode >= 300 && d.StatusCode < 400
}

func (d *Document) WasRedirected() bool {
	return d.URL != "" && d.FinalURL != "" && d.URL != d.FinalURL
}

func (d *Document) Header(key string) string {
	return d.Headers.Get(key)
}

func (d *Document) HasHeader(key string) bool {
	return strings.TrimSpace(d.Headers.Get(key)) != ""
}

func (d *Document) SizeBytes() int {
	return len(d.Body)
}

func (d *Document) BodyString() string {
	return string(d.Body)
}
