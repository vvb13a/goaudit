package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/vvb13a/goaudit/domain"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type auditsLoadedMsg struct {
	audits []*domain.Audit
	err    error
}

type auditsState int

const (
	auditsListState auditsState = iota
	auditsDeleteState
	auditsRunningState
	auditsPromptState
)

type AuditsModel struct {
	deps         Deps
	state        auditsState
	audits       []*domain.Audit
	table        table.Model
	prompt       textinput.Model
	progress     ProgressModel
	progressCh   chan ProgressMsg
	progressDone chan struct{}
	loaded       bool
	deleteID     string
	deleteName   string
	width        int
	height       int
}

func NewAuditsModel(deps Deps) AuditsModel {
	return AuditsModel{
		deps:     deps,
		state:    auditsListState,
		table:    table.New(),
		progress: NewProgressModel(),
	}
}

func (m AuditsModel) Init() tea.Cmd {
	return m.loadCmd()
}

func (m AuditsModel) Loaded() bool {
	return m.loaded
}

func (m AuditsModel) NavigationEnabled() bool {
	return m.state == auditsListState
}

func (m AuditsModel) loadCmd() tea.Cmd {
	return func() tea.Msg {
		audits, err := m.deps.AuditService.List(context.Background(), domain.AuditFilter{})
		return auditsLoadedMsg{audits: audits, err: err}
	}
}

func (m AuditsModel) Update(msg tea.Msg) (AuditsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.rebuildTable()
		return m, nil

	case auditsLoadedMsg:
		m.loaded = true
		if msg.err != nil {
			return m, NotifyDanger(fmt.Sprintf("Failed to load audits: %v", msg.err))
		}
		m.audits = msg.audits
		m.rebuildTable()
		return m, nil

	case ProgressMsg:
		if m.state != auditsRunningState {
			return m, nil
		}
		var cmd tea.Cmd
		m.progress, cmd = m.progress.Update(msg)
		return m, tea.Batch(cmd, newProgressWaitCmd(m.progressCh, m.progressDone))

	case progress.FrameMsg:
		if m.state == auditsRunningState {
			var cmd tea.Cmd
			m.progress, cmd = m.progress.Update(msg)
			return m, cmd
		}
	}

	switch m.state {
	case auditsListState:
		return m.updateList(msg)
	case auditsDeleteState:
		return m.updateDelete(msg)
	case auditsPromptState:
		return m.updatePrompt(msg)
	}
	return m, nil
}

func (m AuditsModel) updateList(msg tea.Msg) (AuditsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q":
			return m, tea.Quit
		case "n":
			ti := textinput.New()
			ti.Placeholder = "https://example.com/page"
			ti.CharLimit = 2048
			ti.Width = 60
			ti.Focus()
			m.prompt = ti
			m.state = auditsPromptState
			return m, nil
		case "r":
			idx := m.table.Cursor()
			if idx < len(m.audits) {
				return m.startRerun(m.audits[idx])
			}
			return m, nil
		case "d", "x":
			idx := m.table.Cursor()
			if idx < len(m.audits) {
				m.deleteID = m.audits[idx].ID
				m.deleteName = m.audits[idx].PlanName
				m.state = auditsDeleteState
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m AuditsModel) updatePrompt(msg tea.Msg) (AuditsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.state = auditsListState
			return m, nil
		case "enter":
			raw := strings.TrimSpace(m.prompt.Value())
			if raw == "" {
				return m, NotifyDanger("Enter a URL to audit")
			}
			if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
				raw = "https://" + raw
			}
			plan := &domain.Plan{Name: "Ad-hoc Run", URLs: []string{raw}}
			return m.startRun(plan, "Ad-hoc Run")
		}
	}

	var cmd tea.Cmd
	m.prompt, cmd = m.prompt.Update(msg)
	return m, cmd
}

// startRerun audits the URLs behind a historical audit entry again, using the
// original plan when it still exists and otherwise the stored report URLs.
func (m AuditsModel) startRerun(a *domain.Audit) (AuditsModel, tea.Cmd) {
	plan := &domain.Plan{Name: a.PlanName}

	if a.PlanID != "" {
		if existing, err := m.deps.PlanService.GetByID(context.Background(), a.PlanID); err == nil {
			plan = existing
		}
	}

	if len(plan.URLs) == 0 {
		full, err := m.deps.AuditService.GetByID(context.Background(), a.ID)
		if err != nil {
			return m, NotifyDanger(fmt.Sprintf("Cannot rerun audit: %v", err))
		}
		for _, rep := range full.Reports {
			plan.URLs = append(plan.URLs, rep.URL)
		}
		if len(plan.URLs) == 0 {
			return m, NotifyDanger("Cannot rerun audit: no target URLs found")
		}
	}

	return m.startRun(plan, fmt.Sprintf("Rerunning '%s'", a.PlanName))
}

