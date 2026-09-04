package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vvb13a/goaudit/domain"
)

// AuditChecksModel edits which checks run for the current audit (tenant). It
// is its own tab so per-check configuration can grow later without touching
// the audit editor.
type AuditChecksModel struct {
	deps    Deps
	auditID string
	loaded  bool
	// snapshot of the audit configuration preserved on save.
	audit   *domain.Audit
	options []auditOption
	cursor  int
	width   int
	height  int
}

func NewAuditChecksModel(deps Deps) AuditChecksModel {
	return AuditChecksModel{
		deps: deps,
	}
}

func (m AuditChecksModel) NavigationEnabled() bool {
	return false
}

// Track points the editor at the given audit and marks it stale so the next
// activation reloads the stored configuration.
func (m AuditChecksModel) Track(id string) AuditChecksModel {
	if m.auditID != id {
		m.loaded = false
		m.options = nil
	}
	m.auditID = id
	return m
}

func (m AuditChecksModel) loadCmd(id string) tea.Cmd {
	return func() tea.Msg {
		audit, err := m.deps.AuditService.GetConfig(context.Background(), id)
		return auditConfigLoadedMsg{id: id, audit: audit, err: err}
	}
}

func (m AuditChecksModel) Update(msg tea.Msg) (AuditChecksModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case auditConfigLoadedMsg:
		if msg.id != m.auditID {
			return m, nil
		}
		m.loaded = true
		if msg.err != nil {
			return m, NotifyDanger(fmt.Sprintf("Failed to load audit: %v", msg.err))
		}
		m.audit = msg.audit
		m.options = m.buildOptions(msg.audit.CheckNames)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case "down", "j":
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}
			return m, nil
		case " ":
			if len(m.options) > 0 {
				opt := &m.options[m.cursor]
				opt.selected = !opt.selected
			}
			return m, nil
		case "enter", "ctrl+s":
			return m.save()
		}
	}
	return m, nil
}

// buildOptions renders every registered check, preselected from the audit's
// stored check names. Audits without stored names (legacy rows) default to
// all checks.
func (m AuditChecksModel) buildOptions(checkNames []string) []auditOption {
	selected := make(map[string]struct{}, len(checkNames))
	for _, n := range checkNames {
		selected[n] = struct{}{}
	}
	useAll := len(checkNames) == 0

	var options []auditOption
	for _, c := range m.deps.Registry.All() {
		name := c.Info().Name
		options = append(options, auditOption{
			name:     name,
			category: c.Info().Category,
			selected: useAll,
		})
		if _, ok := selected[name]; ok {
			options[len(options)-1].selected = true
		}
	}
	return options
}

// save persists the selected check names while preserving the rest of the
// audit configuration.
func (m AuditChecksModel) save() (AuditChecksModel, tea.Cmd) {
	if m.auditID == "" || m.audit == nil {
		return m, NotifyDanger("No audit selected")
	}

	var chosen []string
	for _, opt := range m.options {
		if opt.selected {
			chosen = append(chosen, opt.name)
		}
	}
	if len(chosen) == 0 {
		return m, NotifyDanger("Select at least one check")
	}

	id := m.auditID
	name := m.audit.Name
	description := m.audit.Description
	targets := append([]string(nil), m.audit.Targets...)
	config := append(json.RawMessage(nil), m.audit.Config...)
	return m, func() tea.Msg {
		err := m.deps.AuditService.UpdateConfig(context.Background(), id, name, description, targets, chosen, config)
		return auditConfigSavedMsg{id: id, name: name, err: err}
	}
}

func (m AuditChecksModel) View() string {
	if m.auditID == "" {
		return centerLines(helpStyle.Render("No audit selected. Press Ctrl+O to open the audit switcher."), m.width)
	}
	if !m.loaded {
		return "Loading checks..."
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render("Edit Audit Checks"))
	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render(fmt.Sprintf("Running checks for '%s' — Space to toggle.", m.audit.Name)))
	b.WriteString("\n\n")

	for i, opt := range m.options {
		cursor := "  "
		if i == m.cursor {
			cursor = labelStyle.Render("> ")
		}
		checked := "[ ]"
		if opt.selected {
			checked = labelStyle.Render("[x]")
		}
		cat := helpStyle.Render(string(opt.category))
		b.WriteString(fmt.Sprintf("%s%s %-25s %s\n", cursor, checked, opt.name, cat))
	}

	selected := 0
	for _, opt := range m.options {
		if opt.selected {
			selected++
		}
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render(fmt.Sprintf("%d of %d checks selected.", selected, len(m.options))))
	return b.String()
}

func (m AuditChecksModel) Help() string {
	if m.auditID == "" {
		return "Ctrl+O: Audits"
	}
	return "↑/↓: Move  •  Space: Toggle  •  Enter/Ctrl+S: Save  •  Esc: Back  •  Ctrl+O: Audits"
}
