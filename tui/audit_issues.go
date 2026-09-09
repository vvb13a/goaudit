package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/vvb13a/goaudit/domain"
	"github.com/vvb13a/goaudit/service"
)

// issuesPageSize is how many issue rows one page of the issues tab holds.
// Browsing an audit never keeps more than a page resident: jumping loads the
// requested page from the store and drops the previous one.
const issuesPageSize = 250

// issuesPageLoadedMsg carries one page of issues of the current audit
// (tenant) together with the severity distribution of the whole filtered
// list. auditID, offset and showAll guard against stale responses.
type issuesPageLoadedMsg struct {
	auditID     string
	offset      int
	showAll     bool
	issues      []*domain.Issue
	counts      domain.SeverityCounts
	minSeverity domain.Severity
	err         error
}

// AuditIssuesModel lists the stored issues of the current audit (tenant) in
// a 4/5 list, with the details of the selected issue in a 1/5 pane on the
// right. Every row is one check result of the page it was checked against:
// the URL column is the issue's own page URL. Issues are browsed in pages of
// issuesPageSize rows that load on demand, so only one page is ever
// resident. By default only real issues are shown — severity notice and
// worse per the audit's "min_issue_severity" config — with a toggle to show
// everything. A severity stat widget spans the top of the tab.
type AuditIssuesModel struct {
	deps    Deps
	auditID string
	loaded  bool
	issues  []*domain.Issue
	table   table.Model

	minSeverity domain.Severity
	showAll     bool
	offset      int
	total       int
	counts      domain.SeverityCounts

	// cursorTo and cursorToLast steer the table cursor of the page that is
	// loading: crossing into a page from above lands on its last row, all
	// other jumps land on its first row.
	cursorTo     int
	cursorToLast bool

	width  int
	height int
}

func NewAuditIssuesModel(deps Deps) AuditIssuesModel {
	return AuditIssuesModel{
		deps:     deps,
		table:    table.New(),
		cursorTo: -1,
	}
}

// NavigationEnabled lets the tab leave and re-enter freely (list only).
func (m AuditIssuesModel) NavigationEnabled() bool {
	return true
}

// Track points the tab at the given audit and resets it so the next
// activation reloads its first page.
func (m AuditIssuesModel) Track(id string) AuditIssuesModel {
	if m.auditID != id {
		m = m.release()
	}
	m.auditID = id
	return m
}

// release drops the loaded page and counts so the memory is returned before
// the tab sits in the background. The next activation reloads them.
func (m AuditIssuesModel) release() AuditIssuesModel {
	m.loaded = false
	m.issues = nil
	m.offset = 0
	m.total = 0
	m.counts = domain.SeverityCounts{}
	m.showAll = false
	m.cursorTo = -1
	m.cursorToLast = false
	m.table = table.New()
	return m
}

// pageCount returns the number of pages of the filtered issue list.
func (m AuditIssuesModel) pageCount() int {
	if m.total <= 0 {
		return 1
	}
	return (m.total-1)/issuesPageSize + 1
}

// clampOffset snaps a requested offset to the start of a page within the
// filtered list.
func (m AuditIssuesModel) clampOffset(offset int) int {
	if m.total <= 0 {
		return 0
	}
	if offset < 0 {
		offset = 0
	}
	last := ((m.total - 1) / issuesPageSize) * issuesPageSize
	if offset > last {
		return last
	}
	return offset
}

// loadCmd loads the page at the given offset for the current filter state.
// The returned message carries the stored config threshold, the severity
// distribution of the whole filtered list and the page rows.
func (m AuditIssuesModel) loadCmd(offset int) tea.Cmd {
	offset = m.clampOffset(offset)
	return func() tea.Msg {
		cfg, err := m.deps.AuditService.GetConfig(context.Background(), m.auditID)
		if err != nil {
			return issuesPageLoadedMsg{auditID: m.auditID, offset: offset, showAll: m.showAll, err: err}
		}
		minSeverity := minIssueSeverity(cfg)
		counts, err := m.deps.AuditService.IssueDistribution(context.Background(), m.auditID, minSeverity, m.showAll)
		if err != nil {
			return issuesPageLoadedMsg{auditID: m.auditID, offset: offset, showAll: m.showAll, err: err}
		}
		issues, err := m.deps.AuditService.ListIssuesPage(context.Background(), m.auditID, minSeverity, m.showAll, issuesPageSize, offset)
		if err != nil {
			return issuesPageLoadedMsg{auditID: m.auditID, offset: offset, showAll: m.showAll, err: err}
		}
		return issuesPageLoadedMsg{
			auditID:     m.auditID,
			offset:      offset,
			showAll:     m.showAll,
			issues:      issues,
			counts:      counts,
			minSeverity: minSeverity,
		}
	}
}

