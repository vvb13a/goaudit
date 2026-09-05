package tui

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/vvb13a/goaudit/domain"
	"github.com/vvb13a/goaudit/service"
)

// Deps bundles the application services and infrastructure that the UI
// operates on. main wires them together before starting the program.
type Deps struct {
	ConfigManager *service.Manager
	Registry      *service.CheckRegistry
	Runner        *service.Runner
	AuditService  *service.AuditService
	ExcelService  *service.ExcelService
	HtmlService   *service.HtmlService
}

// ViewID identifies which top-level view is currently active.
type ViewID int

const (
	DashboardView ViewID = iota
	IssuesView
	UrlsView
	TimelineView
	ConfigView
)

// viewSizeMsg replays the content dimensions to a view that just became
// active. It is consumed by the root only to forward to the nested model,
// without altering the root's own tracked size.
type viewSizeMsg struct {
	width  int
	height int
}

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
	dashboard     DashboardModel
	auditUrls     AuditUrlsModel
	auditIssues   AuditIssuesModel
	timeline      TimelineModel
	config        AuditConfigModel
	width         int
	height        int

	// Tenant state: the audit is the tenant. tenants holds the audit list
	// for the switcher overlay, and tenantID/tenantName describe the audit
	// currently shown in the header and the audit view.
	tenants      []*domain.Audit
	tenantID     string
	tenantName   string
	tenantPicked bool

	// Tenant switcher overlay state.
	switcherOpen    bool
	switcherCursor  int
	switcherConfirm *domain.Audit

	// Event cycle timing for the footer metrics: eventStart marks the
	// beginning of the current (or most recent) user event, cycleActive
	// stays true while background commands spawned by it are still running,
	// and lastQuietAt is when the last cycle settled.
	eventStart  time.Time
	cycleActive bool
	lastQuietAt time.Time

	dashboardSized   bool
	auditUrlsSized   bool
	auditIssuesSized bool
	timelineSized    bool
	configSized      bool
}

func New(deps Deps) Model {
	return Model{
		deps: deps,
		nav: NewNavModel([]Tab{
			{ID: DashboardView, Label: "Dashboard"},
			{ID: IssuesView, Label: "Issues"},
			{ID: UrlsView, Label: "URLs"},
			{ID: TimelineView, Label: "Timeline"},
			{ID: ConfigView, Label: "Config"},
		}),
		footer:        NewFooterModel(),
		notifications: NewNotificationModel(),
		dashboard:     NewDashboardModel(deps),
		auditUrls:     NewAuditUrlsModel(deps),
		auditIssues:   NewAuditIssuesModel(deps),
		timeline:      NewTimelineModel(deps),
		config:        NewAuditConfigModel(deps),
	}
}

