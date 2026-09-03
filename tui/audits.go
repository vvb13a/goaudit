package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/vvb13a/goaudit/domain"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type auditsLoadedMsg struct {
	audits []*domain.Audit
	err    error
}

// auditsDetailMsg carries the fully hydrated audit (reports with issues) for
// the middle and right panes. id guards against stale responses.
type auditsDetailMsg struct {
	id    string
	audit *domain.Audit
	err   error
}

type auditsState int

const (
	auditsListState auditsState = iota
	auditsDeleteState
	auditsRunningState
	auditsPromptState
)

// Panes of the split audits view.
const (
	paneAudits = iota
	paneReports
	paneIssues
)

type AuditsModel struct {
	deps         Deps
	state        auditsState
	audits       []*domain.Audit
	table        table.Model
	prompt       textinput.Model
	progress     ProgressModel
	progressCh   chan ProgressMsg
	progressDone chan struct{}
	loaded       bool
	deleteID     string
	deleteName   string
	width        int
	height       int

	// Split audits/reports/issues view.
	focusPane     int
	detailID      string
	detailLoading bool
	detailAudit   *domain.Audit
	reportIdx     int
	reportsTable  table.Model
	issuesTable   table.Model

	// Sorted issues of the currently selected report plus issue detail modal.
	reportIssues    []domain.Issue
	issueDetailOpen bool
	issueDetail     domain.Issue
	issueDetailURL  string
}

func NewAuditsModel(deps Deps) AuditsModel {
	return AuditsModel{
		deps:         deps,
		state:        auditsListState,
		table:        table.New(),
		reportsTable: table.New(),
		issuesTable:  table.New(),
		progress:     NewProgressModel(),
	}
}

func (m AuditsModel) Init() tea.Cmd {
	return m.loadCmd()
}

func (m AuditsModel) Loaded() bool {
	return m.loaded
}

func (m AuditsModel) NavigationEnabled() bool {
	return m.state == auditsListState
}

func (m AuditsModel) loadCmd() tea.Cmd {
	return func() tea.Msg {
		audits, err := m.deps.AuditService.List(context.Background(), domain.AuditFilter{})
		return auditsLoadedMsg{audits: audits, err: err}
	}
}

func (m AuditsModel) loadDetailCmd(id string) tea.Cmd {
	return func() tea.Msg {
		audit, err := m.deps.AuditService.GetByID(context.Background(), id)
		if err != nil {
			return auditsDetailMsg{id: id, err: err}
		}
		return auditsDetailMsg{id: id, audit: audit}
	}
}

