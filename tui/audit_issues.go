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

// auditIssuesLoadedMsg carries the fully hydrated audit whose issues are
// shown in the issues tab. id guards against stale responses.
type auditIssuesLoadedMsg struct {
	id    string
	audit *domain.Audit
	err   error
}

// issueRow flattens one issue of the audit together with the URL of the
// report (page) it belongs to.
type issueRow struct {
	url   string
	issue domain.Issue
}

// AuditIssuesModel lists every issue of the current audit (tenant) across
// all its URLs in a 4/5 list, with the details of the selected issue in a 1/5
// pane on the right. By default only real issues are shown — severity notice
// and worse per the audit's "min_issue_severity" config — with a toggle to
// show everything. A severity stat widget spans the top of the tab.
type AuditIssuesModel struct {
	deps    Deps
	auditID string
	loaded  bool
	audit   *domain.Audit
	rows    []issueRow
	visible []issueRow
	table   table.Model

	minSeverity domain.Severity
	showAll     bool
	width       int
	height      int
}

func NewAuditIssuesModel(deps Deps) AuditIssuesModel {
	return AuditIssuesModel{
		deps:  deps,
		table: table.New(),
	}
}

// NavigationEnabled lets the tab leave and re-enter freely (list only).
func (m AuditIssuesModel) NavigationEnabled() bool {
	return true
}

// Track points the tab at the given audit and marks it stale so the next
// activation reloads its issues.
func (m AuditIssuesModel) Track(id string) AuditIssuesModel {
	if m.auditID != id {
		m.loaded = false
		m.audit = nil
		m.rows = nil
		m.visible = nil
	}
	m.auditID = id
	return m
}

func (m AuditIssuesModel) loadCmd(id string) tea.Cmd {
	return func() tea.Msg {
		audit, err := m.deps.AuditService.GetByID(context.Background(), id)
		return auditIssuesLoadedMsg{id: id, audit: audit, err: err}
	}
}

func (m AuditIssuesModel) Update(msg tea.Msg) (AuditIssuesModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.rebuildTable()
		return m, nil

	case auditIssuesLoadedMsg:
		if msg.id != m.auditID {
			return m, nil
		}
		m.loaded = true
		if msg.err != nil {
			return m, NotifyDanger(fmt.Sprintf("Failed to load issues: %v", msg.err))
		}
		m.audit = msg.audit
		m.minSeverity = minIssueSeverity(msg.audit)
		m.showAll = false
		m.rows = flattenIssues(msg.audit)
		m.rebuildTable()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q":
			return m, tea.Quit
		case "a":
			m.showAll = !m.showAll
			m.rebuildTable()
			return m, nil
		case "o":
			if row := m.selRow(); row != nil {
				url := row.issue.URL
				if url == "" {
					url = row.url
				}
				if url != "" {
					return m, openInBrowserCmd(url)
				}
			}
			return m, nil
		case "r":
			// Re-run the checks of the audited URL of the selected issue
			// without a full audit run: no snapshot is recorded.
			if row := m.selRow(); row != nil && row.url != "" {
				return m, recheckURLCmd(m.deps, m.auditID, row.url)
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
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

// flattenIssues collects every issue of the audit with the URL of the report
// it belongs to.
func flattenIssues(a *domain.Audit) []issueRow {
	var rows []issueRow
	for _, rep := range a.Urls {
		for _, iss := range rep.Issues {
			rows = append(rows, issueRow{url: rep.URL, issue: iss})
		}
	}
	return rows
}

// selRow returns the issue under the table cursor.
func (m AuditIssuesModel) selRow() *issueRow {
	idx := m.table.Cursor()
	if idx < 0 || idx >= len(m.visible) {
		return nil
	}
	return &m.visible[idx]
}

// ---- Rendering ----

func (m AuditIssuesModel) View() string {
	if m.auditID == "" {
		return centerLines(helpStyle.Render("No audit selected. Press Ctrl+O to open the audit switcher."), m.width)
	}
	if !m.loaded {
		return "Loading issues..."
	}
	if len(m.rows) == 0 {
		return centerLines(helpStyle.Render("No issues for this audit."), m.width)
	}

	var b strings.Builder
	if widget := m.severityWidget(); widget != "" {
		b.WriteString(widget)
		b.WriteString("\n\n")
	}
	b.WriteString(m.splitView())
	return b.String()
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

	extra := 0
	if m.width >= 60 {
		extra = 5 // 4 widget rows + one blank line
	}
	boxH := m.height - extra
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
	row := m.selRow()
	if row == nil {
		return ""
	}
	lines := m.detailText(row, innerW)
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
func (m AuditIssuesModel) detailText(row *issueRow, width int) []string {
	iss := &row.issue
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
	pageURL := iss.URL
	if pageURL == "" {
		pageURL = row.url
	}
	add("URL:       " + pageURL)
	if row.url != "" && row.url != pageURL {
		add("Target:    " + row.url)
	}
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
// width, counting the issues currently listed by the table.
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

	counts := make(map[domain.Severity]int)
	for _, row := range m.visible {
		counts[row.issue.Severity]++
	}

	metrics := make([]widgetMetric, 0, len(boxes))
	for _, bx := range boxes {
		metrics = append(metrics, widgetMetric{
			label: bx.label,
			value: fmt.Sprintf("%d", counts[bx.sev]),
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

// visibleRows applies the severity filter: without showAll only issues at or
// above the configured minimum severity are listed.
func (m AuditIssuesModel) visibleRows() []issueRow {
	if m.showAll || m.minSeverity == "" {
		return m.rows
	}
	minW := m.minSeverity.Weight()
	var out []issueRow
	for _, row := range m.rows {
		if row.issue.Severity.Weight() >= minW {
			out = append(out, row)
		}
	}
	return out
}

func (m *AuditIssuesModel) rebuildTable() {
	m.visible = m.visibleRows()
	visible := m.visible

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

	// When every listed URL shares one host, show only the path so the
	// repeated domain does not eat the URL column.
	urls := make([]string, 0, len(visible))
	for _, row := range visible {
		urls = append(urls, row.url)
	}
	host, shared := sharedHost(urls)

	for _, row := range visible {
		rows = append(rows, table.Row{
			string(row.issue.Severity),
			lifecycleLabel(row.issue.Lifecycle),
			clipCell(row.issue.CheckName, checkW-1),
			clipCell(row.issue.Category.DisplayName(), catW-1),
			clipCell(displayURL(row.url, host, shared), urlW-1),
			clipCell(row.issue.Message, msgW-1),
		})
	}

	height := m.contentRows()
	cursor := m.table.Cursor()
	if cursor >= len(rows) {
		cursor = len(rows) - 1
	}
	if cursor < 0 {
		cursor = 0
	}

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
// terminal height minus the severity widget block (when wide enough) minus
// the two box border rows.
func (m AuditIssuesModel) contentRows() int {
	extra := 0
	if m.width >= 60 {
		extra = 5 // 4 widget rows + one blank line
	}
	h := m.height - extra - 2
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
	return "↑/↓: Issue  •  r: Recheck URL  •  a: Toggle All  •  o: Open URL  •  Ctrl+O: Audits  •  q: Quit"
}
