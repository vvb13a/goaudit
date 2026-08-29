package data

import (
	"mime"
	"net/http"
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
	return d.ContentType() == "application/json"
}
