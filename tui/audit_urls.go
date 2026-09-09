package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/vvb13a/goaudit/domain"
)

// urlsPageSize is how many audited URL rows one page of the urls tab holds.
// Browsing an audit never keeps more than a page of URL rows resident; the
// issues of the selected URL are fetched on demand.
const urlsPageSize = 100

// urlsPageLoadedMsg carries one page of audited URLs of the current audit
// (tenant) together with the aggregate state backing the summary widget.
// auditID and offset guard against stale responses.
type urlsPageLoadedMsg struct {
	auditID string
	offset  int
	urls    []*domain.AuditedUrl
	total   int
	agg     *domain.URLAggregates
	err     error
}

// urlIssuesLoadedMsg carries the issues of the audited URL selected in the
// urls tab. auditID and url guard against stale responses.
type urlIssuesLoadedMsg struct {
	auditID string
	url     string
	issues  []*domain.Issue
	err     error
}

// Focused panes of the urls view: the audited URLs and the issues of the
// selected URL. The evidence pane of the selected issue is passive.
const (
	paneUrls = iota
	paneUrlIssues
)

// AuditUrlsModel renders the audited URLs of the current audit (tenant) in a
// master/detail split: each URL with its duration, state, highest severity
// and score in the left pane, the issues of the selected URL (check,
// category, severity, lifecycle and message) in the middle pane, and the
// evidence of the selected issue in the right pane. URLs are browsed in
// pages that load on demand and the issues of the selected URL are fetched
// when it is selected, so only one page plus one URL's issues are resident.
// The panes take 2/5, 2/5 and 1/5 of the width.
type AuditUrlsModel struct {
	deps    Deps
	auditID string
	loaded  bool

	urls   []*domain.AuditedUrl // the current page
	agg    *domain.URLAggregates
	total  int
	offset int

	urlTable table.Model
	urlIdx   int

	issues        []domain.Issue
	issuesLoading bool
	issueTab      table.Model
	issueIdx      int
	focusPane     int

	// cursorTo and cursorToLast steer the table cursor of the page that is
	// loading: crossing into a page from above lands on its last row, all
	// other jumps land on its first row.
	cursorTo     int
	cursorToLast bool

	width  int
	height int
}

func NewAuditUrlsModel(deps Deps) AuditUrlsModel {
	return AuditUrlsModel{
		deps:      deps,
		urlTable:  table.New(),
		issueTab:  table.New(),
		focusPane: paneUrls,
		cursorTo:  -1,
	}
}

// NavigationEnabled lets the tab leave and re-enter freely (list only).
func (m AuditUrlsModel) NavigationEnabled() bool {
	return true
}

// Track points the tab at the given audit and resets it so the next
// activation reloads its first page.
func (m AuditUrlsModel) Track(id string) AuditUrlsModel {
	if m.auditID != id {
		m = m.release()
	}
	m.auditID = id
	return m
}

// release drops the loaded page, aggregates and issues so the memory is
// returned before the tab sits in the background. The next activation
// reloads them.
func (m AuditUrlsModel) release() AuditUrlsModel {
	m.loaded = false
	m.urls = nil
	m.agg = nil
	m.total = 0
	m.offset = 0
	m.urlIdx = 0
	m.issueIdx = 0
	m.issues = nil
	m.issuesLoading = false
	m.cursorTo = -1
	m.cursorToLast = false
	m.urlTable = table.New()
	m.issueTab = table.New()
	return m
}

// pageCount returns the number of pages of the audited URL list.
func (m AuditUrlsModel) pageCount() int {
	if m.total <= 0 {
		return 1
	}
	return (m.total-1)/urlsPageSize + 1
}

// clampOffset snaps a requested offset to the start of a page within the
// audited URL list.
func (m AuditUrlsModel) clampOffset(offset int) int {
	if m.total <= 0 {
		return 0
	}
	if offset < 0 {
		offset = 0
	}
	last := ((m.total - 1) / urlsPageSize) * urlsPageSize
	if offset > last {
		return last
	}
	return offset
}