// openAuditExcelCmd hydrates the full audit (reports with issues), renders
// it into an Excel workbook (reusing an existing one instead of
// regenerating) and opens the workbook with the default xlsx viewer.
func (m AuditsModel) openAuditExcelCmd(a *domain.Audit) tea.Cmd {
	return func() tea.Msg {
		if m.deps.ExcelService == nil {
			return notifyMsg{notification: Notification{
				Kind: NotificationDanger,
				Text: "Excel export is not available",
			}}
		}

		full, err := m.deps.AuditService.GetByID(context.Background(), a.ID)
		if err != nil {
			return notifyMsg{notification: Notification{
				Kind: NotificationDanger,
				Text: fmt.Sprintf("Export failed: %v", err),
			}}
		}

		path, err := m.deps.ExcelService.ExportAudit(full)
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

func (m AuditsModel) Update(msg tea.Msg) (AuditsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.rebuildAuditsTable()
		m.rebuildReportsTable()
		m.rebuildIssuesTable()
		return m, nil

	case auditsLoadedMsg:
		m.loaded = true
		if msg.err != nil {
			return m, NotifyDanger(fmt.Sprintf("Failed to load audits: %v", msg.err))
		}
		m.audits = msg.audits
		sort.SliceStable(m.audits, func(i, j int) bool {
			return m.audits[i].StartedAt.After(m.audits[j].StartedAt)
		})
		m.rebuildAuditsTable()

		if len(m.audits) == 0 {
			return m.closeDetail(), nil
		}

		// Keep showing the selected audit if it still exists, otherwise fall
		// back to the first audit of the refreshed history.
		for _, a := range m.audits {
			if a.ID == m.detailID {
				return m, nil
			}
		}
		return m.openDetail(m.audits[0].ID, paneAudits)

	case auditsDetailMsg:
		if msg.id != m.detailID {
			return m, nil
		}
		m.detailLoading = false
		if msg.err != nil {
			return m, NotifyDanger(fmt.Sprintf("Failed to load audit details: %v", msg.err))
		}
		m.detailAudit = msg.audit
		if m.reportIdx >= len(msg.audit.Reports) {
			m.reportIdx = 0
		}
		m.rebuildReportsTable()
		m.rebuildIssuesTable()
		return m, nil

	case ProgressMsg:
		if m.state != auditsRunningState {
			return m, nil
		}
		var cmd tea.Cmd
		m.progress, cmd = m.progress.Update(msg)
		return m, tea.Batch(cmd, newProgressWaitCmd(m.progressCh, m.progressDone))

	case progress.FrameMsg:
		if m.state == auditsRunningState {
			var cmd tea.Cmd
			m.progress, cmd = m.progress.Update(msg)
			return m, cmd
		}
	}

	switch m.state {
	case auditsListState:
		return m.updateSplit(msg)
	case auditsDeleteState:
		return m.updateDelete(msg)
	case auditsPromptState:
		return m.updatePrompt(msg)
	}
	return m, nil
}

// ---- Split view handling ----

// updateSplit routes keys to the focused pane. Left/right move the focus
// across the audits, reports and issues panes; the panes stay in sync with
// the selected audit/report. Audit-level actions (adhoc run, rerun, delete,
// export) are available while the audits pane is focused.
func (m AuditsModel) updateSplit(msg tea.Msg) (AuditsModel, tea.Cmd) {
	if m.issueDetailOpen {
		if key, ok := msg.(tea.KeyMsg); ok && key.String() == "esc" {
			m.issueDetailOpen = false
		}
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q":
			return m, tea.Quit
		case "n":
			if m.focusPane == paneAudits {
				ti := textinput.New()
				ti.Placeholder = "https://example.com/page"
				ti.CharLimit = 2048
				ti.Width = 60
				ti.Focus()
				m.prompt = ti
				m.state = auditsPromptState
			}
			return m, nil
		case "r":
			if m.focusPane == paneAudits {
				if sel := m.selAudit(); sel != nil {
					return m.startRerun(sel)
				}
			}
			return m, nil
		case "d", "x":
			if m.focusPane == paneAudits {
				if sel := m.selAudit(); sel != nil {
					m.deleteID = sel.ID
					m.deleteName = sel.PlanName
					m.state = auditsDeleteState
				}
			}
			return m, nil
		case "e":
			if m.focusPane == paneAudits {
				if sel := m.selAudit(); sel != nil {
					return m, m.openAuditExcelCmd(sel)
				}
			}
			return m, nil
		case "left", "h":
			if m.focusPane > paneAudits {
				m.focusPane--
			}
			return m, nil
		case "right", "l":
			if m.focusPane < paneIssues {
				m.focusPane++
				if m.focusPane == paneReports && m.detailAudit != nil {
					m.rebuildReportsTable()
				}
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	switch m.focusPane {
	case paneAudits:
		before := m.table.Cursor()
		m.table, cmd = m.table.Update(msg)
		if sel := m.selAudit(); sel != nil && m.table.Cursor() != before {
			if sel.ID != m.detailID {
				return m.openDetail(sel.ID, paneAudits)
			}
		}

	case paneReports:
		if key, ok := msg.(tea.KeyMsg); ok && key.String() == "o" {
			if rep := m.currentReport(); rep != nil && rep.URL != "" {
				return m, openInBrowserCmd(rep.URL)
			}
			return m, nil
		}
		before := m.reportsTable.Cursor()
		m.reportsTable, cmd = m.reportsTable.Update(msg)
		if m.detailAudit != nil && m.reportsTable.Cursor() != before {
			m.reportIdx = m.reportsTable.Cursor()
			m.rebuildIssuesTable()
		}

	case paneIssues:
		if key, ok := msg.(tea.KeyMsg); ok && key.String() == "enter" {
			idx := m.issuesTable.Cursor()
			if idx >= 0 && idx < len(m.reportIssues) {
				rep := m.currentReport()
				m.issueDetail = m.reportIssues[idx]
				if rep != nil {
					m.issueDetailURL = rep.URL
				}
				m.issueDetailOpen = true
			}
			return m, nil
		}
		m.issuesTable, cmd = m.issuesTable.Update(msg)
	}
	return m, cmd
}

// openDetail switches the detail panes to the given audit, loading it when
// it is not the audit currently shown.
func (m AuditsModel) openDetail(id string, focusPane int) (AuditsModel, tea.Cmd) {
	if id != m.detailID {
		m.detailID = id
		m.detailLoading = true
		m.detailAudit = nil
		m.reportIdx = 0
	}
	m.focusPane = focusPane
	m.rebuildAuditsTable()
	m.rebuildReportsTable()
	m.rebuildIssuesTable()
	if m.detailLoading {
		return m, m.loadDetailCmd(id)
	}
	return m, nil
}

// closeDetail drops the audit shown in the detail panes.
func (m AuditsModel) closeDetail() AuditsModel {
	m.detailID = ""
	m.detailLoading = false
	m.detailAudit = nil
	m.reportIdx = 0
	m.reportIssues = nil
	m.issueDetailOpen = false
	m.rebuildReportsTable()
	m.rebuildIssuesTable()
	return m
}

func (m AuditsModel) selAudit() *domain.Audit {
	idx := m.table.Cursor()
	if idx < 0 || idx >= len(m.audits) {
		return nil
	}
	return m.audits[idx]
}

func (m AuditsModel) currentReport() *domain.Report {
	if m.detailAudit == nil || m.reportIdx < 0 || m.reportIdx >= len(m.detailAudit.Reports) {
		return nil
	}
	return m.detailAudit.Reports[m.reportIdx]
}

// ---- Other states ----

func (m AuditsModel) updatePrompt(msg tea.Msg) (AuditsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.state = auditsListState
			return m, nil
		case "enter":
			raw := strings.TrimSpace(m.prompt.Value())
			if raw == "" {
				return m, NotifyDanger("Enter a URL to audit")
			}
			if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
				raw = "https://" + raw
			}
			plan := &domain.Plan{Name: "Ad-hoc Run", URLs: []string{raw}}
			return m.startRun(plan, "Ad-hoc Run")
		}
	}

	var cmd tea.Cmd
	m.prompt, cmd = m.prompt.Update(msg)
	return m, cmd
}

