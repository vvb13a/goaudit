// Package api exposes the audit engine over a small JSON HTTP API so
// non-terminal clients (such as the Vue frontend) can read the same data the
// TUI renders. It wraps the existing services and adds no business logic of
// its own.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/vvb13a/goaudit/domain"
	"github.com/vvb13a/goaudit/service"
)

// Deps carries the services the HTTP handlers operate on.
type Deps struct {
	AuditService  *service.AuditService
	Registry      *service.CheckRegistry
	ConfigManager *service.Manager
	Runner        *service.Runner
	Notifier      *service.Notifier
}

// Server is the HTTP handler for the JSON API.
type Server struct {
	deps Deps
	mux  *http.ServeMux
	runs *runManager
}

// NewServer builds the API handler with every route registered.
func NewServer(deps Deps) *Server {
	s := &Server{
		deps: deps,
		mux:  http.NewServeMux(),
		runs: newRunManager(),
	}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/audits", s.handleListAudits)
	s.mux.HandleFunc("POST /api/audits", s.handleCreateAudit)
	s.mux.HandleFunc("GET /api/audits/{id}", s.handleGetAudit)
	s.mux.HandleFunc("DELETE /api/audits/{id}", s.handleDeleteAudit)
	s.mux.HandleFunc("POST /api/audits/{id}/duplicate", s.handleDuplicateAudit)
	s.mux.HandleFunc("POST /api/audits/{id}/reset", s.handleResetAudit)
	s.mux.HandleFunc("POST /api/audits/{id}/run", s.handleStartRun)
	s.mux.HandleFunc("GET /api/audits/{id}/run", s.handleRunStatus)
	s.mux.HandleFunc("POST /api/audits/{id}/recheck", s.handleRecheck)
	s.mux.HandleFunc("GET /api/audits/{id}/dashboard", s.handleDashboard)
	s.mux.HandleFunc("GET /api/audits/{id}/snapshots", s.handleListSnapshots)
	s.mux.HandleFunc("GET /api/audits/{id}/urls", s.handleListURLs)
	s.mux.HandleFunc("GET /api/audits/{id}/url-issues", s.handleURLIssues)
	s.mux.HandleFunc("GET /api/audits/{id}/issues", s.handleListIssues)
	s.mux.HandleFunc("GET /api/audits/{id}/issues/{issueId}", s.handleGetIssue)
	s.mux.HandleFunc("GET /api/audits/{id}/issue-filters", s.handleIssueFilters)
	s.mux.HandleFunc("GET /api/audits/{id}/checks", s.handleAuditChecks)
	s.mux.HandleFunc("GET /api/checks", s.handleChecks)
	s.mux.HandleFunc("GET /api/config", s.handleConfig)
	s.mux.HandleFunc("PUT /api/config", s.handleUpdateConfig)
	s.mux.HandleFunc("GET /api/audits/{id}/config", s.handleGetAuditConfig)
	s.mux.HandleFunc("PUT /api/audits/{id}/config", s.handleUpdateAuditConfig)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleListAudits(w http.ResponseWriter, r *http.Request) {
	audits, err := s.deps.AuditService.List(r.Context(), domain.AuditFilter{})
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]auditSummary, 0, len(audits))
	for _, a := range audits {
		out = append(out, toAuditSummary(a))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetAudit(w http.ResponseWriter, r *http.Request) {
	audit, err := s.deps.AuditService.GetConfig(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toAuditSummary(audit))
}

// handleCreateAudit persists a new audit from the submitted configuration
// without running it. The first run happens later through the run endpoint.
func (s *Server) handleCreateAudit(w http.ResponseWriter, r *http.Request) {
	var req auditConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := validateConfig(req.Config); err != nil {
		writeError(w, err)
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, invalidConfig("Audit name is required"))
		return
	}
	if len(req.Targets) == 0 {
		writeError(w, invalidConfig("Add at least one target URL"))
		return
	}
	if req.Config.EnableChecks && len(req.CheckNames) == 0 {
		writeError(w, invalidConfig("Select at least one check"))
		return
	}
	raw, err := json.Marshal(req.Config)
	if err != nil {
		writeError(w, err)
		return
	}
	audit, err := s.deps.AuditService.CreateBlank(r.Context(), req.Name, req.Description, req.Targets, req.CheckNames, raw)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toAuditSummary(audit))
}

