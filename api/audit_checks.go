package api

import (
	"net/http"
	"strings"

	"github.com/vvb13a/goaudit/domain"
	"github.com/vvb13a/goaudit/service"
)

// checkRow is a check summary enriched with the registry's human-readable
// label and description. The metadata lives in code, not the database.
type checkRow struct {
	Name        string                `json:"name"`
	Label       string                `json:"label"`
	Description string                `json:"description"`
	Category    domain.Category       `json:"category"`
	Total       int                   `json:"total"`
	Severity    domain.SeverityCounts `json:"severity"`
}

type checksResponse struct {
	Items         []checkRow            `json:"items"`
	Total         int                   `json:"total"`
	HighestCounts domain.SeverityCounts `json:"highest_counts"`
}

// handleAuditChecks serves one filtered, sorted and paginated page of the
// audit's per-check summaries, plus the totals of the whole filtered set that
// back the checks widget. It backs the checks tab.
func (s *Server) handleAuditChecks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	q := r.URL.Query()

	filter := service.CheckFilter{
		NameContains: strings.TrimSpace(q.Get("name")),
		Categories:   parseCategories(q.Get("category")),
		Highest:      parseSeverities(q.Get("highest")),
	}
	if n, ok := parseOptionalInt(q.Get("fatal_min")); ok {
		filter.FatalMin = n
	}
	if n, ok := parseOptionalInt(q.Get("error_min")); ok {
		filter.ErrorMin = n
	}
	if n, ok := parseOptionalInt(q.Get("warning_min")); ok {
		filter.WarningMin = n
	}
	if n, ok := parseOptionalInt(q.Get("notice_min")); ok {
		filter.NoticeMin = n
	}
	if n, ok := parseOptionalInt(q.Get("success_min")); ok {
		filter.SuccessMin = n
	}

	sortSpec := service.CheckSort{
		Field: q.Get("sort"),
		Desc:  isDescending(q.Get("order")),
	}

	limit := clampInt(intParam(q.Get("limit"), 25), 1, 200)
	pageNum := intParam(q.Get("page"), 1)
	if pageNum < 1 {
		pageNum = 1
	}
	offset := (pageNum - 1) * limit

	result, err := s.deps.AuditService.ListCheckSummariesPage(ctx, id, filter, sortSpec, limit, offset)
	if err != nil {
		writeError(w, err)
		return
	}

	rows := make([]checkRow, 0, len(result.Items))
	for _, c := range result.Items {
		row := checkRow{
			Name:     c.Name,
			Label:    c.Name,
			Category: c.Category,
			Total:    c.Total,
			Severity: c.Severity,
		}
		if chk, ok := s.deps.Registry.Get(c.Name); ok {
			info := chk.Info()
			if info.Label != "" {
				row.Label = info.Label
			}
			row.Description = info.Description
		}
		rows = append(rows, row)
	}

	writeJSON(w, http.StatusOK, checksResponse{
		Items:         rows,
		Total:         result.Total,
		HighestCounts: result.HighestCounts,
	})
}
