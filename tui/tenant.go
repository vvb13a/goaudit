package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vvb13a/goaudit/domain"
)

// tenantsLoadedMsg carries the audit list backing the tenant switcher.
type tenantsLoadedMsg struct {
	tenants []*domain.Audit
	err     error
}

// tenantDeletedMsg reports the outcome of deleting a tenant (audit).
type tenantDeletedMsg struct {
	id   string
	name string
	err  error
}

// loadTenantsCmd refreshes the tenant (audit) list of the switcher.
func (m Model) loadTenantsCmd() tea.Cmd {
	return func() tea.Msg {
		tenants, err := m.deps.AuditService.List(context.Background(), domain.AuditFilter{})
		return tenantsLoadedMsg{tenants: tenants, err: err}
	}
}

func (m Model) handleTenantsLoaded(msg tenantsLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m.pushNotification(Notification{
			Kind: NotificationDanger,
			Text: fmt.Sprintf("Failed to load audits: %v", msg.err),
		}), nil
	}

	m.tenants = msg.tenants
	sort.SliceStable(m.tenants, func(i, j int) bool {
		return m.tenants[i].StartedAt.After(m.tenants[j].StartedAt)
	})
	m.clampSwitcherCursor()

	// The first load automatically opens the most recent audit as tenant,
	// unless a tenant is already set (e.g. right after a finished run).
	if !m.tenantPicked && m.tenantID == "" && len(m.tenants) > 0 {
		m.tenantPicked = true
		return m.switchToTenant(m.tenants[0])
	}
	return m, nil
}

func (m Model) handleTenantDeleted(msg tenantDeletedMsg) (tea.Model, tea.Cmd) {
	m.switcherConfirm = nil

	if msg.err != nil {
		return m.pushNotification(Notification{
			Kind: NotificationDanger,
			Text: fmt.Sprintf("Delete failed: %v", msg.err),
		}), nil
	}

	tenants := m.tenants[:0]
	for _, t := range m.tenants {
		if t.ID != msg.id {
			tenants = append(tenants, t)
		}
	}
	m.tenants = tenants
	m.clampSwitcherCursor()

	if msg.id == m.tenantID {
		m.setTenant("", "")
		m.audits = m.audits.clearTenant()
		m.auditEdit = m.auditEdit.Track("")
		m.auditChecks = m.auditChecks.Track("")
		m.auditIssues = m.auditIssues.Track("")
	}

	return m.pushNotification(Notification{
		Kind: NotificationSuccess,
		Text: fmt.Sprintf("Deleted audit '%s'", msg.name),
	}), nil
}

// deleteTenantCmd removes the given audit (and its exported files) and
// reports back through tenantDeletedMsg.
func (m Model) deleteTenantCmd(a *domain.Audit) tea.Cmd {
	return func() tea.Msg {
		err := m.deps.AuditService.Delete(context.Background(), a.ID)
		return tenantDeletedMsg{id: a.ID, name: a.Name, err: err}
	}
}

// setTenant updates the current tenant shown in the header and audit view.
func (m Model) setTenant(id, name string) Model {
	m.tenantID = id
	m.tenantName = name
	return m
}

// openSwitcher opens the tenant switcher overlay, anchored on the current
// tenant when it is still part of the list.
func (m Model) openSwitcher() Model {
	m.switcherOpen = true
	m.switcherConfirm = nil
	m.clampSwitcherCursor()
	for i, t := range m.tenants {
		if t.ID == m.tenantID {
			m.switcherCursor = i
			break
		}
	}
	return m
}

func (m Model) closeSwitcher() Model {
	m.switcherOpen = false
	m.switcherConfirm = nil
	return m
}

