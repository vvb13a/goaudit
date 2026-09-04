package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/vvb13a/goaudit/domain"
)

// ProgressMsg reports one step of an ongoing audit run.
type ProgressMsg struct {
	CurrentURL string
	Completed  int
	Total      int
}

// runCompleteMsg is emitted when an audit run finishes. It is handled by the
// root model, which restores the initiating view, refreshes the audits list
// and navigates to the requested tab.
type runCompleteMsg struct {
	target ViewID
	audit  *domain.Audit
	title  string
	err    error
}

// ProgressModel is a standalone model rendering the state of an audit run:
// the audit being executed, the URL currently being audited, a progress bar
// and completion counts.
type ProgressModel struct {
	bar        progress.Model
	title      string
	currentURL string
	completed  int
	total      int
}

func NewProgressModel() ProgressModel {
	return ProgressModel{}
}

// Start resets the model for a new run with the given title, sizing the bar
// to the available width.
func (m ProgressModel) Start(title string, width int) ProgressModel {
	w := width - 10
	if w < 20 {
		w = 20
	}
	if w > 80 {
		w = 80
	}
	return ProgressModel{
		bar:   progress.New(progress.WithDefaultGradient(), progress.WithWidth(w)),
		title: title,
	}
}

func (m ProgressModel) Update(msg tea.Msg) (ProgressModel, tea.Cmd) {
	switch msg := msg.(type) {
	case ProgressMsg:
		m.currentURL = msg.CurrentURL
		m.completed = msg.Completed
		m.total = msg.Total

		percent := 0.0
		if msg.Total > 0 {
			percent = float64(msg.Completed) / float64(msg.Total)
			if percent > 1 {
				percent = 1
			}
		}
		return m, m.bar.SetPercent(percent)

	case progress.FrameMsg:
		updated, cmd := m.bar.Update(msg)
		if bar, ok := updated.(progress.Model); ok {
			m.bar = bar
		}
		return m, cmd
	}
	return m, nil
}

func (m ProgressModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render(m.title))
	b.WriteString("\n\n")

	b.WriteString(fmt.Sprintf("Auditing: %s", m.currentURL))
	b.WriteString("\n\n")

	b.WriteString(m.bar.View())
	b.WriteString("\n\n")

	if m.total > 0 {
		b.WriteString(helpStyle.Render(fmt.Sprintf("Completed %d of %d endpoints", m.completed, m.total)))
	} else {
		b.WriteString(helpStyle.Render("Resolving targets..."))
	}
	return b.String()
}
