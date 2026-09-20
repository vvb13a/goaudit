package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/vvb13a/goaudit/domain"
)

// checkIssuesPageSize is how many issue rows one page of the checks tab's
// middle pane holds. Browsing a check never keeps more than a page of its
// issues resident.
const checkIssuesPageSize = 100

// checkSummariesLoadedMsg carries the per-check summaries of the current
// audit. auditID guards against stale responses.
type checkSummariesLoadedMsg struct {
	auditID string
	checks  []*domain.CheckSummary
	err     error
}

// checkIssuesLoadedMsg carries one page of issues of the selected check
// together with the total number of matching issues. auditID, checkName and
// offset guard against stale responses.
type checkIssuesLoadedMsg struct {
	auditID   string
	checkName string
	offset    int
	issues    []*domain.Issue
	total     int
	err       error
}

// Focused panes of the checks view: the checks and the issues of the selected
// check. The issue detail pane on the right is passive.
const (
	paneChecks = iota
	paneCheckIssues
)

// AuditChecksModel lists the stored issues of the current audit (tenant)
// grouped by check: each check with its category and issue count on the left
// (2/5 of the width), the issues of the selected check in the middle (2/5)
// and the details of the selected issue on the right (1/5). The issues of a
// check are browsed in pages that load on demand, so only one page is ever
// resident.
type AuditChecksModel struct {
	deps    Deps
	auditID string
	loaded  bool

	checks     []*domain.CheckSummary
	checkIdx   int
	checkTable table.Model

	issues        []domain.Issue
	issuesLoading bool
	issueTab      table.Model
	issueIdx      int
	issueOffset   int
	issueTotal    int

	focusPane int

	// cursorTo and cursorToLast steer the issue table cursor of the page that
	// is loading: crossing into a page from below lands on its last row, all
	// other jumps land on its first row.
	cursorTo     int
	cursorToLast bool

	width  int
	height int
}

func NewAuditChecksModel(deps Deps) AuditChecksModel {
	return AuditChecksModel{
		deps:       deps,
		checkTable: table.New(),
		issueTab:   table.New(),
		focusPane:  paneChecks,
		cursorTo:   -1,
	}
}

// NavigationEnabled lets the tab leave and re-enter freely (list only).
func (m AuditChecksModel) NavigationEnabled() bool {
	return true
}

// Track points the tab at the given audit and resets it so the next
// activation reloads its checks.
func (m AuditChecksModel) Track(id string) AuditChecksModel {
	if m.auditID != id {
		m = m.release()
	}
	m.auditID = id
	return m
}

// release drops the loaded checks, issues and tables so the memory is
// returned before the tab sits in the background. The next activation
// reloads them.
func (m AuditChecksModel) release() AuditChecksModel {
	m.loaded = false
	m.checks = nil
	m.checkIdx = 0
	m.issues = nil
	m.issuesLoading = false
	m.issueIdx = 0
	m.issueOffset = 0
	m.issueTotal = 0
	m.cursorTo = -1
	m.cursorToLast = false
	m.checkTable = table.New()
	m.issueTab = table.New()
	return m
}

// loadCmd loads the per-check summaries of the audit.
func (m AuditChecksModel) loadCmd() tea.Cmd {
	return func() tea.Msg {
		checks, err := m.deps.AuditService.CheckSummaries(context.Background(), m.auditID)
		return checkSummariesLoadedMsg{auditID: m.auditID, checks: checks, err: err}
	}
}

// issuesLoadCmd loads one page of the issues of the given check.
func (m AuditChecksModel) issuesLoadCmd(checkName string, offset int) tea.Cmd {
	return func() tea.Msg {
		filter := domain.IssueFilter{CheckNames: []string{checkName}}
		total, err := m.deps.AuditService.CountIssues(context.Background(), m.auditID, filter)
		if err != nil {
			return checkIssuesLoadedMsg{auditID: m.auditID, checkName: checkName, offset: offset, err: err}
		}
		issues, err := m.deps.AuditService.ListIssuesPage(context.Background(), m.auditID, filter, checkIssuesPageSize, offset)
		if err != nil {
			return checkIssuesLoadedMsg{auditID: m.auditID, checkName: checkName, offset: offset, err: err}
		}
		return checkIssuesLoadedMsg{auditID: m.auditID, checkName: checkName, offset: offset, issues: issues, total: total}
	}
}

