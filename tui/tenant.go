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

// tenantResetMsg reports the outcome of resetting the run data of a tenant
// (audit): its issues, URLs and snapshots were cleared.
type tenantResetMsg struct {
	id   string
	name string
	err  error
}

// tenantClonedMsg reports the outcome of duplicating a tenant (audit)
// without its run data.
type tenantClonedMsg struct {
	audit *domain.Audit
	err   error
}

// switcherAction is a destructive action awaiting confirmation in the tenant
// switcher.
type switcherAction string

const (
	switcherActionDelete switcherAction = "delete"
	switcherActionReset  switcherAction = "reset"
)

// switcherPrompt is one audit pending a destructive confirmation.
type switcherPrompt struct {
	audit  *domain.Audit
	action switcherAction
}

// promptText explains the destructive action that is pending confirmation.
func (p *switcherPrompt) promptText() string {
	switch p.action {
	case switcherActionReset:
		return fmt.Sprintf("Reset audit '%s'?\n\nDeletes its URLs, issues and run history.\nThe audit configuration is kept.\n\nPress y to confirm or any other key to cancel.", p.audit.Name)
	default:
		return fmt.Sprintf("Delete audit '%s'?\n\nThis cannot be undone.\n\nPress y to confirm or any other key to cancel.", p.audit.Name)
	}
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
	m.switcherPrompt = nil

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
		m.config = m.config.Track("")
		m.auditIssues = m.auditIssues.Track("")
		m.timeline = m.timeline.Track("")
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

// resetTenantCmd clears the run data of the given audit (URLs, issues,
// snapshots and exports) while keeping the audit record and reports back
// through tenantResetMsg.
func (m Model) resetTenantCmd(a *domain.Audit) tea.Cmd {
	return func() tea.Msg {
		err := m.deps.AuditService.ResetData(context.Background(), a.ID)
		return tenantResetMsg{id: a.ID, name: a.Name, err: err}
	}
}

// cloneTenantCmd duplicates the given audit without its run data and reports
// back through tenantClonedMsg.
func (m Model) cloneTenantCmd(a *domain.Audit) tea.Cmd {
	return func() tea.Msg {
		audit, err := m.deps.AuditService.Duplicate(context.Background(), a.ID)
		return tenantClonedMsg{audit: audit, err: err}
	}
}

// handleTenantReset updates the current tenant after its run data was
// cleared: the audit stays selected but every view that showed its data is
// released and reloaded empty.
func (m Model) handleTenantReset(msg tenantResetMsg) (tea.Model, tea.Cmd) {
	m.switcherPrompt = nil

	if msg.err != nil {
		return m.pushNotification(Notification{
			Kind: NotificationDanger,
			Text: fmt.Sprintf("Reset failed: %v", msg.err),
		}), nil
	}

	// The audit row moved to the top of the switcher list (its started_at
	// refreshed); reload the list even when another audit is selected.
	var cmds []tea.Cmd
	if msg.id == m.tenantID {
		m.dashboard = m.dashboard.release()
		m = m.releaseView(UrlsView)
		m = m.releaseView(IssuesView)
		m = m.releaseView(TimelineView)
		cmds = append(cmds, m.activateCmd())
	}
	cmds = append(cmds, m.loadTenantsCmd())

	return m.pushNotification(Notification{
		Kind: NotificationSuccess,
		Text: fmt.Sprintf("Reset audit '%s'", msg.name),
	}), tea.Batch(cmds...)
}

// handleTenantCloned refreshes the switcher list so the duplicate appears.
func (m Model) handleTenantCloned(msg tenantClonedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m.pushNotification(Notification{
			Kind: NotificationDanger,
			Text: fmt.Sprintf("Clone failed: %v", msg.err),
		}), nil
	}
	if msg.audit == nil {
		return m.pushNotification(Notification{
			Kind: NotificationDanger,
			Text: "Clone failed: no audit returned",
		}), nil
	}
	return m.pushNotification(Notification{
		Kind: NotificationSuccess,
		Text: fmt.Sprintf("Cloned audit '%s'", msg.audit.Name),
	}), m.loadTenantsCmd()
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
	m.switcherPrompt = nil
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
	m.switcherPrompt = nil
	return m
}

// handleSwitcherKey processes a key while the tenant switcher overlay is
// open. All other keys are consumed by the switcher.
func (m Model) handleSwitcherKey(key tea.KeyMsg) (Model, tea.Cmd) {
	// A destructive confirmation supersedes the list keys.
	if m.switcherPrompt != nil {
		if key.String() == "y" || key.String() == "Y" {
			audit := m.switcherPrompt.audit
			var cmd tea.Cmd
			switch m.switcherPrompt.action {
			case switcherActionReset:
				cmd = m.resetTenantCmd(audit)
			default:
				cmd = m.deleteTenantCmd(audit)
			}
			m.switcherPrompt = nil
			m.switcherOpen = false
			return m, cmd
		}
		m.switcherPrompt = nil
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
			m.switcherPrompt = &switcherPrompt{audit: t, action: switcherActionDelete}
		}
		return m, nil
	case "x":
		if t := m.selTenant(); t != nil {
			m.switcherPrompt = &switcherPrompt{audit: t, action: switcherActionReset}
		}
		return m, nil
	case "c":
		if t := m.selTenant(); t != nil {
			return m, m.cloneTenantCmd(t)
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
	m.switcherPrompt = nil
	m.nav = m.nav.Select(DashboardView)

	// Release the list views of the previous tenant so their loaded pages
	// are not kept resident while browsing the new audit.
	m = m.releaseView(UrlsView)
	m = m.releaseView(IssuesView)
	m = m.releaseView(TimelineView)

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
	if p := m.switcherPrompt; p != nil {
		return p.promptText()
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
		line := fmt.Sprintf("%s%s  %s  %s", marker, clipCell(t.Name, nameW), timeAgo(t.StartedAt, true), fmt.Sprintf("%.1f", t.Score))
		b.WriteString(line)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) switcherHelp() string {
	if p := m.switcherPrompt; p != nil {
		if p.action == switcherActionReset {
			return "y: Reset  •  any other key: Cancel"
		}
		return "y: Delete  •  any other key: Cancel"
	}
	return "↑/↓: Move  •  Enter: Open  •  n: New Audit  •  r: Rerun  •  d: Delete  •  x: Reset  •  c: Duplicate  •  Esc: Close"
}
