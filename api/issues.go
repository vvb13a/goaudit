package api

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/vvb13a/goaudit/domain"
	"github.com/vvb13a/goaudit/service"
)

type issuesResponse struct {
	Items    []*domain.Issue       `json:"items"`
	Total    int                   `json:"total"`
	Severity domain.SeverityCounts `json:"severity_counts"`
}

// handleListIssues serves one filtered, sorted and paginated page of an
// audit's stored issues. Every dimension of the query is optional; the
// zero-value query returns the first page of all issues.
func (s *Server) handleListIssues(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	q := r.URL.Query()

	filter := domain.IssueFilter{
		Severities:      parseSeverities(q.Get("severity")),
		Lifecycles:      parseLifecycles(q.Get("lifecycle")),
		CheckNames:      splitCSV(q.Get("check")),
		Categories:      parseCategories(q.Get("category")),
		URLContains:     strings.TrimSpace(q.Get("url")),
		MessageContains: strings.TrimSpace(q.Get("message")),
	}

	sortSpec := service.IssueSort{
		Field: q.Get("sort"),
		Desc:  isDescending(q.Get("order")),
	}

	limit := clampInt(intParam(q.Get("limit"), 25), 1, 200)
	page := intParam(q.Get("page"), 1)
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit

	// The severity distribution doubles as the row count: its total is the
	// number of rows this filter matches, and it backs the severity widget
	// above the table (mirroring the TUI issues view).
	counts, err := s.deps.AuditService.IssueDistribution(ctx, id, filter)
	if err != nil {
		writeError(w, err)
		return
	}

	items, err := s.deps.AuditService.ListIssuesPageBrief(ctx, id, filter, sortSpec, limit, offset)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, issuesResponse{
		Items:    items,
		Total:    counts.Total(),
		Severity: counts,
	})
}

// handleGetIssue serves a single issue, including the findings (evidence)
// that the list endpoint omits. The client calls it when a row is opened.
func (s *Server) handleGetIssue(w http.ResponseWriter, r *http.Request) {
	issue, err := s.deps.AuditService.GetIssue(
		r.Context(),
		r.PathValue("id"),
		r.PathValue("issueId"),
	)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, issue)
}

type issueFiltersResponse struct {
	CheckNames []string `json:"check_names"`
}

// handleIssueFilters returns the check names observed in the audit's stored
// issues, which the client uses as the options of its check filter.
func (s *Server) handleIssueFilters(w http.ResponseWriter, r *http.Request) {
	names, err := s.deps.AuditService.IssueCheckNames(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	if names == nil {
		names = []string{}
	}
	writeJSON(w, http.StatusOK, issueFiltersResponse{CheckNames: names})
}

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

func parseSeverities(raw string) []domain.Severity {
	var out []domain.Severity
	for _, v := range splitCSV(raw) {
		if sev, err := domain.ParseSeverity(v); err == nil {
			out = append(out, sev)
		}
	}
	return out
}

func parseLifecycles(raw string) []domain.IssueLifecycle {
	valid := make(map[string]struct{}, len(domain.AllLifecycles))
	for _, lc := range domain.AllLifecycles {
		valid[string(lc)] = struct{}{}
	}
	var out []domain.IssueLifecycle
	for _, v := range splitCSV(raw) {
		if _, ok := valid[v]; ok {
			out = append(out, domain.IssueLifecycle(v))
		}
	}
	return out
}

func parseCategories(raw string) []domain.Category {
	var out []domain.Category
	for _, v := range splitCSV(raw) {
		if cat, err := domain.ParseCategory(v); err == nil {
			out = append(out, cat)
		}
	}
	return out
}

func isDescending(order string) bool {
	return strings.EqualFold(order, "desc") || order == "-1"
}

func intParam(raw string, def int) int {
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return n
}

func clampInt(n, min, max int) int {
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}
