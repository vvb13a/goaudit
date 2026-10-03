package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/vvb13a/goaudit/domain"
	"github.com/vvb13a/goaudit/service"
)

type urlRow struct {
	URL             string                `json:"url"`
	FinalURL        string                `json:"final_url"`
	Title           string                `json:"title"`
	EditURL         string                `json:"edit_url"`
	StatusCode      int                   `json:"status_code"`
	DurationMs      int64                 `json:"duration_ms"`
	State           string                `json:"state"`
	HighestSeverity string                `json:"highest_severity"`
	Score           float64               `json:"score"`
	SeverityCounts  domain.SeverityCounts `json:"severity_counts"`
	CreatedAt       string                `json:"created_at"`
	LastAuditedAt   string                `json:"last_audited_at"`
}

type urlAggregates struct {
	New           int     `json:"new"`
	Active        int     `json:"active"`
	Missing       int     `json:"missing"`
	Total         int     `json:"total"`
	AvgDurationMs float64 `json:"avg_duration_ms"`
	AvgScore      float64 `json:"avg_score"`
}

type urlsResponse struct {
	Items      []urlRow      `json:"items"`
	Total      int           `json:"total"`
	Aggregates urlAggregates `json:"aggregates"`
}

// handleListURLs serves one filtered, sorted and paginated page of an audit's
// audited URLs together with the aggregates of the whole filtered set, which
// back the summary widget (mirroring the TUI urls tab).
func (s *Server) handleListURLs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	q := r.URL.Query()

	filter := domain.URLFilter{
		States:            parseURLStates(q.Get("state")),
		HighestSeverities: parseSeverities(q.Get("highest")),
		URLContains:       strings.TrimSpace(q.Get("url")),
	}
	if n, ok := parseOptionalInt(q.Get("duration_min")); ok {
		filter.DurationMin = time.Duration(n) * time.Millisecond
	}
	if n, ok := parseOptionalInt(q.Get("duration_max")); ok {
		filter.DurationMax = time.Duration(n) * time.Millisecond
	}
	if v, ok := parseOptionalFloat(q.Get("score_min")); ok {
		filter.ScoreMin = v
	}
	if v, ok := parseOptionalFloat(q.Get("score_max")); ok {
		filter.ScoreMax = v
	}

	sortSpec := service.URLSort{
		Field: q.Get("sort"),
		Desc:  isDescending(q.Get("order")),
	}

	limit := clampInt(intParam(q.Get("limit"), 100), 1, 500)
	page := intParam(q.Get("page"), 1)
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit

	agg, err := s.deps.AuditService.URLAggregates(ctx, id, filter)
	if err != nil {
		writeError(w, err)
		return
	}
	rows, total, err := s.deps.AuditService.ListURLsPageSorted(ctx, id, filter, sortSpec, limit, offset)
	if err != nil {
		writeError(w, err)
		return
	}

	items := make([]urlRow, 0, len(rows))
	for _, u := range rows {
		items = append(items, urlRow{
			URL:             u.URL,
			FinalURL:        u.FinalURL,
			Title:           u.Title,
			EditURL:         u.EditURL,
			StatusCode:      u.StatusCode,
			DurationMs:      u.Duration.Milliseconds(),
			State:           string(u.State),
			HighestSeverity: string(u.Summary.HighestSeverity),
			Score:           u.Summary.Score,
			SeverityCounts:  u.Summary.SeverityCounts,
			CreatedAt:       formatTime(u.CreatedAt),
			LastAuditedAt:   formatTime(u.LastAudited),
		})
	}

	writeJSON(w, http.StatusOK, urlsResponse{
		Items: items,
		Total: total,
		Aggregates: urlAggregates{
			New:           agg.URLStates.New,
			Active:        agg.URLStates.Active,
			Missing:       agg.URLStates.Missing,
			Total:         agg.Total(),
			AvgDurationMs: float64(agg.AvgDuration.Milliseconds()),
			AvgScore:      agg.AvgScore,
		},
	})
}

// handleURLIssues serves the stored issues of one audited URL, including the
// evidence the list endpoints omit. The URL is passed as a query parameter
// because URLs contain slashes and other path-significant characters.
func (s *Server) handleURLIssues(w http.ResponseWriter, r *http.Request) {
	url := strings.TrimSpace(r.URL.Query().Get("url"))
	if url == "" {
		writeError(w, invalidConfig("A URL is required"))
		return
	}
	issues, err := s.deps.AuditService.URLIssues(r.Context(), r.PathValue("id"), url)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, issues)
}

func parseURLStates(raw string) []domain.UrlState {
	var out []domain.UrlState
	for _, v := range splitCSV(raw) {
		s := domain.UrlState(v)
		if s.IsValid() {
			out = append(out, s)
		}
	}
	return out
}

func parseOptionalInt(raw string) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

func parseOptionalFloat(raw string) (float64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v < 0 {
		return 0, false
	}
	return v, true
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02T15:04:05Z07:00")
}
