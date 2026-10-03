package domain

import (
	"errors"
	"time"
)

// ErrGraphNodeNotFound is returned when a graph node id does not belong to the
// audit.
var ErrGraphNodeNotFound = errors.New("graph node not found")

// GraphNode is one unique resource in an audit's link graph, identified by its
// normalized absolute URL. Filetype is a coarse content classification (html,
// css, js, jpg, webp, ...) derived from the URL and the referencing element.
// External marks hosts outside the audit's target hosts. IsRoot marks the
// configured target URLs. InLinks/OutLinks are the reference counts, computed
// when the graph is persisted.
type GraphNode struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Filetype  string    `json:"filetype"`
	External  bool      `json:"external"`
	IsRoot    bool      `json:"is_root"`
	Audited   bool      `json:"audited"`
	InLinks   int       `json:"in_links"`
	OutLinks  int       `json:"out_links"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`

	// StatusCode and LastValidated are the HTTP status and the time the node
	// was last fetched: from the page fetch for audited pages, from the link
	// validation result for targets. Zero means not (yet) validated.
	StatusCode    int       `json:"status_code"`
	LastValidated time.Time `json:"last_validated"`
}

// GraphEdge is a directed reference from a source node to a target node. Type
// is the reference kind (hyperlink, stylesheet, script, image, ...); Count
// aggregates repeated references of the same kind between the same pair.
type GraphEdge struct {
	ID           string `json:"id"`
	SourceNodeID string `json:"source_node_id"`
	TargetNodeID string `json:"target_node_id"`
	Type         string `json:"type"`
	Count        int    `json:"count"`
}

// RawLink is one outbound reference extracted from a fetched document before
// it is resolved into nodes and edges. URL is already absolute and
// fragment-free; Filetype is the hint from the referencing element, used when
// the target URL has no recognizable extension.
type RawLink struct {
	URL      string `json:"url"`
	Type     string `json:"type"`
	Filetype string `json:"filetype"`
}

// GraphEdgeRow is a denormalized edge for display: the edge plus the source and
// target node URLs and filetypes, so the API does not need an N+1 lookup.
type GraphEdgeRow struct {
	ID             string `json:"id"`
	Type           string `json:"type"`
	Count          int    `json:"count"`
	SourceNodeID   string `json:"source_node_id"`
	SourceURL      string `json:"source_url"`
	SourceFiletype string `json:"source_filetype"`
	TargetNodeID   string `json:"target_node_id"`
	TargetURL      string `json:"target_url"`
	TargetFiletype string `json:"target_filetype"`
	TargetExternal bool   `json:"target_external"`
}

// GraphNodeFilter narrows a node listing. The Seen bounds filter on the
// first/last-seen timestamps.
type GraphNodeFilter struct {
	Filetypes         []string
	External          *bool
	IsRoot            *bool
	Search            string
	FirstSeenFrom     *time.Time
	FirstSeenTo       *time.Time
	LastSeenFrom      *time.Time
	LastSeenTo        *time.Time
	Statuses          []int
	LastValidatedFrom *time.Time
	LastValidatedTo   *time.Time
	Sort              string
	Order             string
}

// GraphEdgeFilter narrows an edge listing. SourceNodeID/TargetNodeID select
// the edges touching one node (used by the navigator); SourceContains and
// TargetContains search the endpoint URLs (used by the column filters).
type GraphEdgeFilter struct {
	Types          []string
	SourceNodeID   string
	TargetNodeID   string
	SourceContains string
	TargetContains string
	Sort           string
	Order          string
}

// FiletypeCount is one bucket of the graph's filetype distribution.
type FiletypeCount struct {
	Filetype string `json:"filetype"`
	Count    int    `json:"count"`
}

// StatusCodeCount is one bucket of the graph's status-code distribution. A
// zero status_code means the node has not been validated yet.
type StatusCodeCount struct {
	StatusCode int `json:"status_code"`
	Count      int `json:"count"`
}

// GraphSummary is the headline of an audit's graph.
type GraphSummary struct {
	TotalNodes    int               `json:"total_nodes"`
	TotalEdges    int               `json:"total_edges"`
	ExternalNodes int               `json:"external_nodes"`
	RootNodes     int               `json:"root_nodes"`
	Filetypes     []FiletypeCount   `json:"filetypes"`
	Statuses      []StatusCodeCount `json:"statuses"`
}
