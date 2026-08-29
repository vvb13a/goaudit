package tui

import (
	"context"
	"time"

	"github.com/vvb13a/goaudit/data"
	"github.com/vvb13a/goaudit/engine"

	tea "github.com/charmbracelet/bubbletea"
)

type ProgressMsg struct {
	CurrentURL string
	Completed  int
	Total      int
}

type InspectionResultMsg struct {
	Inspection *data.Inspection
	Err        error
}

func WaitForProgress(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

func RunPlanWithProgress(
	ch chan tea.Msg,
	runner *engine.Runner,
	planName string,
	rawURLs []string,
	checklistName string,
) tea.Cmd {
	return func() tea.Msg {
		go func() {
			defer close(ch)

			ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
			defer cancel()

			progressCallback := func(currentURL string, completed int, total int) {
				ch <- ProgressMsg{
					CurrentURL: currentURL,
					Completed:  completed,
					Total:      total,
				}
			}

			insp, err := runner.InspectPlan(ctx, planName, rawURLs, checklistName, progressCallback)
			ch <- InspectionResultMsg{
				Inspection: insp,
				Err:        err,
			}
		}()

		return nil
	}
}
