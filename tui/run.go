package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vvb13a/goaudit/domain"
)

// prepareRun fetches the currently active checklist and resolves its checks
// so an audit run can be started.
func prepareRun(deps Deps) (*domain.Checklist, []domain.Check, error) {
	checklist, err := deps.ChecklistService.GetActive(context.Background())
	if err != nil {
		return nil, nil, fmt.Errorf("fetch active checklist: %w", err)
	}
	checks, err := deps.Registry.Resolve(checklist.CheckNames)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve checklist checks: %w", err)
	}
	return checklist, checks, nil
}

// newRunCmd executes an audit plan through the runner and persists the
// resulting audit. Progress events are pushed onto prog; the done channel is
// closed when the run finishes, so progress waiters can stop.
func newRunCmd(
	deps Deps,
	plan *domain.Plan,
	checklist *domain.Checklist,
	checks []domain.Check,
	owner, target ViewID,
	prog chan<- ProgressMsg,
	done chan struct{},
) tea.Cmd {
	return func() tea.Msg {
		defer close(done)

		audit, err := deps.Runner.ExecutePlan(
			context.Background(),
			plan,
			checklist,
			checks,
			func(url string, completed, total int) {
				prog <- ProgressMsg{CurrentURL: url, Completed: completed, Total: total}
			},
		)
		if err != nil {
			return runCompleteMsg{owner: owner, target: target, title: plan.Name, err: err}
		}
		if err := deps.AuditService.Create(context.Background(), audit); err != nil {
			return runCompleteMsg{owner: owner, target: target, audit: audit, title: plan.Name, err: err}
		}
		return runCompleteMsg{owner: owner, target: target, audit: audit, title: plan.Name}
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