func (m AuditsModel) startRerun(a *domain.Audit) (AuditsModel, tea.Cmd) {
	plan := &domain.Plan{Name: a.PlanName}

	if a.PlanID != "" {
		if existing, err := m.deps.PlanService.GetByID(context.Background(), a.PlanID); err == nil {
			plan = existing
		}
	}

	if len(plan.URLs) == 0 {
		full, err := m.deps.AuditService.GetByID(context.Background(), a.ID)
		if err != nil {
			return m, NotifyDanger(fmt.Sprintf("Cannot rerun audit: %v", err))
		}
		for _, rep := range full.Reports {
			plan.URLs = append(plan.URLs, rep.URL)
		}
		if len(plan.URLs) == 0 {
			return m, NotifyDanger("Cannot rerun audit: no target URLs found")
		}
	}

	return m.startRun(plan, fmt.Sprintf("Rerunning '%s'", a.PlanName))
}

func (m AuditsModel) startRun(plan *domain.Plan, title string) (AuditsModel, tea.Cmd) {
	checklist, checks, err := prepareRun(m.deps)
	if err != nil {
		return m, NotifyDanger(err.Error())
	}

	m.state = auditsRunningState
	m.progress = m.progress.Start(title, m.width)
	m.progressCh = make(chan ProgressMsg, 16)
	m.progressDone = make(chan struct{})

	return m, tea.Batch(
		newRunCmd(m.deps, plan, checklist, checks, AuditsView, AuditsView, m.progressCh, m.progressDone),
		newProgressWaitCmd(m.progressCh, m.progressDone),
	)
}

