package tui

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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
	AuditsView ViewID = iota
	HistoryView
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
	audits        AuditsModel
	history       HistoryModel
	width         int
	height        int

	// Event cycle timing for the footer metrics: eventStart marks the
	// beginning of the current (or most recent) user event, cycleActive
	// stays true while background commands spawned by it are still running,
	// and lastQuietAt is when the last cycle settled.
	eventStart  time.Time
	cycleActive bool
	lastQuietAt time.Time

	auditsSized  bool
	historySized bool
}

func New(deps Deps) Model {
	return Model{
		deps: deps,
		nav: NewNavModel([]Tab{
			{ID: AuditsView, Label: "Audits"},
			{ID: HistoryView, Label: "History"},
		}),
		footer:        NewFooterModel(),
		notifications: NewNotificationModel(),
		audits:        NewAuditsModel(deps),
		history:       NewHistoryModel(),
	}
}

// initCmdForActiveView kicks off the initial data load for the active view,
// so a view only loads once it is first shown.
func (m Model) initCmdForActiveView() tea.Cmd {
	switch m.nav.Active() {
	case AuditsView:
		if !m.audits.Loaded() {
			return m.audits.Init()
		}
	case HistoryView:
		// The session history has no data to load.
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

// activateCmd returns the commands to run when a view is activated: its
// initial data load (once) and, if it has not been sized yet, a replay of the
// current content dimensions.
func (m Model) activateCmd() tea.Cmd {
	var cmds []tea.Cmd

	if c := m.initCmdForActiveView(); c != nil {
		cmds = append(cmds, c)
	}

	sized := false
	switch m.nav.Active() {
	case AuditsView:
		sized = m.auditsSized
	case HistoryView:
		sized = m.historySized
	}

	if m.width > 0 && !sized {
		width, height := m.width, m.contentHeight()
		cmds = append(cmds, func() tea.Msg {
			return viewSizeMsg{width: width, height: height}
		})
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
	case AuditsView:
		m.auditsSized = true
	case HistoryView:
		m.historySized = true
	}
}

// viewIsRoot reports whether the active nested view is at its top-level list
// state, where tab navigation is allowed.
func (m Model) viewIsRoot() bool {
	switch m.nav.Active() {
	case AuditsView:
		return m.audits.NavigationEnabled()
	case HistoryView:
		return m.history.NavigationEnabled()
	}
	return false
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
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "tab":
			if m.viewIsRoot() {
				m.nav = m.nav.Next()
				return m, m.activateCmd()
			}
		case "shift+tab", "backtab":
			if m.viewIsRoot() {
				m.nav = m.nav.Prev()
				return m, m.activateCmd()
			}
		}

		if len(msg.Runes) == 1 && msg.Runes[0] >= '1' && msg.Runes[0] <= '9' {
			if m.viewIsRoot() {
				m.nav = m.nav.SelectIndex(int(msg.Runes[0]-'0') - 1)
				return m, m.activateCmd()
			}
		}
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
	case AuditsView:
		var cmd tea.Cmd
		m.audits, cmd = m.audits.Update(msg)
		return m, cmd
	case HistoryView:
		var cmd tea.Cmd
		m.history, cmd = m.history.Update(msg)
		return m, cmd
	}
	return m, nil
}

// pushNotification records a notification in the header bar and appends it
// to the session history, keeping both in sync for every notification
// source.
func (m Model) pushNotification(n Notification) Model {
	m.notifications = m.notifications.Push(n)
	m.history = m.history.Push(n)
	return m
}

func (m Model) handleRunComplete(msg runCompleteMsg) (tea.Model, tea.Cmd) {
	m.audits = m.audits.finishRun()

	var notification Notification
	switch {
	case msg.err != nil:
		notification = Notification{Kind: NotificationDanger, Text: fmt.Sprintf("Audit failed: %v", msg.err)}
	case msg.audit != nil:
		notification = Notification{
			Kind: NotificationSuccess,
			Text: fmt.Sprintf("Audit '%s' finished: %d endpoints in %v",
				msg.audit.Name, len(msg.audit.Reports), msg.audit.Duration.Round(time.Millisecond).String()),
		}
	default:
		notification = Notification{Kind: NotificationSuccess, Text: fmt.Sprintf("Audit '%s' finished", msg.title)}
	}
	m = m.pushNotification(notification)

	m.audits = m.audits.markStale()
	m.nav = m.nav.Select(msg.target)
	return m, m.activateCmd()
}

func (m Model) activeViewHelp() string {
	switch m.nav.Active() {
	case AuditsView:
		return m.audits.Help()
	case HistoryView:
		return m.history.Help()
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

	right := metricsText(m.eventTotal())
	b.WriteString(m.footer.WithContent(m.activeViewHelp(), right).View())

	return b.String()
}

func (m Model) activeViewContent() string {
	switch m.nav.Active() {
	case AuditsView:
		return m.audits.View()
	case HistoryView:
		return m.history.View()
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
