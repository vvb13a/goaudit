package domain

import "time"

// GraphSnapshot is the graph state of an audit after a run: the headline counts
// and the filetype distribution of the stored graph. It is the graph
// counterpart of AuditSnapshot and is captured only when the run actually
// performed graph extraction.
type GraphSnapshot struct {
	ID            string          `json:"id"`
	AuditID       string          `json:"audit_id"`
	CreatedAt     time.Time       `json:"created_at"`
	TotalNodes    int             `json:"total_nodes"`
	TotalEdges    int             `json:"total_edges"`
	InternalNodes int             `json:"internal_nodes"`
	ExternalNodes int             `json:"external_nodes"`
	RootNodes     int             `json:"root_nodes"`
	Filetypes     []FiletypeCount `json:"filetypes"`
	Duration      time.Duration   `json:"duration_ns"`
}