func (m AuditsModel) finishRun() AuditsModel {
	m.state = auditsListState
	m.closeDetail()
	m.progressCh = nil
	m.progressDone = nil
	return m
}

func (m AuditsModel) markStale() AuditsModel {
	m.loaded = false
	return m
}

func (m AuditsModel) updateDelete(msg tea.Msg) (AuditsModel, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch key.String() {
	case "y", "Y":
		var notice tea.Cmd
		if err := m.deps.AuditService.Delete(context.Background(), m.deleteID); err != nil {
			notice = NotifyDanger(fmt.Sprintf("Delete failed: %v", err))
		} else {
			notice = NotifySuccess(fmt.Sprintf("Deleted audit '%s'", m.deleteName))
		}
		m.state = auditsListState
		return m, tea.Batch(m.loadCmd(), notice)
	default:
		m.deleteID = ""
		m.deleteName = ""
		m.state = auditsListState
		return m, nil
	}
}

// ---- Rendering ----

func (m AuditsModel) contentView() string {
	return m.splitView()
}

func (m AuditsModel) View() string {
	switch m.state {
	case auditsListState:
		content := m.contentView()
		if m.issueDetailOpen {
			return overlay(content, m.issueDetailView(), m.width, m.height)
		}
		return content
	case auditsDeleteState:
		return overlay(m.contentView(), m.deleteView(), m.width, m.height)
	case auditsRunningState:
		return m.progress.View()
	case auditsPromptState:
		return overlay(m.contentView(), m.promptView(), m.width, m.height)
	}
	return ""
}

func (m AuditsModel) listView() string {
	if !m.loaded {
		return "Loading audits..."
	}
	if len(m.audits) == 0 {
		return "No audits yet. Press 'n' to audit a single URL or run a plan."
	}
	return m.table.View()
}

