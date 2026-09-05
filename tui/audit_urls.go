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

// auditUrlsLoadedMsg carries the fully hydrated audit whose audited URLs are
// shown in the urls tab. id guards against stale responses.
type auditUrlsLoadedMsg struct {
	id    string
	audit *domain.Audit
	err   error
}

// Focused panes of the urls view: the audited URLs and the issues of the
// selected URL. The evidence pane of the selected issue is passive.
const (
	paneUrls = iota
	paneUrlIssues
)

// AuditUrlsModel renders every audited URL of the current audit (tenant) in
// a master/detail split: each URL with its duration, state, highest severity
// and score in the left pane, the issues of the selected URL (check,
// category, severity, lifecycle and message) in the middle pane, and the
// evidence of the selected issue in the right pane. The panes take 2/5,
// 2/5 and 1/5 of the width.
type AuditUrlsModel struct {
	deps    Deps
	auditID string
	loaded  bool
	audit   *domain.Audit

	urls      []*domain.AuditedUrl
	urlTable  table.Model
	urlIdx    int
	issues    []domain.Issue
	issueTab  table.Model
	issueIdx  int
	focusPane int

	width  int
	height int
}

func NewAuditUrlsModel(deps Deps) AuditUrlsModel {
	return AuditUrlsModel{
		deps:      deps,
		urlTable:  table.New(),
		issueTab:  table.New(),
		focusPane: paneUrls,
	}
}

// NavigationEnabled lets the tab leave and re-enter freely (list only).
func (m AuditUrlsModel) NavigationEnabled() bool {
	return true
}

// Track points the tab at the given audit and marks it stale so the next
// activation reloads its URLs.
func (m AuditUrlsModel) Track(id string) AuditUrlsModel {
	if m.auditID != id {
		m.loaded = false
		m.audit = nil
		m.urls = nil
		m.issues = nil
		m.urlIdx = 0
		m.issueIdx = 0
		m.urlTable = table.New()
		m.issueTab = table.New()
	}
	m.auditID = id
	return m
}

func (m AuditUrlsModel) loadCmd(id string) tea.Cmd {
	return func() tea.Msg {
		audit, err := m.deps.AuditService.GetByID(context.Background(), id)
		return auditUrlsLoadedMsg{id: id, audit: audit, err: err}
	}
}

func (m AuditUrlsModel) Update(msg tea.Msg) (AuditUrlsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.rebuildTables()
		return m, nil

	case auditUrlsLoadedMsg:
		if msg.id != m.auditID {
			return m, nil
		}
		m.loaded = true
		if msg.err != nil {
			return m, NotifyDanger(fmt.Sprintf("Failed to load URLs: %v", msg.err))
		}
		m.audit = msg.audit
		m.urls = msg.audit.Urls
		if m.urlIdx >= len(m.urls) {
			m.urlIdx = 0
		}
		m.rebuildTables()
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
		if key, ok := msg.(tea.KeyMsg); ok && key.String() == "o" {
			if u := m.selURL(); u != nil && u.URL != "" {
				return m, openInBrowserCmd(u.URL)
			}
			return m, nil
		}
		before := m.urlTable.Cursor()
		m.urlTable, cmd = m.urlTable.Update(msg)
		if len(m.urls) > 0 && m.urlTable.Cursor() != before {
			m.urlIdx = m.urlTable.Cursor()
			m.issueIdx = 0
			m.rebuildIssueTable()
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
	if len(m.urls) == 0 {
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
	return m.width >= summaryWidgetMinWidth && len(m.urls) > 0 && m.height-5 >= 3
}

// paneBoxH is the height of the panes below the summary widget: the full
// content height minus the widget block when it is shown.
func (m AuditUrlsModel) paneBoxH() int {
	h := m.height
	if m.widgetVisible() {
		h -= widgetBlockLines
	}
	if h < 1 {
		h = 1
	}
	return h
}

// summaryWidget renders one stat box per audited URL lifecycle metric:
// total, new, active and missing counts plus the average duration and score
// of the listed URLs.
func (m AuditUrlsModel) summaryWidget() string {
	if !m.widgetVisible() {
		return ""
	}

	total := len(m.urls)
	var fresh, active, missing int
	for _, u := range m.urls {
		switch u.State {
		case domain.UrlStateNew:
			fresh++
		case domain.UrlStateActive:
			active++
		case domain.UrlStateMissing:
			missing++
		}
	}

	var durationSum time.Duration
	var scoreSum float64
	for _, u := range m.urls {
		durationSum += u.Duration
		scoreSum += u.Summary.Score
	}

	metrics := []widgetMetric{
		{label: "Total", value: fmt.Sprintf("%d", total), color: lipgloss.Color("#7aa2f7")},
		{label: "New", value: fmt.Sprintf("%d", fresh), color: lipgloss.Color("#2ac3de")},
		{label: "Active", value: fmt.Sprintf("%d", active), color: lipgloss.Color("#9ece6a")},
		{label: "Missing", value: fmt.Sprintf("%d", missing), color: lipgloss.Color("#f7768e")},
		{label: "Avg Duration", value: formatDuration(durationSum / time.Duration(total)), color: lipgloss.Color("#bb9af7")},
		{label: "Avg Score", value: fmt.Sprintf("%.1f", scoreSum/float64(total)), color: lipgloss.Color("#e0af68")},
	}
	return metricsWidget(metrics, m.width)
}

// splitView renders the summary widget on top and the three panes below when
// the terminal is wide enough: URLs take 2/5, their issues 2/5 and the issue
// evidence 1/5. On narrow terminals only the URL pane is shown.
func (m AuditUrlsModel) splitView() string {
	if m.height < 5 {
		return "Terminal too small for the urls view."
	}

	widget := ""
	if m.widgetVisible() {
		widget = m.summaryWidget() + "\n"
	}
	boxH := m.paneBoxH()
	rows := boxH - 2

	if !m.split() {
		left := m.paneView(m.leftPane(), m.width-2, rows)
		box := paneBox(left, m.width, boxH, m.focusPane == paneUrls)
		if widget == "" {
			return box
		}
		return widget + box
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

	var b strings.Builder
	b.WriteString(widget)
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
	} else if len(u.Issues) == 0 {
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
	if m.audit == nil || len(m.issues) == 0 {
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
	switch m.focusPane {
	case paneUrls:
		return "→: Issues  •  ↑/↓: URL  •  o: Open in Browser  •  Ctrl+O: Audits  •  q: Quit"
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
	if m.audit == nil {
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
	m.urlTable = t
}

func (m *AuditUrlsModel) rebuildIssueTable() {
	if m.audit == nil {
		m.issueTab = table.New()
		return
	}

	u := m.selURL()
	if u == nil {
		m.issueTab = table.New()
		return
	}

	sorted := make([]domain.Issue, len(u.Issues))
	copy(sorted, u.Issues)
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
// borders take two lines, and the summary widget takes its own block above).
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

// displayURL shortens a URL to its path when every URL of the audit shares
// the given host, so the repeated domain does not eat the column width.
// Mixed hosts keep full URLs.
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
