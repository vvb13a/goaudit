package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/vvb13a/goaudit/domain"
	"github.com/vvb13a/goaudit/service"

	"github.com/charmbracelet/bubbles/table"
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
)

type AuditsModel struct {
	svc        *service.AuditService
	state      auditsState
	audits     []*domain.Audit
	table      table.Model
	loaded     bool
	deleteID   string
	deleteName string
	width      int
	height     int
}

func NewAuditsModel(svc *service.AuditService) AuditsModel {
	return AuditsModel{
		svc:   svc,
		state: auditsListState,
		table: table.New(),
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
		audits, err := m.svc.List(context.Background(), domain.AuditFilter{})
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
	}

	switch m.state {
	case auditsListState:
		return m.updateList(msg)
	case auditsDeleteState:
		return m.updateDelete(msg)
	}
	return m, nil
}

func (m AuditsModel) updateList(msg tea.Msg) (AuditsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q":
			return m, tea.Quit
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

func (m AuditsModel) updateDelete(msg tea.Msg) (AuditsModel, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch key.String() {
	case "y", "Y":
		var notice tea.Cmd
		if err := m.svc.Delete(context.Background(), m.deleteID); err != nil {
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
		return m.deleteView()
	}
	return ""
}

func (m AuditsModel) listView() string {
	if !m.loaded {
		return "Loading audits..."
	}
	if len(m.audits) == 0 {
		return "No audits yet. Run a plan to see its history here."
	}
	return m.table.View()
}

func (m AuditsModel) deleteView() string {
	return fmt.Sprintf("Delete audit '%s'? This cannot be undone.", m.deleteName)
}

func (m AuditsModel) Help() string {
	switch m.state {
	case auditsDeleteState:
		return "y: Delete  •  any other key: Cancel"
	default:
		return "d: Delete  •  q: Quit"
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
