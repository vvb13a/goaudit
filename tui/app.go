package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vvb13a/goaudit/service"
)

// Deps bundles the application services and infrastructure that the UI
// operates on. main wires them together before starting the program.
type Deps struct {
	ConfigManager    *service.Manager
	Registry         *service.CheckRegistry
	Runner           *service.Runner
	PlanService      *service.PlanService
	ChecklistService *service.ChecklistService
	AuditService     *service.AuditService
}

// ViewID identifies which top-level view is currently active.
type ViewID int

const (
	PlansView ViewID = iota
	ChecklistsView
)

// Model is the root bubbletea model of the GoAudit terminal UI. It renders
// the navigation bar and routes messages to the currently active nested view
// model.
type Model struct {
	deps       Deps
	nav        NavModel
	plans      PlansModel
	checklists ChecklistsModel
}

func New(deps Deps) Model {
	return Model{
		deps: deps,
		nav: NewNavModel([]Tab{
			{ID: PlansView, Label: "Plans"},
			{ID: ChecklistsView, Label: "Checklists"},
		}),
		plans:      NewPlansModel(deps.PlanService),
		checklists: NewChecklistsModel(deps.ChecklistService, deps.Registry),
	}
}

// initCmdForActiveView kicks off the initial data load for the active view,
// so a view only loads once it is first shown.
func (m Model) initCmdForActiveView() tea.Cmd {
	switch m.nav.Active() {
	case PlansView:
		if !m.plans.Loaded() {
			return m.plans.Init()
		}
	case ChecklistsView:
		if !m.checklists.Loaded() {
			return m.checklists.Init()
		}
	}
	return nil
}

func (m Model) Init() tea.Cmd {
	return m.initCmdForActiveView()
}

// viewIsRoot reports whether the active nested view is at its top-level list
// state, where tab navigation is allowed.
func (m Model) viewIsRoot() bool {
	switch m.nav.Active() {
	case PlansView:
		return m.plans.NavigationEnabled()
	case ChecklistsView:
		return m.checklists.NavigationEnabled()
	}
	return false
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "tab":
			if m.viewIsRoot() {
				m.nav = m.nav.Next()
				return m, m.initCmdForActiveView()
			}
		case "shift+tab", "backtab":
			if m.viewIsRoot() {
				m.nav = m.nav.Prev()
				return m, m.initCmdForActiveView()
			}
		}

		if len(msg.Runes) == 1 && msg.Runes[0] >= '1' && msg.Runes[0] <= '9' {
			if m.viewIsRoot() {
				m.nav = m.nav.SelectIndex(int(msg.Runes[0]-'0') - 1)
				return m, m.initCmdForActiveView()
			}
		}
	}

	switch m.nav.Active() {
	case PlansView:
		var cmd tea.Cmd
		m.plans, cmd = m.plans.Update(msg)
		return m, cmd
	case ChecklistsView:
		var cmd tea.Cmd
		m.checklists, cmd = m.checklists.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(m.nav.View())
	b.WriteString("\n\n")

	switch m.nav.Active() {
	case PlansView:
		b.WriteString(m.plans.View())
	case ChecklistsView:
		b.WriteString(m.checklists.View())
	}
	return b.String()
}