// loadSelectedCheckIssues loads the issue page of the selected check at the
// model's current offset.
func (m AuditChecksModel) loadSelectedCheckIssues() tea.Cmd {
	c := m.selCheck()
	if c == nil {
		return nil
	}
	return m.issuesLoadCmd(c.Name, m.issueOffset)
}

func (m AuditChecksModel) Update(msg tea.Msg) (AuditChecksModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.rebuildTables()
		return m, nil

	case checkSummariesLoadedMsg:
		if msg.auditID != m.auditID {
			return m, nil
		}
		m.loaded = true
		if msg.err != nil {
			return m, NotifyDanger(fmt.Sprintf("Failed to load checks: %v", msg.err))
		}
		m.checks = msg.checks
		if m.checkIdx >= len(m.checks) {
			m.checkIdx = len(m.checks) - 1
		}
		if m.checkIdx < 0 {
			m.checkIdx = 0
		}
		m.issueIdx = 0
		m.issueOffset = 0
		m.issues = nil
		m.issuesLoading = true
		m.rebuildCheckTable()
		return m, m.loadSelectedCheckIssues()

	case checkIssuesLoadedMsg:
		if msg.auditID != m.auditID {
			return m, nil
		}
		if c := m.selCheck(); c == nil || c.Name != msg.checkName {
			return m, nil
		}
		if msg.offset != m.issueOffset {
			return m, nil
		}
		m.issuesLoading = false
		if msg.err != nil {
			return m, NotifyDanger(fmt.Sprintf("Failed to load issues: %v", msg.err))
		}
		m.issues = make([]domain.Issue, 0, len(msg.issues))
		for _, iss := range msg.issues {
			m.issues = append(m.issues, *iss)
		}
		m.issueTotal = msg.total
		m.issueOffset = m.clampIssueOffset(m.issueOffset)
		if m.issueIdx >= len(m.issues) {
			m.issueIdx = len(m.issues) - 1
		}
		if m.issueIdx < 0 {
			m.issueIdx = 0
		}
		m.rebuildIssueTable()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q":
			return m, tea.Quit
		case "left", "h":
			if m.focusPane > paneChecks {
				m.focusPane--
			}
			return m, nil
		case "right", "l":
			if m.focusPane < paneCheckIssues {
				m.focusPane++
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	switch m.focusPane {
	case paneChecks:
		key, ok := msg.(tea.KeyMsg)
		if !ok {
			return m, nil
		}
		switch key.String() {
		case "down", "j", "up", "k":
			before := m.checkTable.Cursor()
			m.checkTable, cmd = m.checkTable.Update(msg)
			if len(m.checks) > 0 && m.checkTable.Cursor() != before {
				m.checkIdx = m.checkTable.Cursor()
				m.issueIdx = 0
				m.issueOffset = 0
				m.issues = nil
				m.issuesLoading = true
				m.rebuildIssueTable()
				return m, tea.Batch(cmd, m.loadSelectedCheckIssues())
			}
			return m, cmd
		}
		return m, nil

	case paneCheckIssues:
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "o":
				if iss := m.selIssue(); iss != nil && iss.URL != "" {
					return m, openInBrowserCmd(iss.URL)
				}
				return m, nil
			case "r":
				// Re-run the checks of the audited URL that reached the page
				// of the selected issue, without a full audit run: no
				// snapshot is recorded.
				if iss := m.selIssue(); iss != nil && iss.URL != "" {
					return m, recheckPageCmd(m.deps, m.auditID, iss.URL)
				}
				return m, nil
			case "n", "pgdown":
				if m.issueOffset+checkIssuesPageSize < m.issueTotal {
					return m.jumpIssuePage(m.issueOffset + checkIssuesPageSize)
				}
				return m, nil
			case "p", "pgup":
				if m.issueOffset > 0 {
					return m.jumpIssuePage(m.issueOffset - checkIssuesPageSize)
				}
				return m, nil
			case "g":
				if m.issueOffset > 0 {
					return m.jumpIssuePage(0)
				}
				return m, nil
			case "G":
				last := m.clampIssueOffset(m.issueTotal)
				if m.issueOffset == last {
					return m, nil
				}
				m.cursorTo = -1
				m.cursorToLast = true
				m.issueOffset = last
				return m, m.loadSelectedCheckIssues()
			case "down", "j":
				// Crossing the last row of a page moves into the next one, so
				// scrolling feels continuous while only one page is resident.
				if cur := m.issueTab.Cursor(); len(m.issues) > 0 && cur == len(m.issues)-1 && m.issueOffset+len(m.issues) < m.issueTotal {
					return m.jumpIssuePage(m.issueOffset + checkIssuesPageSize)
				}
			case "up", "k":
				if cur := m.issueTab.Cursor(); cur == 0 && m.issueOffset > 0 {
					m.cursorTo = -1
					m.cursorToLast = true
					m.issueOffset = m.clampIssueOffset(m.issueOffset - checkIssuesPageSize)
					return m, m.loadSelectedCheckIssues()
				}
			}
		}
		before := m.issueTab.Cursor()
		m.issueTab, cmd = m.issueTab.Update(msg)
		if len(m.issues) > 0 && m.issueTab.Cursor() != before {
			m.issueIdx = m.issueTab.Cursor()
		}
	}
	return m, cmd
}