func (m AuditIssuesModel) Update(msg tea.Msg) (AuditIssuesModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.rebuildTable()
		return m, nil

	case issuesPageLoadedMsg:
		if msg.auditID != m.auditID || msg.offset != m.offset || msg.showAll != m.showAll {
			return m, nil
		}
		m.loaded = true
		if msg.err != nil {
			return m, NotifyDanger(fmt.Sprintf("Failed to load issues: %v", msg.err))
		}
		m.minSeverity = msg.minSeverity
		m.issues = msg.issues
		m.counts = msg.counts
		m.total = msg.counts.Total()
		m.offset = m.clampOffset(m.offset)
		m.rebuildTable()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q":
			return m, tea.Quit
		case "a":
			m.showAll = !m.showAll
			m.cursorTo, m.cursorToLast = 0, false
			m.offset = 0
			return m.jumpToPage(m.offset)
		case "o":
			if iss := m.selIssue(); iss != nil && iss.URL != "" {
				return m, openInBrowserCmd(iss.URL)
			}
			return m, nil
		case "r":
			// Re-run the checks of the audited URL that reached the page of
			// the selected issue, without a full audit run: no snapshot is
			// recorded.
			if iss := m.selIssue(); iss != nil && iss.URL != "" {
				return m, recheckPageCmd(m.deps, m.auditID, iss.URL)
			}
			return m, nil
		case "n", "pgdown":
			if m.offset+issuesPageSize < m.total {
				return m.jumpToPage(m.offset + issuesPageSize)
			}
			return m, nil
		case "p", "pgup":
			if m.offset > 0 {
				return m.jumpToPage(m.offset - issuesPageSize)
			}
			return m, nil
		case "g":
			if m.offset > 0 {
				return m.jumpToPage(0)
			}
			return m, nil
		case "G":
			last := m.clampOffset(m.total)
			if m.offset == last {
				return m, nil
			}
			m.cursorTo = -1
			m.cursorToLast = true
			m.offset = last
			return m, m.loadCmd(m.offset)
		case "down", "j":
			// Crossing the last row of a page moves into the next one, so
			// scrolling feels continuous while only one page is resident.
			if cur := m.table.Cursor(); len(m.issues) > 0 && cur == len(m.issues)-1 && m.offset+len(m.issues) < m.total {
				return m.jumpToPage(m.offset + issuesPageSize)
			}
		case "up", "k":
			if cur := m.table.Cursor(); cur == 0 && m.offset > 0 {
				m.cursorTo = -1
				m.cursorToLast = true
				return m, m.loadCmd(m.offset - issuesPageSize)
			}
		}
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// jumpToPage moves the list to the start of the given page and loads it,
// keeping the previous page visible until the new one arrives.
func (m AuditIssuesModel) jumpToPage(offset int) (AuditIssuesModel, tea.Cmd) {
	m.cursorTo, m.cursorToLast = 0, false
	m.offset = m.clampOffset(offset)
	return m, m.loadCmd(m.offset)
}

// selIssue returns the issue under the table cursor.
func (m AuditIssuesModel) selIssue() *domain.Issue {
	idx := m.table.Cursor()
	if idx < 0 || idx >= len(m.issues) {
		return nil
	}
	return m.issues[idx]
}

// ---- Rendering ----

func (m AuditIssuesModel) View() string {
	if m.auditID == "" {
		return centerLines(helpStyle.Render("No audit selected. Press Ctrl+O to open the audit switcher."), m.width)
	}
	if !m.loaded {
		return "Loading issues..."
	}
	if m.total == 0 {
		if m.showAll {
			return centerLines(helpStyle.Render("No issues for this audit."), m.width)
		}
		return centerLines(helpStyle.Render("No issues at or above "+string(m.minSeverity)+". Press 'a' to show all."), m.width)
	}

	var b strings.Builder
	if widget := m.severityWidget(); widget != "" {
		b.WriteString(widget)
		b.WriteString("\n\n")
	}
	if status := m.pageStatus(); status != "" {
		b.WriteString(status)
		b.WriteString("\n\n")
	}
	b.WriteString(m.splitView())
	return b.String()
}