func (m Model) Init() tea.Cmd {
	return m.loadTenantsCmd()
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

// activateCmd returns the commands to run when a view is activated: a config
// reload for the audit editors and, if it has not been sized yet, a replay of
// the current content dimensions.
func (m Model) activateCmd() tea.Cmd {
	var cmds []tea.Cmd

	sized := false
	switch m.nav.Active() {
	case DashboardView:
		sized = m.dashboardSized
	case UrlsView:
		sized = m.auditUrlsSized
	case IssuesView:
		sized = m.auditIssuesSized
	case TimelineView:
		sized = m.timelineSized
	case ConfigView:
		sized = m.configSized
	}

	if m.width > 0 && !sized {
		width, height := m.width, m.contentHeight()
		cmds = append(cmds, func() tea.Msg {
			return viewSizeMsg{width: width, height: height}
		})
	}

	if m.tenantID != "" {
		switch m.nav.Active() {
		case UrlsView:
			cmds = append(cmds, m.auditUrls.loadCmd(m.tenantID))
		case IssuesView:
			cmds = append(cmds, m.auditIssues.loadCmd(m.tenantID))
		case TimelineView:
			cmds = append(cmds, m.timeline.loadCmd(m.tenantID))
		case ConfigView:
			cmds = append(cmds, m.config.loadCmd(m.tenantID))
		}
	}

	switch len(cmds) {
	case 0:
		return nil
	case 1:
		return cmds[0]
	default:
		return tea.Batch(cmds...)
	}
}

func (m Model) markViewSized() {
	switch m.nav.Active() {
	case DashboardView:
		m.dashboardSized = true
	case UrlsView:
		m.auditUrlsSized = true
	case IssuesView:
		m.auditIssuesSized = true
	case ConfigView:
		m.configSized = true
	case TimelineView:
		m.timelineSized = true
	}
}

// viewIsRoot reports whether the active nested view is at its top-level list
// state, where tab and number-key navigation is allowed. The audit editors
// keep navigation out of their forms so keys stay usable for editing.
func (m Model) viewIsRoot() bool {
	switch m.nav.Active() {
	case DashboardView:
		return m.dashboard.NavigationEnabled()
	case UrlsView:
		return m.auditUrls.NavigationEnabled()
	case IssuesView:
		return m.auditIssues.NavigationEnabled()
	case ConfigView:
		return m.config.NavigationEnabled()
	case TimelineView:
		return m.timeline.NavigationEnabled()
	}
	return false
}

// goToView activates the given top-level view, tracking the current tenant in
// the audit views and returning their activation commands.
func (m Model) goToView(id ViewID) (Model, tea.Cmd) {
	m.nav = m.nav.Select(id)
	if m.tenantID != "" {
		switch id {
		case UrlsView:
			m.auditUrls = m.auditUrls.Track(m.tenantID)
		case IssuesView:
			m.auditIssues = m.auditIssues.Track(m.tenantID)
		case TimelineView:
			m.timeline = m.timeline.Track(m.tenantID)
		case ConfigView:
			m.config = m.config.Track(m.tenantID)
		}
	}
	return m, m.activateCmd()
}

// cycleTailWindow is how long after an event cycle settled that trailing
// background messages (e.g. the second half of a tea.Batch) are still
// attributed to that same event instead of starting a new one.
const cycleTailWindow = 250 * time.Millisecond

// Update tracks the full lifetime of an event cycle. A cycle starts when an
// external event (user input, resize) arrives, continues while the event
// spawns background commands, and ends when an Update returns no further
// command; the closing View() then reports the total, which therefore
// includes Update processing, command execution and rendering.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	now := time.Now()

	if isExternalEvent(msg) {
		// A new user event: whatever the previous cycle was, it is over.
		m.eventStart = now
		m.cycleActive = true
	} else if !m.cycleActive && now.Sub(m.lastQuietAt) > cycleTailWindow {
		// Stray background message with no originating user event.
		m.eventStart = now
		m.cycleActive = true
	}

	updated, cmd := m.update(msg)

	if root, ok := updated.(Model); ok {
		if cmd == nil && root.cycleActive {
			// No further command was spawned: the cycle settles here and
			// its total is finalized on the upcoming View.
			root.cycleActive = false
			root.lastQuietAt = time.Now()
		}
		return root, cmd
	}
	return updated, cmd
}