// jumpIssuePage moves the issue list to the start of the given page and loads
// it, keeping the previous page visible until the new one arrives.
func (m AuditChecksModel) jumpIssuePage(offset int) (AuditChecksModel, tea.Cmd) {
	m.cursorTo, m.cursorToLast = 0, false
	m.issueOffset = m.clampIssueOffset(offset)
	return m, m.loadSelectedCheckIssues()
}

// selCheck returns the check under the cursor of the left table.
func (m AuditChecksModel) selCheck() *domain.CheckSummary {
	if m.checkIdx < 0 || m.checkIdx >= len(m.checks) {
		return nil
	}
	return m.checks[m.checkIdx]
}

// selIssue returns the issue under the cursor of the middle table.
func (m AuditChecksModel) selIssue() *domain.Issue {
	if m.issueIdx < 0 || m.issueIdx >= len(m.issues) {
		return nil
	}
	return &m.issues[m.issueIdx]
}

// issuePageCount returns the number of pages of the selected check's issues.
func (m AuditChecksModel) issuePageCount() int {
	if m.issueTotal <= 0 {
		return 1
	}
	return (m.issueTotal-1)/checkIssuesPageSize + 1
}

// clampIssueOffset snaps a requested offset to the start of a page within the
// selected check's issues.
func (m AuditChecksModel) clampIssueOffset(offset int) int {
	if m.issueTotal <= 0 {
		return 0
	}
	if offset < 0 {
		offset = 0
	}
	last := ((m.issueTotal - 1) / checkIssuesPageSize) * checkIssuesPageSize
	if offset > last {
		return last
	}
	return offset
}

// ---- Rendering ----

func (m AuditChecksModel) View() string {
	if m.auditID == "" {
		return centerLines(helpStyle.Render("No audit selected. Press Ctrl+O to open the audit switcher."), m.width)
	}
	if !m.loaded {
		return "Loading checks..."
	}
	if len(m.checks) == 0 {
		return centerLines(helpStyle.Render("No issues for this audit."), m.width)
	}
	return m.splitView()
}

// split reports whether the three-pane split fits the current width.
func (m AuditChecksModel) split() bool {
	return m.width >= 96
}

// statusVisible reports whether the issue page status line has room and is
// useful.
func (m AuditChecksModel) statusVisible() bool {
	return m.split() && m.loaded && m.issueTotal > 0 && m.width >= 60
}

// leadLines is how many lines the issue page status occupies above the panes.
func (m AuditChecksModel) leadLines() int {
	if !m.statusVisible() {
		return 0
	}
	if m.height-2 < 3 {
		return 0
	}
	return 2
}

