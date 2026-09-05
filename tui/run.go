package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vvb13a/goaudit/domain"
	"github.com/vvb13a/goaudit/service"
)

// newRunCmd executes an audit through the runner and persists the resulting
// audit. cfg is the effective engine configuration of the run (already
// resolved from the audit's stored config over the app defaults). When
// replaceID is non-empty the run is a rerun: the results replace the issue
// and report rows of that existing audit instead of creating a new one.
// Progress events are pushed onto prog; the done channel is closed when the
// run finishes, so progress waiters can stop.
func newRunCmd(
	deps Deps,
	name string,
	description string,
	targets []string,
	checks []domain.Check,
	cfg service.Config,
	replaceID string,
	target ViewID,
	prog chan<- ProgressMsg,
	done chan struct{},
) tea.Cmd {
	return func() tea.Msg {
		defer close(done)

		audit, err := deps.Runner.ExecuteAudit(
			context.Background(),
			name,
			description,
			targets,
			checks,
			cfg,
			func(url string, completed, total int) {
				prog <- ProgressMsg{CurrentURL: url, Completed: completed, Total: total}
			},
		)
		if err != nil {
			return runCompleteMsg{target: target, title: name, err: err}
		}

		if replaceID != "" {
			audit.ID = replaceID
			err = deps.AuditService.ReplaceRun(context.Background(), audit)
		} else {
			err = deps.AuditService.Create(context.Background(), audit)
		}
		if err != nil {
			return runCompleteMsg{target: target, audit: audit, title: name, err: err}
		}
		return runCompleteMsg{target: target, audit: audit, title: name}
	}
}

// newProgressWaitCmd blocks until the next progress event or until the run
// signals completion.
func newProgressWaitCmd(prog <-chan ProgressMsg, done <-chan struct{}) tea.Cmd {
	return func() tea.Msg {
		select {
		case p, ok := <-prog:
			if !ok {
				return nil
			}
			return p
		case <-done:
			return nil
		}
	}
}

// urlRecheckedMsg reports the outcome of rechecking a single audited URL of
// the current audit.
type urlRecheckedMsg struct {
	auditID string
	url     string
	err     error
}

// recheckURLCmd re-runs the audit's checks against one audited URL and
// persists the result in place. Unlike a full run, a recheck never records
// an audit snapshot, so the timeline stays untouched.
func recheckURLCmd(deps Deps, auditID, targetURL string) tea.Cmd {
	return func() tea.Msg {
		audit, err := deps.AuditService.GetConfig(context.Background(), auditID)
		if err != nil {
			return urlRecheckedMsg{auditID: auditID, url: targetURL, err: err}
		}
		checks, err := deps.Registry.Resolve(audit.CheckNames)
		if err != nil {
			return urlRecheckedMsg{auditID: auditID, url: targetURL, err: err}
		}
		cfg := effectiveConfig(deps, audit.Config)

		u, err := deps.Runner.AuditURL(context.Background(), targetURL, checks, cfg)
		if err != nil {
			return urlRecheckedMsg{auditID: auditID, url: targetURL, err: err}
		}
		err = deps.AuditService.ApplyURLRecheck(context.Background(), auditID, u)
		return urlRecheckedMsg{auditID: auditID, url: targetURL, err: err}
	}
}
