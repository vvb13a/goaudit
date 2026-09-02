package tui

import (
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
)

// Model is the root bubbletea model of the GoAudit terminal UI. It routes
// messages to the currently active nested view model.
type Model struct {
	deps  Deps
	view  ViewID
	plans PlansModel
}

func New(deps Deps) Model {
	return Model{
		deps:  deps,
		view:  PlansView,
		plans: NewPlansModel(deps.PlanService),
	}
}

func (m Model) Init() tea.Cmd {
	return m.plans.Init()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	}

	switch m.view {
	case PlansView:
		var cmd tea.Cmd
		m.plans, cmd = m.plans.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) View() string {
	switch m.view {
	case PlansView:
		return m.plans.View()
	}
	return ""
}
