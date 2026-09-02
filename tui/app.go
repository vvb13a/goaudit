package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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
	AuditsView
)

// Reserved screen lines for the top header bar and the bottom footer bar.
const (
	reservedHeaderLines = 1
	reservedFooterLines = 1
)

// Model is the root bubbletea model of the GoAudit terminal UI. It lays the
// full screen out as: a header bar (navigation pills, notifications on the
// right) on top, the active nested view filling the middle edge to edge, and
// a footer bar with contextual help pinned to the bottom.
type Model struct {
	deps          Deps
	nav           NavModel
	footer        FooterModel
	notifications NotificationModel
	plans         PlansModel
	checklists    ChecklistsModel
	audits        AuditsModel
	width         int
	height        int
}

func New(deps Deps) Model {
	return Model{
		deps: deps,
		nav: NewNavModel([]Tab{
			{ID: PlansView, Label: "Plans"},
			{ID: ChecklistsView, Label: "Checklists"},
			{ID: AuditsView, Label: "Audits"},
		}),
		footer:        NewFooterModel(),
		notifications: NewNotificationModel(),
		plans:         NewPlansModel(deps.PlanService),
		checklists:    NewChecklistsModel(deps.ChecklistService, deps.Registry),
		audits:        NewAuditsModel(deps.AuditService),
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
	case AuditsView:
		if !m.audits.Loaded() {
			return m.audits.Init()
		}
	}
	return nil
}

func (m Model) Init() tea.Cmd {
	return m.initCmdForActiveView()
}

// contentHeight returns the number of lines available to the active nested
// view, between the header and footer bars.
func (m Model) contentHeight() int {
	h := m.height - reservedHeaderLines - reservedFooterLines
	if h < 1 {
		h = 1
	}
	return h
}

// viewIsRoot reports whether the active nested view is at its top-level list
// state, where tab navigation is allowed.
func (m Model) viewIsRoot() bool {
	switch m.nav.Active() {
	case PlansView:
		return m.plans.NavigationEnabled()
	case ChecklistsView:
		return m.checklists.NavigationEnabled()
	case AuditsView:
		return m.audits.NavigationEnabled()
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

	switch msg := msg.(type) {
	case notifyMsg:
		m.notifications = m.notifications.Push(msg.notification)
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.footer, _ = m.footer.Update(msg)
		// Hand the nested view only the region between the two bars.
		msg = tea.WindowSizeMsg{Width: msg.Width, Height: m.contentHeight()}
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
	case AuditsView:
		var cmd tea.Cmd
		m.audits, cmd = m.audits.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) activeViewHelp() string {
	switch m.nav.Active() {
	case PlansView:
		return m.plans.Help()
	case ChecklistsView:
		return m.checklists.Help()
	case AuditsView:
		return m.audits.Help()
	}
	return ""
}

// chromeSpaces returns n background-colored spaces used to extend the header
// bar across the full terminal width.
func (m Model) chromeSpaces(n int) string {
	if n <= 0 || m.width <= 0 {
		return ""
	}
	return chromeSpaceStyle.Render(strings.Repeat(" ", n))
}

// headerLine renders the header bar: navigation pills on the left and any
// active notification on the right, stretched edge to edge.
func (m Model) headerLine() string {
	pills := m.nav.Pills()
	if m.width <= 0 {
		return strings.Join(pills, " ")
	}

	pillGap := len(pills) - 1

	leftW := chromePadding
	for _, p := range pills {
		leftW += lipgloss.Width(p)
	}
	leftW += pillGap

	notification := ""
	if m.notifications.IsActive() {
		budget := m.width - leftW - chromePadding*3
		if budget > 10 {
			notification = m.notifications.View(budget)
		}
	}
	notifW := lipgloss.Width(notification)

	fill := m.width - leftW - notifW - chromePadding
	if notification != "" {
		fill--
	}
	if fill < 0 {
		fill = 0
	}

	var b strings.Builder
	b.WriteString(m.chromeSpaces(chromePadding))
	for i, pill := range pills {
		if i > 0 {
			b.WriteString(m.chromeSpaces(1))
		}
		b.WriteString(pill)
	}
	b.WriteString(m.chromeSpaces(fill))
	if notification != "" {
		b.WriteString(m.chromeSpaces(1))
		b.WriteString(notification)
	}
	b.WriteString(m.chromeSpaces(chromePadding))
	return b.String()
}

func (m Model) View() string {
	var b strings.Builder

	b.WriteString(m.headerLine())
	b.WriteString("\n")

	content := fillLines(m.activeViewContent(), m.contentHeight())
	b.WriteString(strings.Join(content, "\n"))

	b.WriteString("\n")

	right := ""
	if m.viewIsRoot() {
		right = "Tab / Shift+Tab: Switch View"
	}
	b.WriteString(m.footer.WithContent(m.activeViewHelp(), right).View())

	return b.String()
}

func (m Model) activeViewContent() string {
	switch m.nav.Active() {
	case PlansView:
		return m.plans.View()
	case ChecklistsView:
		return m.checklists.View()
	case AuditsView:
		return m.audits.View()
	}
	return ""
}

// fillLines pads or truncates content so it occupies exactly h lines.
func fillLines(content string, h int) []string {
	lines := strings.Split(content, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return lines
}