// isExternalEvent reports whether the message originates from the user or
// the terminal rather than from a background command of this application.
func isExternalEvent(msg tea.Msg) bool {
	switch msg.(type) {
	case tea.KeyMsg, tea.MouseMsg, tea.FocusMsg, tea.BlurMsg, tea.WindowSizeMsg:
		return true
	}
	return false
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "ctrl+o":
			if m.switcherOpen {
				return m.closeSwitcher(), nil
			}
			if !m.dashboard.Running() {
				return m.openSwitcher(), nil
			}
			return m, nil
		}

		// While the tenant switcher overlay is open, keys are consumed by it.
		if m.switcherOpen {
			return m.handleSwitcherKey(key)
		}

		switch key.String() {
		case "tab":
			if m.viewIsRoot() {
				next := m.nav.Next()
				return m.goToView(next.Active())
			}
		case "shift+tab", "backtab":
			if m.viewIsRoot() {
				prev := m.nav.Prev()
				return m.goToView(prev.Active())
			}
		case "esc":
			// The audit editors use Esc as their way back to the audit view
			// (Tab cycles their fields, so it cannot leave the view).
			switch m.nav.Active() {
			case ConfigView:
				return m.goToView(DashboardView)
			}
		}

		if len(key.Runes) == 1 && key.Runes[0] >= '1' && key.Runes[0] <= '9' {
			idx := int(key.Runes[0]-'0') - 1
			if idx < m.nav.Count() && m.viewIsRoot() {
				return m.goToView(ViewID(idx))
			}
		}
	}

	// Audit configuration saved by an editor.
	if cm, ok := msg.(auditConfigSavedMsg); ok {
		return m.handleAuditConfigSaved(cm)
	}

	// Tenants of the switcher.
	if tm, ok := msg.(tenantsLoadedMsg); ok {
		return m.handleTenantsLoaded(tm)
	}
	if tm, ok := msg.(tenantDeletedMsg); ok {
		return m.handleTenantDeleted(tm)
	}

	// Global notifications.
	if nm, ok := msg.(notifyMsg); ok {
		return m.pushNotification(nm.notification), nil
	}

	// Completed audit runs.
	if rc, ok := msg.(runCompleteMsg); ok {
		return m.handleRunComplete(rc)
	}

	// Sizing messages are forwarded to the active nested view as the content
	// region only; the root keeps the full terminal dimensions for itself.
	if wm, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = wm.Width
		m.height = wm.Height
		m.footer, _ = m.footer.Update(wm)
		m.markViewSized()
		msg = tea.WindowSizeMsg{Width: wm.Width, Height: m.contentHeight()}
	}
	if vs, ok := msg.(viewSizeMsg); ok {
		msg = tea.WindowSizeMsg{Width: vs.width, Height: vs.height}
	}

	switch m.nav.Active() {
	case DashboardView:
		var cmd tea.Cmd
		m.dashboard, cmd = m.dashboard.Update(msg)
		return m, cmd
	case UrlsView:
		var cmd tea.Cmd
		m.auditUrls, cmd = m.auditUrls.Update(msg)
		return m, cmd
	case IssuesView:
		var cmd tea.Cmd
		m.auditIssues, cmd = m.auditIssues.Update(msg)
		return m, cmd
	case ConfigView:
		var cmd tea.Cmd
		m.config, cmd = m.config.Update(msg)
		return m, cmd
	case TimelineView:
		var cmd tea.Cmd
		m.timeline, cmd = m.timeline.Update(msg)
		return m, cmd
	}
	return m, nil
}

// handleAuditConfigSaved updates the header and tenant list after an editor
// persisted a configuration change for the current audit.
func (m Model) handleAuditConfigSaved(msg auditConfigSavedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m.pushNotification(Notification{
			Kind: NotificationDanger,
			Text: fmt.Sprintf("Save failed: %v", msg.err),
		}), nil
	}

	m = m.pushNotification(Notification{
		Kind: NotificationSuccess,
		Text: fmt.Sprintf("Audit '%s' updated", msg.name),
	})
	if msg.id == m.tenantID {
		m = m.setTenant(msg.id, msg.name)
	}
	return m, m.loadTenantsCmd()
}

// pushNotification records a notification in the header bar.
func (m Model) pushNotification(n Notification) Model {
	m.notifications = m.notifications.Push(n)
	return m
}

// handleRunComplete promotes the finished audit to the current tenant,
// refreshes the switcher list and navigates to the audit view.
func (m Model) handleRunComplete(msg runCompleteMsg) (tea.Model, tea.Cmd) {
	m = m.closeSwitcher()
	m.dashboard = m.dashboard.finishRun()

	var notification Notification
	switch {
	case msg.err != nil:
		notification = Notification{Kind: NotificationDanger, Text: fmt.Sprintf("Audit failed: %v", msg.err)}
	case msg.audit != nil:
		m = m.setTenant(msg.audit.ID, msg.audit.Name)
		m.dashboard = m.dashboard.showTenant(msg.audit)
		notification = Notification{
			Kind: NotificationSuccess,
			Text: fmt.Sprintf("Audit '%s' finished: %d endpoints in %v",
				msg.audit.Name, len(msg.audit.Urls), msg.audit.Duration.Round(time.Millisecond).String()),
		}
	default:
		notification = Notification{Kind: NotificationSuccess, Text: fmt.Sprintf("Audit '%s' finished", msg.title)}
	}
	m = m.pushNotification(notification)

	m.nav = m.nav.Select(msg.target)

	// Refresh the dashboard data so its previous-run deltas compare the
	// fresh run against the one before it.
	cmds := []tea.Cmd{m.loadTenantsCmd(), m.activateCmd()}
	if msg.audit != nil {
		cmds = append(cmds, m.dashboard.loadCmd(msg.audit.ID))
	}
	return m, tea.Batch(cmds...)
}

// helpText picks the footer help: the switcher help while its overlay is
// open, otherwise the active view's help.
func (m Model) helpText() string {
	if m.switcherOpen {
		return m.switcherHelp()
	}
	return m.activeViewHelp()
}