// pageStatus renders the position of the current page inside the whole
// filtered list.
func (m AuditIssuesModel) pageStatus() string {
	if m.width < 60 {
		return ""
	}
	first := m.offset + 1
	last := m.offset + len(m.issues)
	label := fmt.Sprintf("Rows %d–%d of %d", first, last, m.total)
	if m.pageCount() > 1 {
		label += fmt.Sprintf("  •  Page %d/%d", m.offset/issuesPageSize+1, m.pageCount())
	}
	return helpStyle.Render(label)
}

// leadLines returns how many lines the widget and page status occupy above
// the split panes. The rendered view and the table geometry must agree on
// this so the boxed panes exactly fill the remaining height.
func (m AuditIssuesModel) leadLines() int {
	lines := 0
	if m.width >= 60 {
		if m.loaded && m.total > 0 {
			lines = 5 + 2 // severity widget rows + blank, status line + blank
		} else {
			lines = 5 // severity widget rows + blank
		}
	}
	return lines
}

// splitView renders the issues table on the left (4/5 of the width) and the
// details of the selected issue on the right (1/5). On narrow terminals the
// detail pane is dropped and the table spans the full width.
func (m AuditIssuesModel) splitView() string {
	split := m.width >= 110
	leftTotal := m.width
	if split {
		leftTotal = m.width * 4 / 5
	}
	rightTotal := m.width - leftTotal

	boxH := m.height - m.leadLines()
	if boxH < 3 {
		boxH = 3
	}
	innerRows := boxH - 2

	leftInner := leftTotal - 2
	if leftInner < 10 {
		leftInner = 10
	}

	left := m.paneView(m.table.View(), leftInner, innerRows)
	leftBox := paneBox(left, leftTotal, boxH, true)

	if !split {
		return leftBox
	}

	rightInner := rightTotal - 2
	if rightInner < 8 {
		rightInner = 8
	}
	right := m.detailPaneLines(rightInner, innerRows)
	rightBox := paneBox(right, rightTotal, boxH, false)

	leftLines := strings.Split(leftBox, "\n")
	rightLines := strings.Split(rightBox, "\n")

	var b strings.Builder
	for i := 0; i < boxH; i++ {
		b.WriteString(leftLines[i])
		b.WriteString(rightLines[i])
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// paneView fills pane content (already sized by the tables) out to exactly
// height lines of width w.
func (m AuditIssuesModel) paneView(content string, w, height int) string {
	lines := strings.Split(content, "\n")
	for len(lines) < height {
		lines = append(lines, "")
	}
	for i := 0; i < height; i++ {
		lines[i] = clipToWidth(lines[i], w)
	}
	return strings.Join(lines, "\n")
}

// detailPaneLines renders the details of the selected issue wrapped and
// padded to exactly height lines of the given width.
func (m AuditIssuesModel) detailPaneLines(innerW, height int) string {
	iss := m.selIssue()
	if iss == nil {
		return ""
	}
	lines := m.detailText(iss, innerW)
	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	for i := range lines {
		lines[i] = clipToWidth(lines[i], innerW)
	}
	return strings.Join(lines, "\n")
}

// detailText renders the fields of one issue, soft-wrapped to the width.
func (m AuditIssuesModel) detailText(iss *domain.Issue, width int) []string {
	var out []string
	add := func(s string) { out = append(out, s) }

	sevColor := issueSeverityColor(iss.Severity)
	sevLine := lipgloss.NewStyle().Bold(true).Foreground(sevColor).
		Render(fmt.Sprintf("%s (%s)", strings.ToUpper(string(iss.Severity)), string(iss.Severity)))

	add(titleStyle.Render(iss.CheckName))
	add(sevLine)
	add("")
	add("Lifecycle: " + lifecycleLabel(iss.Lifecycle))
	add("Category:  " + iss.Category.DisplayName())
	if iss.PriorSeverity != "" {
		add(fmt.Sprintf("Prior:     %s", iss.PriorSeverity))
	}
	add("URL:       " + iss.URL)
	add("")
	add("Message:")
	for _, l := range strings.Split(wrapText(iss.Message, width), "\n") {
		add(l)
	}
	if len(iss.Details) > 0 {
		add("")
		add("Details & Evidence:")
		var buf strings.Builder
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(iss.Details); err == nil {
			raw := strings.TrimRight(buf.String(), "\n")
			for _, l := range strings.Split(wrapText(raw, width), "\n") {
				add(l)
			}
		}
	}
	if !iss.CreatedAt.IsZero() {
		add("")
		add("First seen: " + iss.CreatedAt.Format("2006-01-02 15:04"))
	}
	return out
}

// widgetMetric is one stat box of a summary widget: a label above a colored
// value.
type widgetMetric struct {
	label string
	value string
	color lipgloss.Color
}

// metricsWidget renders one stat box per metric across the full width, four
// lines tall: top border, label, value and bottom border. Boxes share the
// width equally with one-column gaps. It returns an empty string when the
// width cannot fit the boxes.
func metricsWidget(metrics []widgetMetric, width int) string {
	if len(metrics) == 0 {
		return ""
	}
	gap := 1
	usable := width - gap*(len(metrics)-1)
	if usable < len(metrics)*4 {
		return ""
	}
	base := usable / len(metrics)
	rem := usable % len(metrics)

	rows := make([]strings.Builder, 4)
	border := lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89"))
	numStyle := lipgloss.NewStyle().Bold(true)
	labelStyleRow := lipgloss.NewStyle().Foreground(lipgloss.Color("#9aa4b2")).Bold(true)

	for i, mt := range metrics {
		w := base
		if i < rem {
			w++
		}
		inner := w - 2
		if inner < 3 {
			inner = 3
		}

		top := border.Render("┌" + strings.Repeat("─", inner) + "┐")
		bottom := border.Render("└" + strings.Repeat("─", inner) + "┘")

		label := labelStyleRow.Render(clipCell(mt.label, inner))
		value := numStyle.Foreground(mt.color).Render(clipCell(mt.value, inner))

		rows[0].WriteString(top)
		rows[1].WriteString(centerCell(label, inner))
		rows[2].WriteString(centerCell(value, inner))
		rows[3].WriteString(bottom)

		if i < len(metrics)-1 {
			for r := 0; r < 4; r++ {
				rows[r].WriteString(" ")
			}
		}
	}

	out := make([]string, 4)
	for r := 0; r < 4; r++ {
		out[r] = clipToWidth(rows[r].String(), width)
	}
	return strings.Join(out, "\n")
}

// severityWidget renders one stat box per severity type across the full
// width, counting the issues of the current filter.
func (m AuditIssuesModel) severityWidget() string {
	const minWidgetWidth = 60
	if m.width < minWidgetWidth {
		return ""
	}

	type box struct {
		sev   domain.Severity
		label string
	}
	boxes := []box{
		{domain.SeverityFatal, "FATAL"},
		{domain.SeverityError, "ERROR"},
		{domain.SeverityWarning, "WARN"},
		{domain.SeverityNotice, "NOTICE"},
		{domain.SeverityInfo, "INFO"},
		{domain.SeveritySuccess, "PASS"},
	}

	metrics := make([]widgetMetric, 0, len(boxes))
	for _, bx := range boxes {
		count := 0
		switch bx.sev {
		case domain.SeverityFatal:
			count = m.counts.Fatal
		case domain.SeverityError:
			count = m.counts.Error
		case domain.SeverityWarning:
			count = m.counts.Warning
		case domain.SeverityNotice:
			count = m.counts.Notice
		case domain.SeverityInfo:
			count = m.counts.Info
		case domain.SeveritySuccess:
			count = m.counts.Success
		}
		metrics = append(metrics, widgetMetric{
			label: bx.label,
			value: fmt.Sprintf("%d", count),
			color: issueSeverityColor(bx.sev),
		})
	}
	return metricsWidget(metrics, m.width)
}

// centerCell centers a (possibly styled) value inside a box content row
// framed by borders.
func centerCell(value string, inner int) string {
	glyphs := lipgloss.Width(value)
	pad := (inner - glyphs) / 2
	if pad < 0 {
		pad = 0
	}
	right := inner - pad - glyphs
	if right < 0 {
		right = 0
	}
	return "│" + strings.Repeat(" ", pad) + value + strings.Repeat(" ", right) + "│"
}

// minIssueSeverity resolves the tab's filter threshold from the audit's
// stored config, falling back to notice.
func minIssueSeverity(a *domain.Audit) domain.Severity {
	cfg := service.MergeConfig(*service.DefaultConfig(), a.Config)
	sev, err := domain.ParseSeverity(cfg.MinIssueSeverity)
	if err != nil {
		return domain.SeverityNotice
	}
	return sev
}

func (m *AuditIssuesModel) rebuildTable() {
	visible := m.issues

	split := m.width >= 110
	leftTotal := m.width
	if split {
		leftTotal = m.width * 4 / 5
	}
	w := leftTotal - 2
	if w < 30 {
		w = 30
	}
	sevW := 6
	lcW := 11
	checkW := 13
	catW := 15
	urlW := w * 28 / 100
	if urlW < 20 {
		urlW = 20
	}
	if urlW > 60 {
		urlW = 60
	}
	msgW := w - sevW - lcW - checkW - catW - urlW - 8
	if msgW < 8 {
		urlW = 26
		catW = 12
		msgW = w - sevW - lcW - checkW - catW - urlW - 8
	}
	if msgW < 8 {
		msgW = 8
	}
	columns := []table.Column{
		{Title: "Severity", Width: sevW},
		{Title: "Lifecycle", Width: lcW},
		{Title: "Check", Width: checkW},
		{Title: "Category", Width: catW},
		{Title: "URL", Width: urlW},
		{Title: "Issue", Width: msgW},
	}

	rows := make([]table.Row, 0, len(visible))

	// When every listed issue's page shares one host, show only the path so
	// the repeated domain does not eat the URL column. The host is derived
	// from the loaded page, so it is consistent within the page.
	urls := make([]string, 0, len(visible))
	for _, iss := range visible {
		urls = append(urls, iss.URL)
	}
	host, shared := sharedHost(urls)

	for _, iss := range visible {
		rows = append(rows, table.Row{
			string(iss.Severity),
			lifecycleLabel(iss.Lifecycle),
			clipCell(iss.CheckName, checkW-1),
			clipCell(iss.Category.DisplayName(), catW-1),
			clipCell(displayURL(iss.URL, host, shared), urlW-1),
			clipCell(iss.Message, msgW-1),
		})
	}

	height := m.contentRows()

	cursor := m.table.Cursor()
	switch {
	case m.cursorToLast && len(rows) > 0:
		cursor = len(rows) - 1
	case m.cursorTo >= 0:
		cursor = m.cursorTo
	default:
		if cursor >= len(rows) {
			cursor = len(rows) - 1
		}
	}
	if cursor < 0 {
		cursor = 0
	}
	m.cursorTo = -1
	m.cursorToLast = false

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(height),
	)
	t.SetStyles(tableStyle())
	t.SetWidth(w)
	t.SetCursor(cursor)
	m.table = t
}

// contentRows is the number of rows available inside the split panes: the
// terminal height minus the severity widget and page status (when shown)
// minus the two box border rows.
func (m AuditIssuesModel) contentRows() int {
	h := m.height - m.leadLines() - 2
	if m.height <= 0 {
		return 10
	}
	if h < 1 {
		return 1
	}
	return h
}

// lifecycleLabel renders a readable lifecycle value for a row or detail.
func lifecycleLabel(lc domain.IssueLifecycle) string {
	if lc == "" {
		return "-"
	}
	return string(lc)
}

func (m AuditIssuesModel) Help() string {
	if m.auditID == "" {
		return "Ctrl+O: Audits"
	}
	if m.pageCount() > 1 {
		return "↑/↓: Issue  •  n/p: Page  •  g/G: First/Last  •  r: Recheck URL  •  a: Toggle All  •  o: Open URL  •  Ctrl+O: Audits  •  q: Quit"
	}
	return "↑/↓: Issue  •  r: Recheck URL  •  a: Toggle All  •  o: Open URL  •  Ctrl+O: Audits  •  q: Quit"
}
