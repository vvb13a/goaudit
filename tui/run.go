package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vvb13a/goaudit/domain"
)

// newRunCmd executes an audit through the runner and persists the resulting
// audit. Progress events are pushed onto prog; the done channel is closed
// when the run finishes, so progress waiters can stop.
func newRunCmd(
	deps Deps,
	name string,
	targets []string,
	checks []domain.Check,
	target ViewID,
	prog chan<- ProgressMsg,
	done chan struct{},
) tea.Cmd {
	return func() tea.Msg {
		defer close(done)

		audit, err := deps.Runner.ExecuteAudit(
			context.Background(),
			name,
			targets,
			checks,
			func(url string, completed, total int) {
				prog <- ProgressMsg{CurrentURL: url, Completed: completed, Total: total}
			},
		)
		if err != nil {
			return runCompleteMsg{target: target, title: name, err: err}
		}
		if err := deps.AuditService.Create(context.Background(), audit); err != nil {
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