// paneBoxH is the height of the panes below the page status.
func (m AuditChecksModel) paneBoxH() int {
	h := m.height - m.leadLines()
	if h < 1 {
		h = 1
	}
	return h
}

// paneWidths returns the widths of the check, issue and detail panes: 2/5,
// 2/5 and 1/5 of the total width.
func (m AuditChecksModel) paneWidths() (leftW, midW, rightW int) {
	leftW = m.width * 2 / 5
	midW = m.width * 2 / 5
	return leftW, midW, m.width - leftW - midW
}

// paneInners returns the inner widths of the check and issue panes, matching
// the layout of splitView.
func (m AuditChecksModel) paneInners() (checkInner, issueInner int) {
	if m.split() {
		leftW, midW, _ := m.paneWidths()
		return leftW - 2, midW - 2
	}
	return m.width - 2, 0
}

// splitView renders the issue page status on top and the three panes below
// when the terminal is wide enough. On narrow terminals only the checks pane
// is shown.
func (m AuditChecksModel) splitView() string {
	if m.height < 5 {
		return "Terminal too small for the checks view."
	}

	boxH := m.paneBoxH()
	rows := boxH - 2
	if rows < 1 {
		rows = 1
	}

	var b strings.Builder
	if m.statusVisible() {
		b.WriteString(m.pageStatus())
		b.WriteString("\n")
	}
	head := b.String()

	if !m.split() {
		left := m.paneView(m.checkTable.View(), m.width-2, rows)
		box := paneBox(left, m.width, boxH, m.focusPane == paneChecks)
		if head == "" {
			return box
		}
		return head + box
	}

	leftW, midW, rightW := m.paneWidths()

	left := m.paneView(m.checkTable.View(), leftW-2, rows)
	mid := m.paneView(m.midPane(), midW-2, rows)
	right := m.paneView(m.detailPane(rightW-2), rightW-2, rows)

	leftBox := paneBox(left, leftW, boxH, m.focusPane == paneChecks)
	midBox := paneBox(mid, midW, boxH, m.focusPane == paneCheckIssues)
	rightBox := paneBox(right, rightW, boxH, false)

	leftLines := strings.Split(leftBox, "\n")
	midLines := strings.Split(midBox, "\n")
	rightLines := strings.Split(rightBox, "\n")

	b.Reset()
	b.WriteString(head)
	for i := 0; i < boxH; i++ {
		b.WriteString(leftLines[i])
		b.WriteString(midLines[i])
		b.WriteString(rightLines[i])
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// pageStatus renders the position of the current issue page inside the
// selected check's issues.
func (m AuditChecksModel) pageStatus() string {
	first := m.issueOffset + 1
	last := m.issueOffset + len(m.issues)
	label := fmt.Sprintf("Issues %d–%d of %d", first, last, m.issueTotal)
	if m.issuePageCount() > 1 {
		label += fmt.Sprintf("  •  Page %d/%d", m.issueOffset/checkIssuesPageSize+1, m.issuePageCount())
	}
	return helpStyle.Render(label)
}

// midPane renders the issues of the selected check.
func (m AuditChecksModel) midPane() string {
	if m.selCheck() == nil {
		return "Select a check on the left to inspect its issues."
	}
	if m.issuesLoading {
		return "Loading issues..."
	}
	if len(m.issues) == 0 {
		return "No issues for this check."
	}
	return m.issueTab.View()
}

// detailPane renders the details of the selected issue, padded and clipped by
// paneView.
func (m AuditChecksModel) detailPane(width int) string {
	iss := m.selIssue()
	if iss == nil {
		return ""
	}
	return strings.Join(issueDetailText(iss, width), "\n")
}

// paneView fills pane content out to exactly height lines of width w.
func (m AuditChecksModel) paneView(content string, w, height int) string {
	if w < 4 {
		w = 4
	}
	lines := strings.Split(content, "\n")
	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	for i := 0; i < height; i++ {
		lines[i] = clipToWidth(lines[i], w)
	}
	return strings.Join(lines, "\n")
}

func (m AuditChecksModel) Help() string {
	if m.auditID == "" {
		return "Ctrl+O: Audits"
	}
	paging := ""
	if m.issuePageCount() > 1 {
		paging = "n/p: Page  •  g/G: First/Last  •  "
	}
	switch m.focusPane {
	case paneChecks:
		return "→: Issues  •  ↑/↓: Check  •  Ctrl+O: Audits  •  q: Quit"
	default:
		return "←: Checks  •  ↑/↓: Issue  •  " + paging + "r: Recheck URL  •  o: Open URL  •  Ctrl+O: Audits  •  q: Quit"
	}
}

// ---- Tables ----

func (m *AuditChecksModel) rebuildTables() {
	m.rebuildCheckTable()
	m.rebuildIssueTable()
}

func (m *AuditChecksModel) rebuildCheckTable() {
	if !m.loaded {
		m.checkTable = table.New()
		return
	}

	inner, _ := m.paneInners()

	catW, highestW, countW := 16, 8, 7
	if inner < 55 {
		catW, highestW = 11, 7
	}
	nameW := inner - catW - highestW - countW - 3*4
	if nameW < 8 {
		nameW = 8
	}

	columns := []table.Column{
		{Title: "Check", Width: nameW},
		{Title: "Category", Width: catW},
		{Title: "Highest", Width: highestW},
		{Title: "Issues", Width: countW},
	}

	rows := make([]table.Row, 0, len(m.checks))
	for _, c := range m.checks {
		highest := string(c.Severity.Highest())
		if highest == "" {
			highest = "-"
		}
		rows = append(rows, table.Row{
			clipCell(c.Name, nameW-1),
			clipCell(c.Category.DisplayName(), catW-1),
			clipCell(highest, highestW-1),
			fmt.Sprintf("%d", c.Total),
		})
	}

	cursor := m.checkIdx
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
		table.WithHeight(m.rowCount()),
	)
	t.SetStyles(tableStyle())
	t.SetWidth(inner)
	t.SetCursor(cursor)
	m.checkTable = t
}

func (m *AuditChecksModel) rebuildIssueTable() {
	if !m.loaded {
		m.issueTab = table.New()
		return
	}
	if m.selCheck() == nil || m.issues == nil {
		m.issueTab = table.New()
		return
	}

	_, inner := m.paneInners()

	sevW, lcW := 8, 11
	urlW := inner * 30 / 100
	if urlW < 16 {
		urlW = 16
	}
	if urlW > 48 {
		urlW = 48
	}
	msgW := inner - sevW - lcW - urlW - 3*4
	if msgW < 10 {
		msgW = 10
	}

	columns := []table.Column{
		{Title: "Severity", Width: sevW},
		{Title: "Lifecycle", Width: lcW},
		{Title: "URL", Width: urlW},
		{Title: "Issue", Width: msgW},
	}

	urls := make([]string, 0, len(m.issues))
	for i := range m.issues {
		urls = append(urls, m.issues[i].URL)
	}
	host, shared := sharedHost(urls)

	rows := make([]table.Row, 0, len(m.issues))
	for i := range m.issues {
		iss := &m.issues[i]
		rows = append(rows, table.Row{
			clipCell(string(iss.Severity), sevW-1),
			clipCell(lifecycleLabel(iss.Lifecycle), lcW-1),
			clipCell(displayURL(iss.URL, host, shared), urlW-1),
			clipCell(iss.Message, msgW-1),
		})
	}

	cursor := m.issueIdx
	if cursor >= len(rows) {
		cursor = len(rows) - 1
	}
	switch {
	case m.cursorToLast && len(rows) > 0:
		cursor = len(rows) - 1
	case m.cursorTo >= 0:
		cursor = m.cursorTo
	}
	if cursor < 0 {
		cursor = 0
	}
	m.cursorTo = -1
	m.cursorToLast = false
	m.issueIdx = cursor

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(m.rowCount()),
	)
	t.SetStyles(tableStyle())
	t.SetWidth(inner)
	t.SetCursor(cursor)
	m.issueTab = t
}

// rowCount is the number of table rows that fit inside one pane.
func (m AuditChecksModel) rowCount() int {
	h := m.paneBoxH() - 2
	if m.height <= 0 {
		return 10
	}
	if h < 1 {
		return 1
	}
	return h
}
