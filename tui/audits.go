package tui

import (
	"context"
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
	split         bool
	focusPane     int
	detailID      string
	detailLoading bool
	detailAudit   *domain.Audit
	reportIdx     int
	reportsTable  table.Model
	issuesTable   table.Model
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

func (m AuditsModel) Update(msg tea.Msg) (AuditsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.rebuildAuditsTable()
		if m.split {
			m.rebuildReportsTable()
			m.rebuildIssuesTable()
		}
		return m, nil

	case auditsLoadedMsg:
		m.loaded = true
		if msg.err != nil {
			return m, NotifyDanger(fmt.Sprintf("Failed to load audits: %v", msg.err))
		}
		m.audits = msg.audits
		m.rebuildAuditsTable()

		// Default state: open the split on the first audit when history exists.
		if !m.split && len(m.audits) > 0 {
			return m.openDetail(m.audits[0].ID, paneAudits)
		}
		return m, nil

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
		return m.updateList(msg)
	case auditsDeleteState:
		return m.updateDelete(msg)
	case auditsPromptState:
		return m.updatePrompt(msg)
	}
	return m, nil
}

// ---- List / split handling ----

func (m AuditsModel) updateList(msg tea.Msg) (AuditsModel, tea.Cmd) {
	if m.split {
		return m.updateSplit(msg)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q":
			return m, tea.Quit
		case "n":
			ti := textinput.New()
			ti.Placeholder = "https://example.com/page"
			ti.CharLimit = 2048
			ti.Width = 60
			ti.Focus()
			m.prompt = ti
			m.state = auditsPromptState
			return m, nil
		case "r":
			if sel := m.selAudit(); sel != nil {
				return m.startRerun(sel)
			}
			return m, nil
		case "d", "x":
			if sel := m.selAudit(); sel != nil {
				m.deleteID = sel.ID
				m.deleteName = sel.PlanName
				m.state = auditsDeleteState
			}
			return m, nil
		case "enter", "right", "l":
			if sel := m.selAudit(); sel != nil {
				return m.openDetail(sel.ID, paneAudits)
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// updateSplit routes keys to the focused pane. Left/right move the focus
// across the audits, reports and issues panes; the panes stay in sync with
// the selected audit/report.
func (m AuditsModel) updateSplit(msg tea.Msg) (AuditsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q":
			return m, tea.Quit
		case "esc":
			m.split = false
			m.detailID = ""
			m.detailLoading = false
			m.detailAudit = nil
			m.rebuildAuditsTable()
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
		before := m.reportsTable.Cursor()
		m.reportsTable, cmd = m.reportsTable.Update(msg)
		if m.detailAudit != nil && m.reportsTable.Cursor() != before {
			m.reportIdx = m.reportsTable.Cursor()
			m.rebuildIssuesTable()
		}

	case paneIssues:
		m.issuesTable, cmd = m.issuesTable.Update(msg)
	}
	return m, cmd
}

// openDetail enters (or keeps) the split view for the given audit.
func (m AuditsModel) openDetail(id string, focusPane int) (AuditsModel, tea.Cmd) {
	if id != m.detailID {
		m.detailID = id
		m.detailLoading = true
		m.detailAudit = nil
		m.reportIdx = 0
	}
	m.split = true
	m.focusPane = focusPane
	m.rebuildAuditsTable()
	m.rebuildReportsTable()
	m.rebuildIssuesTable()
	if m.detailLoading {
		return m, m.loadDetailCmd(id)
	}
	return m, nil
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
	m.split = false
	m.detailID = ""
	m.detailLoading = false
	m.detailAudit = nil
	m.reportIdx = 0
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
	if m.split {
		return m.splitView()
	}
	return m.listView()
}

func (m AuditsModel) View() string {
	switch m.state {
	case auditsListState:
		return m.contentView()
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
// and the issues of the selected report.
func (m AuditsModel) splitView() string {
	paneW := m.width / 3
	if paneW < 16 {
		return m.listView()
	}

	left := padLines(strings.Split(m.table.View(), "\n"), m.height, paneW)
	middle := padLines(strings.Split(m.middlePaneView(), "\n"), m.height, paneW)
	right := padLines(strings.Split(m.rightPaneView(), "\n"), m.height, paneW)

	divider := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("│")

	var b strings.Builder
	for i := 0; i < m.height; i++ {
		b.WriteString(left[i])
		b.WriteString(divider)
		b.WriteString(middle[i])
		b.WriteString(divider)
		b.WriteString(right[i])
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m AuditsModel) middlePaneView() string {
	if m.detailLoading {
		return "Loading reports..."
	}
	if m.detailAudit == nil || len(m.detailAudit.Reports) == 0 {
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

func padLines(lines []string, height, width int) []string {
	for len(lines) < height {
		lines = append(lines, "")
	}
	for i, l := range lines {
		if w := lipgloss.Width(l); w < width {
			lines[i] = l + strings.Repeat(" ", width-w)
		}
	}
	return lines
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
		if !m.split {
			return "Enter: Open Audit  •  n: Audit URL  •  r: Rerun  •  d: Delete  •  q: Quit"
		}
		switch m.focusPane {
		case paneAudits:
			return "→: Reports  •  ↑/↓: Audit  •  Esc: Close  •  q: Quit"
		case paneReports:
			return "←: Audits  •  →: Issues  •  ↑/↓: Report  •  Esc: Close  •  q: Quit"
		default:
			return "←: Reports  •  ↑/↓: Issue  •  Esc: Close  •  q: Quit"
		}
	}
	return ""
}

// ---- Tables ----

func (m *AuditsModel) rebuildAuditsTable() {
	cursor := m.table.Cursor()
	tableWidth := m.width - 2
	if m.split {
		tableWidth = m.width/3 - 2
	}
	if tableWidth < 10 {
		tableWidth = 10
	}

	var columns []table.Column
	rows := make([]table.Row, 0, len(m.audits))

	if !m.split {
		columns = []table.Column{
			{Title: "Plan", Width: 22},
			{Title: "Checklist", Width: 20},
			{Title: "Started", Width: 17},
			{Title: "Duration", Width: 11},
			{Title: "Failed", Width: 8},
			{Title: "Severity", Width: 10},
		}
		for _, a := range m.audits {
			rows = append(rows, table.Row{
				a.PlanName,
				a.ChecklistName,
				a.StartedAt.Format("2006-01-02 15:04"),
				a.Duration.Round(time.Millisecond).String(),
				fmt.Sprintf("%d", a.Summary.FailedCount),
				string(a.Summary.HighestSeverity),
			})
		}
	} else {
		sevW := 7
		durW := 7
		startedW := 13
		planW := tableWidth - sevW - durW - startedW - 6
		if planW < 8 {
			planW = 8
			startedW = tableWidth - sevW - durW - planW - 6
			if startedW < 8 {
				startedW = 8
			}
		}
		columns = []table.Column{
			{Title: "Plan", Width: planW},
			{Title: "Started", Width: startedW},
			{Title: "Duration", Width: durW},
			{Title: "Severity", Width: sevW},
		}
		for _, a := range m.audits {
			rows = append(rows, table.Row{
				a.PlanName,
				a.StartedAt.Format("2006-01-02 15:04"),
				a.Duration.Round(time.Millisecond).String(),
				string(a.Summary.HighestSeverity),
			})
		}
	}

	height := m.height
	if m.height <= 0 {
		height = 10
	}
	if height < 3 {
		height = 3
	}

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
	paneW := m.width / 3
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

	height := m.height
	if m.height <= 0 {
		height = 10
	}
	if height < 3 {
		height = 3
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(height),
	)
	t.SetStyles(tableStyle())
	t.SetWidth(paneW)
	t.SetCursor(m.reportIdx)
	m.reportsTable = t
}

func (m *AuditsModel) rebuildIssuesTable() {
	paneW := m.width / 3
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

	rows := make([]table.Row, 0, len(rep.Issues))
	for _, iss := range rep.Issues {
		row := []string{string(iss.Severity), clipCell(iss.Message, msgW-1)}
		if checkW > 0 {
			row = []string{string(iss.Severity), clipCell(iss.CheckName, checkW-1), clipCell(iss.Message, msgW-1)}
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return domain.Severity(rows[i][0]).Weight() > domain.Severity(rows[j][0]).Weight()
	})

	height := m.height
	if m.height <= 0 {
		height = 10
	}
	if height < 3 {
		height = 3
	}

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
