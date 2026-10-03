package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/vvb13a/goaudit/domain"
)

type graphNodesResponse struct {
	Items []*domain.GraphNode `json:"items"`
	Total int                 `json:"total"`
}

type graphEdgesResponse struct {
	Items []*domain.GraphEdgeRow `json:"items"`
	Total int                    `json:"total"`
}

// graphCap bounds how many nodes and edges the whole-graph endpoint returns to
// a visualization client.
const graphCap = 2000

type graphResponse struct {
	Nodes     []*domain.GraphNode    `json:"nodes"`
	Edges     []*domain.GraphEdgeRow `json:"edges"`
	Truncated bool                   `json:"truncated"`
}

// handleGraphAll serves the whole graph (nodes and denormalized edges) in one
// response for the visualization, capped at graphCap of each. The node list is
// ordered by out-degree so the most connected nodes survive the cap.
func (s *Server) handleGraphAll(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	nodes, nodeTotal, err := s.deps.AuditService.ListGraphNodes(
		r.Context(), id, domain.GraphNodeFilter{Sort: "out_links", Order: "desc"}, graphCap, 0,
	)
	if err != nil {
		writeError(w, err)
		return
	}
	edges, edgeTotal, err := s.deps.AuditService.ListGraphEdges(
		r.Context(), id, domain.GraphEdgeFilter{}, graphCap, 0,
	)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, graphResponse{
		Nodes:     nodes,
		Edges:     edges,
		Truncated: nodeTotal > len(nodes) || edgeTotal > len(edges),
	})
}

// handleGraphNodes serves one filtered, sorted and paginated page of an
// audit's graph nodes.
func (s *Server) handleGraphNodes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := domain.GraphNodeFilter{
		Filetypes: splitCSV(q.Get("filetype")),
		Statuses:  parseIntsCSV(q.Get("status")),
		Search:    strings.TrimSpace(q.Get("url")),
		Sort:      q.Get("sort"),
		Order:     q.Get("order"),
	}
	if v, ok := parseOptionalBool(q.Get("external")); ok {
		filter.External = &v
	}
	if v, ok := parseOptionalBool(q.Get("root")); ok {
		filter.IsRoot = &v
	}
	filter.FirstSeenFrom = parseOptionalTime(q.Get("first_seen_min"))
	filter.FirstSeenTo = parseOptionalTime(q.Get("first_seen_max"))
	filter.LastSeenFrom = parseOptionalTime(q.Get("last_seen_min"))
	filter.LastSeenTo = parseOptionalTime(q.Get("last_seen_max"))
	filter.LastValidatedFrom = parseOptionalTime(q.Get("last_validated_min"))
	filter.LastValidatedTo = parseOptionalTime(q.Get("last_validated_max"))

	limit := clampInt(intParam(q.Get("limit"), 100), 1, 500)
	page := intParam(q.Get("page"), 1)
	if page < 1 {
		page = 1
	}

	items, total, err := s.deps.AuditService.ListGraphNodes(r.Context(), r.PathValue("id"), filter, limit, (page-1)*limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, graphNodesResponse{Items: items, Total: total})
}

// handleGraphEdges serves one filtered, sorted and paginated page of an
// audit's graph edges. source_id/target_id select the edges touching one node
// (navigator); source/target search the endpoint URLs (column filters).
func (s *Server) handleGraphEdges(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := domain.GraphEdgeFilter{
		Types:          splitCSV(q.Get("type")),
		Containers:     splitCSV(q.Get("container")),
		Roles:          splitCSV(q.Get("role")),
		SourceNodeID:   strings.TrimSpace(q.Get("source_id")),
		TargetNodeID:   strings.TrimSpace(q.Get("target_id")),
		SourceContains: strings.TrimSpace(q.Get("source")),
		TargetContains: strings.TrimSpace(q.Get("target")),
		Sort:           q.Get("sort"),
		Order:          q.Get("order"),
	}

	limit := clampInt(intParam(q.Get("limit"), 100), 1, 500)
	page := intParam(q.Get("page"), 1)
	if page < 1 {
		page = 1
	}

	items, total, err := s.deps.AuditService.ListGraphEdges(r.Context(), r.PathValue("id"), filter, limit, (page-1)*limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, graphEdgesResponse{Items: items, Total: total})
}

// handleGraphNode serves one graph node by id.
func (s *Server) handleGraphNode(w http.ResponseWriter, r *http.Request) {
	nodeID := strings.TrimSpace(r.URL.Query().Get("node"))
	if nodeID == "" {
		writeError(w, invalidConfig("A node id is required"))
		return
	}
	node, err := s.deps.AuditService.GetGraphNode(r.Context(), r.PathValue("id"), nodeID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, node)
}

// handleGraphSummary serves the graph headline totals and filetype
// distribution.
func (s *Server) handleGraphSummary(w http.ResponseWriter, r *http.Request) {
	summary, err := s.deps.AuditService.GraphSummary(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

// parseOptionalBool parses a tri-state boolean query value: absent or
// unrecognized returns ok=false so the filter stays unset.
func parseOptionalBool(raw string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "1":
		return true, true
	case "false", "0":
		return false, true
	default:
		return false, false
	}
}

// parseIntsCSV parses a comma-separated list of integers, dropping any part
// that is not a valid integer.
func parseIntsCSV(raw string) []int {
	var out []int
	for _, part := range splitCSV(raw) {
		if n, err := strconv.Atoi(part); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// parseOptionalTime parses an RFC3339 timestamp query value, returning nil when
// absent or unparsable so the bound stays unset.
func parseOptionalTime(raw string) *time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil
	}
	return &t
}
