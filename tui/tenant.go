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
		m.dashboard = m.dashboard.clearTenant()
		m.auditUrls = m.auditUrls.Track("")
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
		m.dashboard = m.dashboard.beginNewAudit()
		m.nav = m.nav.Select(DashboardView)
		return m, m.activateCmd()
	case "r":
		if t := m.selTenant(); t != nil {
			m.switcherOpen = false
			m.nav = m.nav.Select(DashboardView)
			var cmd tea.Cmd
			m.dashboard, cmd = m.dashboard.startRerun(t)
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
	m.nav = m.nav.Select(DashboardView)

	var cmd tea.Cmd
	m.dashboard, cmd = m.dashboard.openDetail(t.ID)
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

// ---- Exports ----

// exportExcelCmd hydrates the full audit (reports with issues), renders it
// into an Excel workbook (reusing an existing one instead of regenerating)
// and opens the workbook with the default xlsx viewer.
func exportExcelCmd(deps Deps, a *domain.Audit) tea.Cmd {
	return func() tea.Msg {
		if deps.ExcelService == nil {
			return notifyMsg{notification: Notification{
				Kind: NotificationDanger,
				Text: "Excel export is not available",
			}}
		}

		full, err := deps.AuditService.GetByID(context.Background(), a.ID)
		if err != nil {
			return notifyMsg{notification: Notification{
				Kind: NotificationDanger,
				Text: fmt.Sprintf("Export failed: %v", err),
			}}
		}

		path, err := deps.ExcelService.ExportAudit(full)
		if err != nil {
			return notifyMsg{notification: Notification{
				Kind: NotificationDanger,
				Text: fmt.Sprintf("Export failed: %v", err),
			}}
		}

		if err := openWithDefaultApp(path); err != nil {
			return notifyMsg{notification: Notification{
				Kind: NotificationDanger,
				Text: fmt.Sprintf("Failed to open '%s': %v", path, err),
			}}
		}

		return notifyMsg{notification: Notification{
			Kind: NotificationSuccess,
			Text: fmt.Sprintf("Opened audit workbook %s", path),
		}}
	}
}

// exportHTMLCmd hydrates the full audit (reports with issues), renders it
// into an HTML report (reusing an existing one instead of regenerating) and
// opens the report in the default browser.
func exportHTMLCmd(deps Deps, a *domain.Audit) tea.Cmd {
	return func() tea.Msg {
		if deps.HtmlService == nil {
			return notifyMsg{notification: Notification{
				Kind: NotificationDanger,
				Text: "HTML report is not available",
			}}
		}

		full, err := deps.AuditService.GetByID(context.Background(), a.ID)
		if err != nil {
			return notifyMsg{notification: Notification{
				Kind: NotificationDanger,
				Text: fmt.Sprintf("Export failed: %v", err),
			}}
		}

		path, err := deps.HtmlService.ExportAudit(full)
		if err != nil {
			return notifyMsg{notification: Notification{
				Kind: NotificationDanger,
				Text: fmt.Sprintf("Export failed: %v", err),
			}}
		}

		if err := openWithDefaultApp(path); err != nil {
			return notifyMsg{notification: Notification{
				Kind: NotificationDanger,
				Text: fmt.Sprintf("Failed to open '%s': %v", path, err),
			}}
		}

		return notifyMsg{notification: Notification{
			Kind: NotificationSuccess,
			Text: fmt.Sprintf("Opened audit report %s", path),
		}}
	}
}