func (m Model) activeViewHelp() string {
	switch m.nav.Active() {
	case DashboardView:
		return m.dashboard.Help()
	case UrlsView:
		return m.auditUrls.Help()
	case IssuesView:
		return m.auditIssues.Help()
	case ConfigView:
		return m.config.Help()
	case TimelineView:
		return m.timeline.Help()
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

// headerLine renders the header bar: navigation pills on the left, the
// current tenant and any active notification on the right, stretched edge to
// edge.
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

	// The tenant pill is reserved room on the right hand side; the
	// notification sits next to it when there is space left.
	tenantBudget := m.width - leftW - chromePadding*3
	if tenantBudget > 8 {
		tenantBudget -= 8
	} else {
		tenantBudget = 0
	}
	tenant := m.tenantPill(tenantBudget)
	tenantW := lipgloss.Width(tenant)

	notification := ""
	if m.notifications.IsActive() {
		budget := m.width - leftW - tenantW - chromePadding*3
		if budget > 10 {
			notification = m.notifications.View(budget)
		}
	}
	notifW := lipgloss.Width(notification)

	fill := m.width - leftW - tenantW - notifW - chromePadding
	gaps := 0
	if tenant != "" {
		gaps++
	}
	if notification != "" {
		gaps++
	}
	fill -= gaps
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
	if tenant != "" {
		b.WriteString(m.chromeSpaces(1))
		b.WriteString(tenant)
	}
	if notification != "" {
		b.WriteString(m.chromeSpaces(1))
		b.WriteString(notification)
	}
	b.WriteString(m.chromeSpaces(chromePadding))
	return b.String()
}

// tenantPill renders the current tenant (audit) for the header bar, clipped
// to the given budget. An empty pill is returned when there is no room.
func (m Model) tenantPill(budget int) string {
	if budget < 12 {
		return ""
	}
	name := m.tenantName
	if name == "" {
		name = "none"
	}
	maxName := budget - lipgloss.Width(tenantLabel)
	if maxName < 1 {
		maxName = 1
	}
	name = clipCell(name, maxName)
	return tenantLabel + tenantNameStyle.Render(name)
}

func (m Model) View() string {
	var b strings.Builder

	b.WriteString(m.headerLine())
	b.WriteString("\n")

	content := fillLines(m.activeViewContent(), m.contentHeight())
	view := strings.Join(content, "\n")
	if m.switcherOpen {
		view = overlay(view, m.switcherView(), m.width, m.contentHeight())
	}
	b.WriteString(view)

	b.WriteString("\n")

	right := metricsText(m.eventTotal())
	b.WriteString(m.footer.WithContent(m.helpText(), right).View())

	return b.String()
}

func (m Model) activeViewContent() string {
	switch m.nav.Active() {
	case DashboardView:
		return m.dashboard.View()
	case UrlsView:
		return m.auditUrls.View()
	case IssuesView:
		return m.auditIssues.View()
	case ConfigView:
		return m.config.View()
	case TimelineView:
		return m.timeline.View()
	}
	return ""
}

// eventTotal returns the elapsed time of the current event cycle: from the
// originating user event through every background command it spawned. While
// commands are still running the total keeps growing; after the cycle
// settled (or within the tail window of its last message) the total is
// measured at the closing render, so it includes View time. Renders only
// happen right after a message, so a settled cycle is never shown stale.
func (m Model) eventTotal() time.Duration {
	if m.eventStart.IsZero() {
		return 0
	}
	if m.cycleActive || time.Since(m.lastQuietAt) <= cycleTailWindow {
		return time.Since(m.eventStart)
	}
	return 0
}

// metricsText reports the current heap usage and the time the last event
// took end to end (update + background commands + render), for the right
// side of the footer.
func metricsText(total time.Duration) string {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	mem := fmt.Sprintf("%.1f MiB", float64(ms.Alloc)/(1<<20))
	if ms.Alloc < 1<<20 {
		mem = fmt.Sprintf("%.0f KiB", float64(ms.Alloc)/(1<<10))
	}

	eventMS := float64(total) / float64(time.Millisecond)
	if eventMS <= 0 {
		return fmt.Sprintf("%s  •  –", mem)
	}
	if eventMS < 1 {
		return fmt.Sprintf("%s  •  %.1f ms/event", mem, eventMS)
	}
	return fmt.Sprintf("%s  •  %.0f ms/event", mem, eventMS)
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