// startRun launches an audit for the given plan against the active checklist.
func (m AuditsModel) startRun(plan *domain.Plan, title string) (AuditsModel, tea.Cmd) {
	checklist, checks, err := prepareRun(m.deps)
	if err != nil {
		return m, NotifyDanger(err.Error())
	}

	m.state = auditsRunningState
	m.progress = m.progress.Start(title, m.width)
	m.progressCh = make(chan ProgressMsg, 16)
	m.progressDone = make(chan struct{})

	return m, tea.Batch(
		newRunCmd(m.deps, plan, checklist, checks, AuditsView, AuditsView, m.progressCh, m.progressDone),
		newProgressWaitCmd(m.progressCh, m.progressDone),
	)
}

// finishRun restores the model to its list state after a run completes.
func (m AuditsModel) finishRun() AuditsModel {
	m.state = auditsListState
	m.progressCh = nil
	m.progressDone = nil
	return m
}

// markStale forces the next activation to reload the audit history.
func (m AuditsModel) markStale() AuditsModel {
	m.loaded = false
	return m
}

func (m AuditsModel) updateDelete(msg tea.Msg) (AuditsModel, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch key.String() {
	case "y", "Y":
		var notice tea.Cmd
		if err := m.deps.AuditService.Delete(context.Background(), m.deleteID); err != nil {
			notice = NotifyDanger(fmt.Sprintf("Delete failed: %v", err))
		} else {
			notice = NotifySuccess(fmt.Sprintf("Deleted audit '%s'", m.deleteName))
		}
		m.state = auditsListState
		return m, tea.Batch(m.loadCmd(), notice)
	default:
		m.deleteID = ""
		m.deleteName = ""
		m.state = auditsListState
		return m, nil
	}
}

func (m AuditsModel) View() string {
	switch m.state {
	case auditsListState:
		return m.listView()
	case auditsDeleteState:
		return overlay(m.listView(), m.deleteView(), m.width, m.height)
	case auditsRunningState:
		return m.progress.View()
	case auditsPromptState:
		return overlay(m.listView(), m.promptView(), m.width, m.height)
	}
	return ""
}

func (m AuditsModel) listView() string {
	if !m.loaded {
		return "Loading audits..."
	}
	if len(m.audits) == 0 {
		return "No audits yet. Press 'n' to audit a single URL or run a plan."
	}
	return m.table.View()
}

func (m AuditsModel) deleteView() string {
	return fmt.Sprintf("Delete audit '%s'? This cannot be undone.", m.deleteName)
}

func (m AuditsModel) promptView() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Audit a single URL"))
	b.WriteString("\n\n")
	b.WriteString("URL: " + m.prompt.View())
	return b.String()
}

func (m AuditsModel) Help() string {
	switch m.state {
	case auditsDeleteState:
		return "y: Delete  •  any other key: Cancel"
	case auditsRunningState:
		return "Audit in progress  •  Ctrl+C: Quit"
	case auditsPromptState:
		return "Enter: Run  •  Esc: Cancel"
	default:
		return "n: Audit URL  •  r: Rerun  •  d: Delete  •  q: Quit"
	}
}

func (m *AuditsModel) rebuildTable() {
	columns := []table.Column{
		{Title: "Plan", Width: 22},
		{Title: "Checklist", Width: 20},
		{Title: "Started", Width: 17},
		{Title: "Duration", Width: 11},
		{Title: "Failed", Width: 8},
		{Title: "Severity", Width: 10},
	}

	rows := make([]table.Row, 0, len(m.audits))
	for _, a := range m.audits {
		rows = append(rows, table.Row{
			a.PlanName,
			a.ChecklistName,
			a.StartedAt.Format("2006-01-02 15:04"),
			a.Duration.Round(time.Millisecond).String(),
			fmt.Sprintf("%d", a.Summary.FailedCount),
			string(a.Summary.HighestSeverity),
		})
	}

	height := m.height
	if m.height <= 0 {
		height = 10
	}
	if height < 3 {
		height = 3
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(height),
	)
	t.SetStyles(tableStyle())
	if m.width > 0 {
		t.SetWidth(m.width - 2)
	}
	m.table = t
}