// splitView renders three equal panes: audits, reports of the selected audit
// and the issues of the selected report, with a severity summary line on top.
func (m AuditsModel) splitView() string {
	paneW := m.width / 3
	if paneW < 20 || m.height < 5 {
		return m.listView()
	}

	boxH := m.height - 1
	innerW := paneW - 2

	leftContent := m.table.View()
	if !m.loaded {
		leftContent = "Loading audits..."
	} else if len(m.audits) == 0 {
		leftContent = "No audits yet. Press 'n' to audit a URL."
	}

	left := m.paneView(leftContent, innerW, boxH-2)
	middle := m.paneView(m.middlePaneView(), innerW, boxH-2)
	right := m.paneView(m.rightPaneView(), innerW, boxH-2)

	leftBox := paneBox(left, paneW, boxH, m.focusPane == paneAudits)
	middleBox := paneBox(middle, paneW, boxH, m.focusPane == paneReports)
	rightBox := paneBox(right, paneW, boxH, m.focusPane == paneIssues)

	leftLines := strings.Split(leftBox, "\n")
	middleLines := strings.Split(middleBox, "\n")
	rightLines := strings.Split(rightBox, "\n")

	var b strings.Builder
	b.WriteString(m.severityWidget())
	b.WriteString("\n")
	for i := 0; i < boxH; i++ {
		b.WriteString(leftLines[i])
		b.WriteString(middleLines[i])
		b.WriteString(rightLines[i])
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// paneView fills pane content (already sized by the tables) out to exactly
// height lines of width w.
func (m AuditsModel) paneView(content string, w, height int) string {
	lines := strings.Split(content, "\n")
	for len(lines) < height {
		lines = append(lines, "")
	}
	for i := 0; i < height; i++ {
		lines[i] = clipToWidth(lines[i], w)
	}
	return strings.Join(lines, "\n")
}

// paneBox frames pane content with a rounded border; the focused pane glows
// with an accent-colored border while the others stay dim.
func paneBox(content string, width, height int, focused bool) string {
	innerW := width - 2
	if innerW < 4 {
		innerW = 4
	}

	color := lipgloss.Color("#444b6a")
	style := lipgloss.NewStyle().Foreground(color)
	if focused {
		style = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7aa2f7"))
	}

	innerRows := height - 2
	if innerRows < 1 {
		innerRows = 1
	}
	lines := strings.Split(content, "\n")
	for len(lines) < innerRows {
		lines = append(lines, "")
	}

	side := style.Render("│")
	var b strings.Builder
	b.WriteString(style.Render("╭" + strings.Repeat("─", innerW) + "╮"))
	b.WriteString("\n")
	for i := 0; i < innerRows; i++ {
		b.WriteString(side)
		b.WriteString(clipToWidth(lines[i], innerW))
		b.WriteString(side)
		b.WriteString("\n")
	}
	b.WriteString(style.Render("╰" + strings.Repeat("─", innerW) + "┘"))
	return b.String()
}

// clipToWidth truncates or pads a string to the given display width.
func clipToWidth(s string, w int) string {
	pad := w - lipgloss.Width(s)
	if pad >= 0 {
		return s + strings.Repeat(" ", pad)
	}
	runes := []rune(s)
	if w <= 1 {
		return "…"
	}
	return string(runes[:w-1]) + "…"
}

// regionHeight is the number of table rows that fit in one pane below the
// severity widget (box borders take two rows).
func (m AuditsModel) regionHeight() int {
	h := m.height - 3
	if m.height <= 0 {
		return 10
	}
	if h < 1 {
		return 1
	}
	return h
}

// severityWidget renders FATAL/ERROR/WARN/NOTICE/INFO/PASS counts for the
// issues of the currently selected report.
func (m AuditsModel) severityWidget() string {
	counts := make(map[domain.Severity]int)
	for _, iss := range m.reportIssues {
		counts[iss.Severity]++
	}

	type badge struct {
		severity domain.Severity
		label    string
	}
	labels := []badge{
		{domain.SeverityFatal, "FATAL"},
		{domain.SeverityError, "ERROR"},
		{domain.SeverityWarning, "WARN"},
		{domain.SeverityNotice, "NOTICE"},
		{domain.SeverityInfo, "INFO"},
		{domain.SeveritySuccess, "PASS"},
	}

	var parts []string
	for _, b := range labels {
		style := lipgloss.NewStyle().Bold(true).Foreground(issueSeverityColor(b.severity))
		parts = append(parts, style.Render(fmt.Sprintf("%s: %d", b.label, counts[b.severity])))
	}
	var line strings.Builder
	for i, part := range parts {
		if i > 0 {
			line.WriteString("  ")
		}
		line.WriteString(part)
	}
	return line.String()
}

func issueSeverityColor(severity domain.Severity) lipgloss.Color {
	switch severity {
	case domain.SeverityFatal, domain.SeverityError:
		return lipgloss.Color("9")
	case domain.SeverityWarning:
		return lipgloss.Color("3")
	case domain.SeverityNotice:
		return lipgloss.Color("39")
	case domain.SeverityInfo:
		return lipgloss.Color("45")
	default:
		return lipgloss.Color("10")
	}
}

func (m AuditsModel) middlePaneView() string {
	if m.detailLoading {
		return "Loading reports..."
	}
	if m.detailAudit == nil {
		return "No audit selected."
	}
	if len(m.detailAudit.Reports) == 0 {
		return "No reports."
	}
	return m.reportsTable.View()
}

func (m AuditsModel) rightPaneView() string {
	if m.detailLoading || m.detailAudit == nil {
		return ""
	}
	if rep := m.currentReport(); rep == nil {
		return ""
	} else if len(rep.Issues) == 0 {
		return "No issues."
	}
	return m.issuesTable.View()
}

// issueDetailView renders the full details of the selected issue, including
// the evidence map as formatted JSON.
func (m AuditsModel) issueDetailView() string {
	issue := m.issueDetail
	var b strings.Builder

	status := "PASSED"
	if !issue.Passed {
		status = "FAILED"
	}
	b.WriteString(titleStyle.Render(fmt.Sprintf("%s — %s", issue.CheckName, string(issue.Severity))))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("Severity:  %s (%s)\n", status, issue.Severity))
	if issue.Category != "" {
		b.WriteString(fmt.Sprintf("Category:  %s\n", issue.Category.DisplayName()))
	}
	b.WriteString(fmt.Sprintf("Page:      %s\n", m.issueDetailURL))
	b.WriteString("\nMessage:\n")
	b.WriteString(wrapText(issue.Message, m.width-16))
	if len(issue.Details) > 0 {
		b.WriteString("\n\nDetails & Evidence:\n")
		var buf strings.Builder
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(issue.Details); err == nil {
			b.WriteString(wrapText(strings.TrimRight(buf.String(), "\n"), m.width-16))
		}
	}
	return b.String()
}

// wrapText soft-wraps long lines at the given width, keeping existing
// newlines.
func wrapText(s string, width int) string {
	if width < 20 {
		width = 20
	}
	var out strings.Builder
	for _, line := range strings.Split(s, "\n") {
		runes := []rune(line)
		for len(runes) > width {
			out.WriteString(string(runes[:width]))
			out.WriteString("\n")
			runes = runes[width:]
		}
		out.WriteString(string(runes))
		out.WriteString("\n")
	}
	return strings.TrimRight(out.String(), "\n")
}

func (m AuditsModel) deleteView() string {
	return fmt.Sprintf("Delete audit '%s'? This cannot be undone.", m.deleteName)
}

func (m AuditsModel) promptView() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Audit a single URL"))
	b.WriteString("\n\n")
	b.WriteString("URL: " + m.prompt.View())
	return b.String()
}