// handleSwitcherKey processes a key while the tenant switcher overlay is
// open. All other keys are consumed by the switcher.
func (m Model) handleSwitcherKey(key tea.KeyMsg) (Model, tea.Cmd) {
	// Deletion confirmation supersedes the list keys.
	if m.switcherConfirm != nil {
		if key.String() == "y" || key.String() == "Y" {
			cmd := m.deleteTenantCmd(m.switcherConfirm)
			m.switcherConfirm = nil
			m.switcherOpen = false
			return m, cmd
		}
		m.switcherConfirm = nil
		return m, nil
	}

	switch key.String() {
	case "esc", "q", "ctrl+o":
		return m.closeSwitcher(), nil
	case "up", "k":
		if m.switcherCursor > 0 {
			m.switcherCursor--
		}
		return m, nil
	case "down", "j":
		if m.switcherCursor < len(m.tenants)-1 {
			m.switcherCursor++
		}
		return m, nil
	case "enter":
		if t := m.selTenant(); t != nil {
			return m.switchToTenant(t)
		}
		return m, nil
	case "n":
		m.switcherOpen = false
		m.audits = m.audits.beginNewAudit()
		m.nav = m.nav.Select(AuditsView)
		return m, m.activateCmd()
	case "r":
		if t := m.selTenant(); t != nil {
			m.switcherOpen = false
			m.nav = m.nav.Select(AuditsView)
			var cmd tea.Cmd
			m.audits, cmd = m.audits.startRerun(t)
			return m, cmd
		}
		return m, nil
	case "d":
		if t := m.selTenant(); t != nil {
			m.switcherConfirm = t
		}
		return m, nil
	case "e":
		if t := m.selTenant(); t != nil {
			return m, exportExcelCmd(m.deps, t)
		}
		return m, nil
	case "w":
		if t := m.selTenant(); t != nil {
			return m, exportHTMLCmd(m.deps, t)
		}
		return m, nil
	}
	return m, nil
}

// switchToTenant makes the given audit the current tenant and opens it in
// the audit view, loading its details from the store.
func (m Model) switchToTenant(t *domain.Audit) (Model, tea.Cmd) {
	m = m.setTenant(t.ID, t.Name)
	m.tenantPicked = true
	m.switcherOpen = false
	m.switcherConfirm = nil
	m.nav = m.nav.Select(AuditsView)

	var cmd tea.Cmd
	m.audits, cmd = m.audits.openDetail(t.ID)
	return m, cmd
}

func (m Model) selTenant() *domain.Audit {
	if m.switcherCursor < 0 || m.switcherCursor >= len(m.tenants) {
		return nil
	}
	return m.tenants[m.switcherCursor]
}

func (m Model) clampSwitcherCursor() {
	if m.switcherCursor < 0 {
		m.switcherCursor = 0
	}
	if m.switcherCursor >= len(m.tenants) {
		m.switcherCursor = len(m.tenants) - 1
	}
	if len(m.tenants) == 0 {
		m.switcherCursor = 0
	}
}

// switcherView renders the tenant picker overlay with its action hints.
func (m Model) switcherView() string {
	if c := m.switcherConfirm; c != nil {
		return fmt.Sprintf("Delete audit '%s'?\n\nThis cannot be undone.\n\nPress y to confirm or any other key to cancel.", c.Name)
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render("Audit Switcher"))
	b.WriteString("\n\n")

	if m.tenants == nil {
		b.WriteString(helpStyle.Render("Loading tenants..."))
		return b.String()
	}
	if len(m.tenants) == 0 {
		b.WriteString(helpStyle.Render("No audits yet — press 'n' to run the first one."))
		return b.String()
	}

	nameW := m.width - 46
	if nameW < 12 {
		nameW = 12
	}
	for i, t := range m.tenants {
		marker := "  "
		if i == m.switcherCursor {
			marker = labelStyle.Render("> ")
		}
		sev := string(t.Summary.HighestSeverity)
		if sev == "" {
			sev = "-"
		}
		line := fmt.Sprintf("%s%s  %s  %s", marker, clipCell(t.Name, nameW), timeAgo(t.StartedAt, true), sev)
		b.WriteString(line)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) switcherHelp() string {
	if m.switcherConfirm != nil {
		return "y: Delete  •  any other key: Cancel"
	}
	return "↑/↓: Move  •  Enter: Open  •  n: New Audit  •  r: Rerun  •  d: Delete  •  e: Excel  •  w: HTML  •  Esc: Close"
}
