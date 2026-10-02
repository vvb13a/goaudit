package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/vvb13a/goaudit/domain"
	"github.com/vvb13a/goaudit/service"
)

// runRequest carries optional one-time overrides for a run. Config, when set,
// replaces the audit's effective engine configuration for this run only;
// Notify overrides the notify-on-direct decision for this run only.
type runRequest struct {
	Config *service.Config `json:"config,omitempty"`
	Notify *bool           `json:"notify,omitempty"`
}

type runState string

const (
	runIdle    runState = "idle"
	runRunning runState = "running"
	runDone    runState = "done"
	runError   runState = "error"
)

// runStatus is the polled state of an audit's most recent background run.
type runStatus struct {
	State      runState  `json:"state"`
	CurrentURL string    `json:"current_url,omitempty"`
	Completed  int       `json:"completed"`
	Total      int       `json:"total"`
	Error      string    `json:"error,omitempty"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
}

// runManager tracks the in-flight and last-completed run of each audit. It is
// in-memory: a server restart clears the statuses, which reads as "idle".
type runManager struct {
	mu   sync.Mutex
	runs map[string]*runStatus
}

func newRunManager() *runManager {
	return &runManager{runs: make(map[string]*runStatus)}
}

func (rm *runManager) status(id string) runStatus {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if s, ok := rm.runs[id]; ok {
		return *s
	}
	return runStatus{State: runIdle}
}

// start registers a running status unless the audit already has a run in
// flight, in which case the existing status is returned.
func (rm *runManager) start(id string) (runStatus, bool) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if s, ok := rm.runs[id]; ok && s.State == runRunning {
		return *s, false
	}
	s := &runStatus{State: runRunning, StartedAt: time.Now().UTC()}
	rm.runs[id] = s
	return *s, true
}

func (rm *runManager) update(id string, fn func(*runStatus)) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if s, ok := rm.runs[id]; ok {
		fn(s)
	}
}

func (rm *runManager) finish(id string) {
	rm.update(id, func(s *runStatus) {
		s.State = runDone
		s.CurrentURL = ""
		s.FinishedAt = time.Now().UTC()
	})
}

func (rm *runManager) fail(id string, err error) {
	rm.update(id, func(s *runStatus) {
		s.State = runError
		s.Error = err.Error()
		s.CurrentURL = ""
		s.FinishedAt = time.Now().UTC()
	})
}

// forget drops the status of an audit whose data or record was removed.
func (rm *runManager) forget(id string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	delete(rm.runs, id)
}

// handleStartRun launches a full audit in the background and returns the
// initial running status. A second request while a run is in flight is
// rejected with 409.
func (s *Server) handleStartRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	audit, err := s.deps.AuditService.GetConfig(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}

	// The body is optional: one-time overrides for this run (engine settings
	// and the notify decision) without persisting them.
	var req runRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Config != nil {
		if err := validateConfig(*req.Config); err != nil {
			writeError(w, err)
			return
		}
	}

	status, started := s.runs.start(id)
	if !started {
		writeError(w, domain.ErrAuditInProgress)
		return
	}
	go s.executeRun(id, audit, req)
	writeJSON(w, http.StatusAccepted, status)
}

func (s *Server) handleRunStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.runs.status(r.PathValue("id")))
}

// executeRun performs the audit and replaces the audit's stored run data with
// the fresh results. It owns the background goroutine and never touches the
// request context.
func (s *Server) executeRun(id string, audit *domain.Audit, req runRequest) {
	ctx := context.Background()

	cfg := s.effectiveConfig(audit.Config)
	// One-time overrides supplied with the run request.
	if req.Config != nil {
		cfg = *req.Config
	}
	if req.Notify != nil {
		cfg.NotifyOnDirect = *req.Notify
	}

	// Checks are only resolved when the workflow is enabled; a graph-only run
	// must run without checks even if stale names are stored on the audit.
	var checks []domain.Check
	if cfg.EnableChecks {
		resolved, err := s.deps.Registry.Resolve(audit.CheckNames)
		if err != nil {
			s.runs.fail(id, err)
			return
		}
		checks = resolved
	}

	result, err := s.deps.Runner.ExecuteAudit(
		ctx,
		audit.Name,
		audit.Description,
		audit.Targets,
		checks,
		cfg,
		func(url string, completed, total int) {
			s.runs.update(id, func(st *runStatus) {
				st.CurrentURL = url
				st.Completed = completed
				st.Total = total
			})
		},
	)
	if err != nil {
		s.runs.fail(id, err)
		return
	}

	result.ID = id
	// A run (including one-time overrides) must not rewrite the audit's stored
	// configuration; only the run data is replaced.
	result.Config = audit.Config
	if err := s.deps.AuditService.ReplaceRun(ctx, result); err != nil {
		s.runs.fail(id, err)
		return
	}
	s.runs.finish(id)

	// Best effort: a failed notification must not affect the run.
	if s.deps.Notifier != nil {
		if err := s.deps.Notifier.Notify(ctx, result, cfg, service.NotifyDirect); err != nil {
			log.Printf("notify audit %s: %v", id, err)
		}
	}
}