// loadCmd loads the URL page at the given offset together with the URL
// aggregates of the audit.
func (m AuditUrlsModel) loadCmd(offset int) tea.Cmd {
	offset = m.clampOffset(offset)
	return func() tea.Msg {
		agg, err := m.deps.AuditService.URLAggregates(context.Background(), m.auditID)
		if err != nil {
			return urlsPageLoadedMsg{auditID: m.auditID, offset: offset, err: err}
		}
		urls, total, err := m.deps.AuditService.ListURLsPage(context.Background(), m.auditID, urlsPageSize, offset)
		if err != nil {
			return urlsPageLoadedMsg{auditID: m.auditID, offset: offset, err: err}
		}
		return urlsPageLoadedMsg{auditID: m.auditID, offset: offset, urls: urls, total: total, agg: agg}
	}
}

// issuesLoadCmd loads the issues of the given audited URL of the audit.
func (m AuditUrlsModel) issuesLoadCmd(url string) tea.Cmd {
	return func() tea.Msg {
		issues, err := m.deps.AuditService.URLIssues(context.Background(), m.auditID, url)
		return urlIssuesLoadedMsg{auditID: m.auditID, url: url, issues: issues, err: err}
	}
}

func (m AuditUrlsModel) Update(msg tea.Msg) (AuditUrlsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.rebuildTables()
		return m, nil

	case urlsPageLoadedMsg:
		if msg.auditID != m.auditID || msg.offset != m.offset {
			return m, nil
		}
		m.loaded = true
		if msg.err != nil {
			return m, NotifyDanger(fmt.Sprintf("Failed to load URLs: %v", msg.err))
		}
		m.urls = msg.urls
		m.agg = msg.agg
		m.total = msg.total
		m.offset = m.clampOffset(m.offset)
		if m.urlIdx >= len(m.urls) {
			m.urlIdx = len(m.urls) - 1
		}
		if m.urlIdx < 0 {
			m.urlIdx = 0
		}
		m.rebuildTables()
		if u := m.selURL(); u != nil {
			m.issues = nil
			m.issuesLoading = true
			return m, m.issuesLoadCmd(u.URL)
		}
		return m, nil

	case urlIssuesLoadedMsg:
		if msg.auditID != m.auditID {
			return m, nil
		}
		if u := m.selURL(); u == nil || u.URL != msg.url {
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
		m.rebuildIssueTable()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q":
			return m, tea.Quit
		case "left", "h":
			if m.focusPane > paneUrls {
				m.focusPane--
			}
			return m, nil
		case "right", "l":
			if m.focusPane < paneUrlIssues {
				m.focusPane++
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	switch m.focusPane {
	case paneUrls:
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "o":
				if u := m.selURL(); u != nil && u.URL != "" {
					return m, openInBrowserCmd(u.URL)
				}
				return m, nil
			case "r":
				// Re-run the checks of the selected URL without a full audit
				// run: no snapshot is recorded.
				if u := m.selURL(); u != nil && u.URL != "" {
					return m, recheckURLCmd(m.deps, m.auditID, u.URL)
				}
				return m, nil
			case "n", "pgdown":
				if m.offset+urlsPageSize < m.total {
					return m.jumpToPage(m.offset + urlsPageSize)
				}
				return m, nil
			case "p", "pgup":
				if m.offset > 0 {
					return m.jumpToPage(m.offset - urlsPageSize)
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
				// Crossing the last row of a page moves into the next one,
				// so scrolling feels continuous while only one page of URLs
				// is resident.
				if cur := m.urlTable.Cursor(); len(m.urls) > 0 && cur == len(m.urls)-1 && m.offset+len(m.urls) < m.total {
					return m.jumpToPage(m.offset + urlsPageSize)
				}
			case "up", "k":
				if cur := m.urlTable.Cursor(); cur == 0 && m.offset > 0 {
					m.cursorTo = -1
					m.cursorToLast = true
					return m, m.loadCmd(m.offset - urlsPageSize)
				}
			}
		}
		before := m.urlTable.Cursor()
		m.urlTable, cmd = m.urlTable.Update(msg)
		if len(m.urls) > 0 && m.urlTable.Cursor() != before {
			m.urlIdx = m.urlTable.Cursor()
			m.issueIdx = 0
			m.issues = nil
			m.issuesLoading = true
			m.rebuildIssueTable()
			if u := m.selURL(); u != nil {
				return m, tea.Batch(cmd, m.issuesLoadCmd(u.URL))
			}
		}

	case paneUrlIssues:
		before := m.issueTab.Cursor()
		m.issueTab, cmd = m.issueTab.Update(msg)
		if len(m.issues) > 0 && m.issueTab.Cursor() != before {
			m.issueIdx = m.issueTab.Cursor()
		}
	}
	return m, cmd
}

// jumpToPage moves the URL list to the start of the given page and loads it,
// keeping the previous page visible until the new one arrives.
func (m AuditUrlsModel) jumpToPage(offset int) (AuditUrlsModel, tea.Cmd) {
	m.cursorTo, m.cursorToLast = 0, false
	m.offset = m.clampOffset(offset)
	return m, m.loadCmd(m.offset)
}

// selURL returns the audited URL under the cursor of the left table.
func (m AuditUrlsModel) selURL() *domain.AuditedUrl {
	if m.urlIdx < 0 || m.urlIdx >= len(m.urls) {
		return nil
	}
	return m.urls[m.urlIdx]
}

// ---- Rendering ----

func (m AuditUrlsModel) View() string {
	if m.auditID == "" {
		return centerLines(helpStyle.Render("No audit selected. Press Ctrl+O to open the audit switcher."), m.width)
	}
	if !m.loaded {
		return "Loading URLs..."
	}
	if m.total == 0 {
		return centerLines(helpStyle.Render("No audited URLs for this audit."), m.width)
	}
	return m.splitView()
}

// summaryWidgetMinWidth is the narrowest terminal that still fits the six
// metric boxes of the summary widget.
const summaryWidgetMinWidth = 84

// widgetBlockLines are the lines the summary widget occupies above the
// panes: its four box rows plus one blank line.
const widgetBlockLines = 5

// widgetVisible reports whether the summary widget has room: the terminal
// must be wide enough for its boxes and the panes must stay usable below it.
func (m AuditUrlsModel) widgetVisible() bool {
	return m.loaded && m.agg != nil && m.agg.Total() > 0 && m.width >= summaryWidgetMinWidth && m.height-7 >= 3
}

// statusVisible reports whether the page status line has room and is useful.
func (m AuditUrlsModel) statusVisible() bool {
	return m.loaded && m.total > 0 && m.width >= 60
}

// leadLines is how many lines the summary widget and the page status occupy
// above the panes. The rendered view and the table geometry must agree on
// this so the boxed panes exactly fill the remaining height. Both blocks are
// dropped when they would leave no room for the panes.
func (m AuditUrlsModel) leadLines() int {
	lines := 0
	if m.widgetVisible() {
		lines += widgetBlockLines
	}
	if m.statusVisible() {
		lines += 2
	}
	if m.height-lines < 3 {
		return 0
	}
	return lines
}

// summaryWidget renders one stat box per audited URL lifecycle metric: total,
// new, active and missing counts plus the average duration and score across
// every URL of the audit.
func (m AuditUrlsModel) summaryWidget() string {
	if !m.widgetVisible() {
		return ""
	}

	agg := m.agg
	states := agg.URLStates
	total := agg.Total()

	metrics := []widgetMetric{
		{label: "Total", value: fmt.Sprintf("%d", total), color: lipgloss.Color("#7aa2f7")},
		{label: "New", value: fmt.Sprintf("%d", states.New), color: lipgloss.Color("#2ac3de")},
		{label: "Active", value: fmt.Sprintf("%d", states.Active), color: lipgloss.Color("#9ece6a")},
		{label: "Missing", value: fmt.Sprintf("%d", states.Missing), color: lipgloss.Color("#f7768e")},
		{label: "Avg Duration", value: formatDuration(agg.AvgDuration), color: lipgloss.Color("#bb9af7")},
		{label: "Avg Score", value: fmt.Sprintf("%.1f", agg.AvgScore), color: lipgloss.Color("#e0af68")},
	}
	return metricsWidget(metrics, m.width)
}

// pageStatus renders the position of the current page inside the audited URL
// list.
func (m AuditUrlsModel) pageStatus() string {
	if !m.statusVisible() {
		return ""
	}
	first := m.offset + 1
	last := m.offset + len(m.urls)
	label := fmt.Sprintf("Rows %d–%d of %d", first, last, m.total)
	if m.pageCount() > 1 {
		label += fmt.Sprintf("  •  Page %d/%d", m.offset/urlsPageSize+1, m.pageCount())
	}
	return helpStyle.Render(label)
}

// splitView renders the summary widget and page status on top and the three
// panes below when the terminal is wide enough: URLs take 2/5, their issues
// 2/5 and the issue evidence 1/5. On narrow terminals only the URL pane is
// shown.
func (m AuditUrlsModel) splitView() string {
	if m.height < 5 {
		return "Terminal too small for the urls view."
	}

	boxH := m.paneBoxH()
	rows := boxH - 2

	var b strings.Builder
	if m.widgetVisible() {
		b.WriteString(m.summaryWidget())
		b.WriteString("\n")
	}
	if m.statusVisible() {
		b.WriteString(m.pageStatus())
		b.WriteString("\n")
	}
	head := b.String()

	if !m.split() {
		left := m.paneView(m.leftPane(), m.width-2, rows)
		box := paneBox(left, m.width, boxH, m.focusPane == paneUrls)
		if head == "" {
			return box
		}
		return head + box
	}

	leftW, midW, rightW := m.paneWidths()

	left := m.paneView(m.leftPane(), leftW-2, rows)
	mid := m.paneView(m.midPane(), midW-2, rows)
	right := m.paneView(m.evidencePane(), rightW-2, rows)

	leftBox := paneBox(left, leftW, boxH, m.focusPane == paneUrls)
	midBox := paneBox(mid, midW, boxH, m.focusPane == paneUrlIssues)
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

func (m AuditUrlsModel) leftPane() string {
	if len(m.urls) == 0 {
		return "No audited URLs."
	}
	return m.urlTable.View()
}

func (m AuditUrlsModel) midPane() string {
	if u := m.selURL(); u == nil {
		return "Select a URL on the left to inspect its issues."
	} else if m.issuesLoading {
		return "Loading issues..."
	} else if len(m.issues) == 0 {
		if u.State == domain.UrlStateMissing {
			return "URL missing from the latest run — no issues."
		}
		return "No issues for this URL."
	}
	return m.issueTab.View()
}

// evidencePane renders the evidence of the selected issue of the selected
// URL as indented JSON. The pane stays empty until an issue with evidence is
// selected.
func (m AuditUrlsModel) evidencePane() string {
	if len(m.issues) == 0 || m.issuesLoading {
		return ""
	}
	idx := m.issueIdx
	if idx < 0 || idx >= len(m.issues) {
		return ""
	}
	iss := &m.issues[idx]
	if len(iss.Details) == 0 {
		return ""
	}

	_, _, rightW := m.paneWidths()
	width := rightW - 2
	if width < 12 {
		width = 12
	}

	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(iss.Details); err != nil {
		return ""
	}
	return wrapNarrow(strings.TrimRight(buf.String(), "\n"), width)
}

// wrapNarrow soft-wraps long lines at the given width without the minimum
// of wrapText, for panes narrower than 20 columns.
func wrapNarrow(s string, width int) string {
	if width < 4 {
		width = 4
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

// paneView fills pane content out to exactly height lines of width w.
func (m AuditUrlsModel) paneView(content string, w, height int) string {
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

func (m AuditUrlsModel) Help() string {
	if m.auditID == "" {
		return "Ctrl+O: Audits"
	}
	paging := ""
	if m.pageCount() > 1 {
		paging = "n/p: Page  •  g/G: First/Last  •  "
	}
	switch m.focusPane {
	case paneUrls:
		return "→: Issues  •  ↑/↓: URL  •  " + paging + "r: Recheck URL  •  o: Open in Browser  •  Ctrl+O: Audits  •  q: Quit"
	default:
		return "←: URLs  •  ↑/↓: Issue  •  Ctrl+O: Audits  •  q: Quit"
	}
}

// ---- Tables ----

func (m *AuditUrlsModel) rebuildTables() {
	m.rebuildUrlTable()
	m.rebuildIssueTable()
}

// split reports whether the three-pane split fits the current width.
func (m AuditUrlsModel) split() bool {
	return m.width >= 96
}

// paneWidths returns the widths of the url, issues and evidence panes: 2/5,
// 2/5 and 1/5 of the total width.
func (m AuditUrlsModel) paneWidths() (leftW, midW, rightW int) {
	leftW = m.width * 2 / 5
	midW = m.width * 2 / 5
	return leftW, midW, m.width - leftW - midW
}

// paneInners returns the inner widths of the url and issues panes, matching
// the layout of splitView.
func (m AuditUrlsModel) paneInners() (urlInner, issueInner int) {
	if m.split() {
		leftW, midW, _ := m.paneWidths()
		return leftW - 2, midW - 2
	}
	return m.width - 2, 0
}

func (m *AuditUrlsModel) rebuildUrlTable() {
	if !m.loaded {
		m.urlTable = table.New()
		return
	}

	inner, _ := m.paneInners()

	// Column budgets, shrinking as the pane narrows: the score and highest
	// severity columns are dropped first, then the state column; URL and
	// duration always survive.
	showScore, showHighest, showState := inner >= 74, inner >= 74, inner >= 52
	durW, stateW, highestW, scoreW := 9, 8, 8, 6
	if !showHighest {
		highestW = 0
	}
	if !showScore {
		scoreW = 0
	}
	if !showState {
		stateW = 0
	}

	colCount := 1 + 1 // url + duration
	for _, w := range []int{stateW, highestW, scoreW} {
		if w > 0 {
			colCount++
		}
	}
	urlW := inner - durW - stateW - highestW - scoreW - 3*colCount
	if urlW < 12 {
		urlW = 12
	}

	host, sameHost := commonHost(m.urls)

	var columns []table.Column
	columns = append(columns, table.Column{Title: "URL", Width: urlW})
	columns = append(columns, table.Column{Title: "Duration", Width: durW})
	if stateW > 0 {
		columns = append(columns, table.Column{Title: "State", Width: stateW})
	}
	if highestW > 0 {
		columns = append(columns, table.Column{Title: "Highest", Width: highestW})
	}
	if scoreW > 0 {
		columns = append(columns, table.Column{Title: "Score", Width: scoreW})
	}

	rows := make([]table.Row, 0, len(m.urls))
	for _, u := range m.urls {
		row := table.Row{clipCell(displayURL(u.URL, host, sameHost), urlW-1), formatDuration(u.Duration)}
		if stateW > 0 {
			row = append(row, clipCell(string(u.State), stateW-1))
		}
		if highestW > 0 {
			row = append(row, clipCell(string(u.Summary.HighestSeverity), highestW-1))
		}
		if scoreW > 0 {
			row = append(row, fmt.Sprintf("%.1f", u.Summary.Score))
		}
		rows = append(rows, row)
	}

	cursor := m.urlIdx
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
	m.urlIdx = cursor

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(m.rowCount()),
	)
	t.SetStyles(tableStyle())
	t.SetWidth(inner)
	t.SetCursor(cursor)
	m.urlTable = t
}

func (m *AuditUrlsModel) rebuildIssueTable() {
	if !m.loaded {
		m.issueTab = table.New()
		return
	}

	u := m.selURL()
	if u == nil || m.issues == nil {
		m.issueTab = table.New()
		return
	}

	sorted := make([]domain.Issue, len(m.issues))
	copy(sorted, m.issues)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Severity.Weight() > sorted[j].Severity.Weight()
	})
	m.issues = sorted
	cur := m.issueIdx
	if cur < 0 {
		cur = 0
	}
	if cur >= len(sorted) {
		cur = len(sorted) - 1
	}
	m.issueIdx = cur

	_, inner := m.paneInners()

	// Column budgets, shrinking as the pane narrows: the check and category
	// columns are dropped before the message column gets squeezed.
	sevW, lcW, catW, checkW := 8, 11, 13, 14
	msgW := inner - sevW - lcW - catW - checkW - 3*5 // sev,lc,cat,check,msg
	if msgW < 10 {
		checkW = 0
		msgW = inner - sevW - lcW - catW - 3*4
	}
	if msgW < 10 {
		catW = 0
		msgW = inner - sevW - lcW - 3*3
	}
	if msgW < 10 {
		msgW = 10
	}

	var columns []table.Column
	columns = append(columns,
		table.Column{Title: "Severity", Width: sevW},
		table.Column{Title: "Lifecycle", Width: lcW},
	)
	if catW > 0 {
		columns = append(columns, table.Column{Title: "Category", Width: catW})
	}
	if checkW > 0 {
		columns = append(columns, table.Column{Title: "Check", Width: checkW})
	}
	columns = append(columns, table.Column{Title: "Issue", Width: msgW})

	rows := make([]table.Row, 0, len(sorted))
	for _, iss := range sorted {
		row := table.Row{
			string(iss.Severity),
			lifecycleLabel(iss.Lifecycle),
		}
		if catW > 0 {
			row = append(row, clipCell(iss.Category.DisplayName(), catW-1))
		}
		if checkW > 0 {
			row = append(row, clipCell(iss.CheckName, checkW-1))
		}
		row = append(row, clipCell(iss.Message, msgW-1))
		rows = append(rows, row)
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(m.rowCount()),
	)
	t.SetStyles(tableStyle())
	t.SetWidth(inner)
	t.SetCursor(cur)
	m.issueTab = t
}

// rowCount is the number of table rows that fit inside one pane (the pane
// borders take two lines, and the summary widget and page status take their
// own block above).
func (m AuditUrlsModel) rowCount() int {
	h := m.paneBoxH() - 2
	if m.height <= 0 {
		return 10
	}
	if h < 1 {
		return 1
	}
	return h
}

// paneBoxH is the height of the panes below the summary widget and page
// status: the full content height minus their blocks when they are shown.
func (m AuditUrlsModel) paneBoxH() int {
	h := m.height - m.leadLines()
	if h < 1 {
		h = 1
	}
	return h
}

// formatDuration renders a duration compactly: milliseconds below a second,
// seconds with one decimal above it.
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

// sharedHost returns the host shared by every URL of the list and whether
// they all agree on it. Lists with fewer than two entries or unparsable
// URLs never share a host.
func sharedHost(urls []string) (host string, shared bool) {
	if len(urls) < 2 {
		return "", false
	}
	var common string
	for _, raw := range urls {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Host == "" {
			return "", false
		}
		h := strings.ToLower(parsed.Host)
		if common == "" {
			common = h
		} else if h != common {
			return "", false
		}
	}
	return common, true
}

// commonHost returns the host shared by every audited URL of the list, if
// any.
func commonHost(urls []*domain.AuditedUrl) (host string, shared bool) {
	raw := make([]string, len(urls))
	for i, u := range urls {
		raw[i] = u.URL
	}
	return sharedHost(raw)
}

// displayURL shortens a URL to its path when every URL of the list shares
// the given host, so the repeated domain does not eat the column width.
// Mixed hosts keep full URLs. In paged lists the host is derived from the
// loaded page, keeping the display consistent within the page.
func displayURL(raw, host string, shared bool) string {
	if !shared {
		return raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if path := parsed.EscapedPath(); path != "" {
		return path
	}
	return "/"
}