func (m AuditsModel) Help() string {
	switch m.state {
	case auditsDeleteState:
		return "y: Delete  •  any other key: Cancel"
	case auditsRunningState:
		return "Audit in progress  •  Ctrl+C: Quit"
	case auditsPromptState:
		return "Enter: Run  •  Esc: Cancel"
	case auditsListState:
		if m.issueDetailOpen {
			return "Esc: Close Details  •  q: Quit"
		}
		switch m.focusPane {
		case paneAudits:
			return "→: Reports  •  ↑/↓: Audit  •  n: URL Audit  •  r: Rerun  •  e: Open Excel  •  d: Delete  •  q: Quit"
		case paneReports:
			return "←: Audits  •  →: Issues  •  ↑/↓: Report  •  o: Open in Browser  •  q: Quit"
		default:
			return "←: Reports  •  ↑/↓: Issue  •  Enter: Details  •  q: Quit"
		}
	}
	return ""
}

// ---- Tables ----

func (m *AuditsModel) rebuildAuditsTable() {
	cursor := m.table.Cursor()
	tableWidth := m.width/3 - 2
	if tableWidth < 10 {
		tableWidth = 10
	}

	// Column widths never shrink below the length of their header label so
	// titles are never clipped. The plan column absorbs the spare width.
	sevW := 8 // "Severity"
	durW := 8 // "Duration"
	startedW := 14
	planW := tableWidth - sevW - durW - startedW
	if planW < 8 {
		// Not enough room: fall back to compact relative times ("3h ago").
		startedW = 8
		planW = tableWidth - sevW - durW - startedW
		if planW < 8 {
			// Still tight: drop the duration column entirely.
			durW = 0
			planW = tableWidth - sevW - startedW
			if planW < 8 {
				planW = 8
				startedW = tableWidth - sevW - planW
				if startedW < 5 {
					startedW = 5
				}
			}
		}
	}

	short := startedW < 14
	columns := []table.Column{
		{Title: "Plan", Width: planW},
		{Title: "Started", Width: startedW},
	}
	if durW > 0 {
		columns = append(columns, table.Column{Title: "Duration", Width: durW})
	}
	columns = append(columns, table.Column{Title: "Severity", Width: sevW})

	rows := make([]table.Row, 0, len(m.audits))
	for _, a := range m.audits {
		row := table.Row{clipCell(a.PlanName, planW-1), timeAgo(a.StartedAt, short)}
		if durW > 0 {
			row = append(row, a.Duration.Round(time.Second).String())
		}
		row = append(row, string(a.Summary.HighestSeverity))
		rows = append(rows, row)
	}

	height := m.regionHeight()

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(height),
	)
	t.SetStyles(tableStyle())
	t.SetWidth(tableWidth)
	if len(rows) > 0 {
		if cursor >= len(rows) {
			cursor = len(rows) - 1
		}
		t.SetCursor(cursor)
	}
	m.table = t
}