// handleRecheck re-audits the configured target behind one page URL and
// persists the fresh result in place without recording a run snapshot. It is
// the per-issue "rerun" action.
func (s *Server) handleRecheck(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req recheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if strings.TrimSpace(req.URL) == "" {
		writeError(w, invalidConfig("A page URL is required"))
		return
	}
	ctx := r.Context()

	audit, err := s.deps.AuditService.GetConfig(ctx, id)
	if err != nil {
		writeError(w, err)
		return
	}
	target, err := s.deps.AuditService.ResolveRecheckTarget(ctx, id, req.URL)
	if err != nil {
		writeError(w, err)
		return
	}
	checks, err := s.deps.Registry.Resolve(audit.CheckNames)
	if err != nil {
		writeError(w, err)
		return
	}
	cfg := s.effectiveConfig(audit.Config)
	fresh, err := s.deps.Runner.AuditURL(ctx, target, checks, cfg)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.deps.AuditService.ApplyURLRecheck(ctx, id, fresh); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "url": target})
}

func (s *Server) handleDeleteAudit(w http.ResponseWriter, r *http.Request) {
	if err := s.deps.AuditService.Delete(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	s.runs.forget(r.PathValue("id"))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleResetAudit(w http.ResponseWriter, r *http.Request) {
	if err := s.deps.AuditService.ResetData(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	s.runs.forget(r.PathValue("id"))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDuplicateAudit(w http.ResponseWriter, r *http.Request) {
	audit, err := s.deps.AuditService.Duplicate(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toAuditSummary(audit))
}

// handleDashboard mirrors the TUI dashboard load: the audit's run record, its
// aggregated metrics, the number of checks that ran and the previous run
// snapshot used for run-over-run deltas.
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")

	audit, err := s.deps.AuditService.GetConfig(ctx, id)
	if err != nil {
		writeError(w, err)
		return
	}
	metrics, err := s.deps.AuditService.DashboardMetrics(ctx, id)
	if err != nil {
		writeError(w, err)
		return
	}

	checks := len(audit.CheckNames)
	if checks == 0 {
		if n, err := s.deps.AuditService.DistinctIssueChecks(ctx, id); err == nil {
			checks = n
		}
	}

	var previous *domain.AuditSnapshot
	if snapshots, err := s.deps.AuditService.ListSnapshots(ctx, id, 2); err == nil && len(snapshots) > 1 {
		previous = snapshots[1]
	}

	writeJSON(w, http.StatusOK, dashboardResponse{
		Audit:    toAuditSummary(audit),
		Metrics:  metrics,
		Checks:   checks,
		Previous: previous,
	})
}

func (s *Server) handleListSnapshots(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := parsePositiveInt(raw); err == nil {
			limit = n
		}
	}
	snapshots, err := s.deps.AuditService.ListSnapshots(r.Context(), r.PathValue("id"), limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, snapshots)
}

type checkInfo struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

func (s *Server) handleChecks(w http.ResponseWriter, _ *http.Request) {
	all := s.deps.Registry.All()
	out := make([]checkInfo, 0, len(all))
	for _, c := range all {
		info := c.Info()
		out = append(out, checkInfo{
			Name:        info.Name,
			Label:       info.Label,
			Description: info.Description,
			Category:    string(info.Category),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleConfig(w http.ResponseWriter, _ *http.Request) {
	if s.deps.ConfigManager == nil {
		writeJSON(w, http.StatusOK, service.Config{})
		return
	}
	writeJSON(w, http.StatusOK, s.deps.ConfigManager.Get())
}

// handleUpdateConfig persists the app-level configuration (engine defaults and
// the notification settings) and returns the stored value.
func (s *Server) handleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	if s.deps.ConfigManager == nil {
		writeError(w, errors.New("config manager unavailable"))
		return
	}
	var cfg service.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := s.deps.ConfigManager.Update(cfg); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.deps.ConfigManager.Get())
}

// handleGetAuditConfig returns the editable configuration of an audit: its
// name, description, targets, selected checks and the effective engine config
// (the audit's stored config merged over the app-level fallback).
func (s *Server) handleGetAuditConfig(w http.ResponseWriter, r *http.Request) {
	audit, err := s.deps.AuditService.GetConfig(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.auditConfig(audit))
}

// handleUpdateAuditConfig validates the submitted configuration and persists
// it on the audit record. Invalid engine values are rejected instead of being
// silently merged to their defaults, mirroring the TUI editor.
func (s *Server) handleUpdateAuditConfig(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req auditConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := validateConfig(req.Config); err != nil {
		writeError(w, err)
		return
	}
	raw, err := json.Marshal(req.Config)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.deps.AuditService.UpdateConfig(r.Context(), id, req.Name, req.Description, req.Targets, req.CheckNames, raw); err != nil {
		writeError(w, err)
		return
	}
	audit, err := s.deps.AuditService.GetConfig(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.auditConfig(audit))
}

func (s *Server) effectiveConfig(raw json.RawMessage) service.Config {
	base := *service.DefaultConfig()
	if s.deps.ConfigManager != nil {
		base = s.deps.ConfigManager.Get()
	}
	return service.MergeConfig(base, raw)
}

func (s *Server) auditConfig(audit *domain.Audit) auditConfigResponse {
	targets := audit.Targets
	if targets == nil {
		targets = []string{}
	}
	checkNames := audit.CheckNames
	if checkNames == nil {
		checkNames = []string{}
	}
	return auditConfigResponse{
		Name:        audit.Name,
		Description: audit.Description,
		Targets:     targets,
		CheckNames:  checkNames,
		Config:      s.effectiveConfig(audit.Config),
	}
}

// validateConfig rejects engine values the runner cannot honor, matching the
// constraints enforced by the TUI config editor.
func validateConfig(c service.Config) error {
	switch {
	case c.MaxConcurrency <= 0:
		return invalidConfig("Max concurrency must be a positive integer")
	case c.RequestDelayMs < 0:
		return invalidConfig("Request delay (ms) must be an integer >= 0")
	case c.HTTPTimeoutSec <= 0:
		return invalidConfig("HTTP timeout (s) must be a positive integer")
	case c.MaxSitemapDepth <= 0:
		return invalidConfig("Max sitemap depth must be a positive integer")
	case c.LinkCacheTTLMin <= 0:
		return invalidConfig("Link cache TTL (min) must be a positive integer")
	case strings.TrimSpace(c.UserAgent) == "":
		return invalidConfig("User agent must not be empty")
	case !c.EnableChecks && !c.EnableGraph:
		return invalidConfig("Enable checks, the link graph, or both")
	}
	if c.NotificationsEnabled {
		webhook := strings.TrimSpace(c.SlackWebhookURL)
		if webhook == "" {
			return invalidConfig("A Slack webhook URL is required to enable notifications")
		}
		if !strings.HasPrefix(webhook, "http://") && !strings.HasPrefix(webhook, "https://") {
			return invalidConfig("The Slack webhook URL must start with http:// or https://")
		}
	}
	return nil
}

func invalidConfig(message string) error {
	return fmt.Errorf("%w: %s", domain.ErrInvalidAudit, message)
}

// ---- DTOs ----

// auditSummary is the API shape of an audit record. It keeps the run
// configuration and derived totals and converts the duration to milliseconds
// so clients do not have to know Go's nanosecond representation.
type auditSummary struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Targets     []string `json:"targets"`
	CheckNames  []string `json:"check_names"`
	StartedAt   string   `json:"started_at"`
	DurationMs  int64    `json:"duration_ms"`
	Score       float64  `json:"score"`
}

func toAuditSummary(a *domain.Audit) auditSummary {
	targets := a.Targets
	if targets == nil {
		targets = []string{}
	}
	checkNames := a.CheckNames
	if checkNames == nil {
		checkNames = []string{}
	}
	return auditSummary{
		ID:          a.ID,
		Name:        a.Name,
		Description: a.Description,
		Targets:     targets,
		CheckNames:  checkNames,
		StartedAt:   a.StartedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		DurationMs:  a.Duration.Milliseconds(),
		Score:       a.Score,
	}
}

type dashboardResponse struct {
	Audit    auditSummary          `json:"audit"`
	Metrics  *domain.AuditMetrics  `json:"metrics"`
	Checks   int                   `json:"checks"`
	Previous *domain.AuditSnapshot `json:"previous"`
}

type auditConfigResponse struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Targets     []string       `json:"targets"`
	CheckNames  []string       `json:"check_names"`
	Config      service.Config `json:"config"`
}

type auditConfigRequest struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Targets     []string       `json:"targets"`
	CheckNames  []string       `json:"check_names"`
	Config      service.Config `json:"config"`
}

type recheckRequest struct {
	URL string `json:"url"`
}

// ---- Helpers ----

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, domain.ErrAuditNotFound) || errors.Is(err, domain.ErrIssueNotFound) {
		status = http.StatusNotFound
	} else if errors.Is(err, domain.ErrInvalidAudit) {
		status = http.StatusBadRequest
	} else if errors.Is(err, domain.ErrAuditInProgress) {
		status = http.StatusConflict
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func parsePositiveInt(raw string) (int, error) {
	n := 0
	for _, r := range raw {
		if r < '0' || r > '9' {
			return 0, errors.New("not a number")
		}
		n = n*10 + int(r-'0')
	}
	if n <= 0 {
		return 0, errors.New("not positive")
	}
	return n, nil
}