func (m *AuditsModel) rebuildReportsTable() {
	paneW := m.width/3 - 2
	if m.detailAudit == nil {
		m.reportsTable = table.New()
		return
	}

	resW := 11
	urlW := paneW - resW - 6
	if urlW < 10 {
		urlW = 10
	}
	columns := []table.Column{
		{Title: "Result", Width: resW},
		{Title: "URL", Width: urlW},
	}

	rows := make([]table.Row, 0, len(m.detailAudit.Reports))
	for _, rep := range m.detailAudit.Reports {
		rows = append(rows, table.Row{
			clipCell(reportResult(rep), resW-1),
			clipCell(rep.URL, urlW-1),
		})
	}

	height := m.regionHeight()

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(height),
	)
	t.SetStyles(tableStyle())
	if paneW < 8 {
		paneW = 8
	}
	t.SetWidth(paneW)
	t.SetCursor(m.reportIdx)
	m.reportsTable = t
}

func (m *AuditsModel) rebuildIssuesTable() {
	paneW := m.width/3 - 2
	rep := m.currentReport()
	if rep == nil {
		m.issuesTable = table.New()
		return
	}

	sevW := 8
	checkW := 12
	msgW := paneW - sevW - checkW - 6
	if msgW < 10 {
		checkW = 9
		msgW = paneW - sevW - checkW - 6
	}
	if msgW < 10 {
		sevW = 7
		checkW = 0
		msgW = paneW - sevW - 6
	}

	var columns []table.Column
	if checkW > 0 {
		columns = []table.Column{
			{Title: "Sev", Width: sevW},
			{Title: "Check", Width: checkW},
			{Title: "Issue", Width: msgW},
		}
	} else {
		columns = []table.Column{
			{Title: "Sev", Width: sevW},
			{Title: "Issue", Width: msgW},
		}
	}

	sorted := make([]domain.Issue, len(rep.Issues))
	copy(sorted, rep.Issues)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Severity.Weight() > sorted[j].Severity.Weight()
	})
	m.reportIssues = sorted

	rows := make([]table.Row, 0, len(sorted))
	for _, iss := range sorted {
		row := []string{string(iss.Severity), clipCell(iss.Message, msgW-1)}
		if checkW > 0 {
			row = []string{string(iss.Severity), clipCell(iss.CheckName, checkW-1), clipCell(iss.Message, msgW-1)}
		}
		rows = append(rows, row)
	}

	height := m.regionHeight()

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(height),
	)
	t.SetStyles(tableStyle())
	t.SetWidth(paneW)
	m.issuesTable = t
}

func reportResult(rep *domain.Report) string {
	if rep.Summary.FailedCount > 0 {
		return fmt.Sprintf("%d fail", rep.Summary.FailedCount)
	}
	return "pass"
}

func clipCell(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 1 {
		return "…"
	}
	return string(runes[:max-1]) + "…"
}

// timeAgo renders a timestamp relative to now as "1 minute ago",
// "10 days ago", etc. When short is set it degrades to compact forms like
// "1m ago" that fit a narrow column.
func timeAgo(t time.Time, short bool) string {
	d := time.Since(t)
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		if short {
			return "now"
		}
		return "just now"
	case d < time.Hour:
		return ago(d, time.Minute, "minute", "m", short)
	case d < 24*time.Hour:
		return ago(d, time.Hour, "hour", "h", short)
	case d < 30*24*time.Hour:
		return ago(d, 24*time.Hour, "day", "d", short)
	case d < 365*24*time.Hour:
		return ago(d, 30*24*time.Hour, "month", "mo", short)
	default:
		return ago(d, 365*24*time.Hour, "year", "y", short)
	}
}

func ago(d, unit time.Duration, fullUnit, shortUnit string, short bool) string {
	n := int(d / unit)
	if n < 1 {
		n = 1
	}
	if short {
		return fmt.Sprintf("%d%s ago", n, shortUnit)
	}
	label := fullUnit
	if n != 1 {
		label += "s"
	}
	return fmt.Sprintf("%d %s ago", n, label)
}
