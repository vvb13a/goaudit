package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/vvb13a/goaudit/domain"
	"github.com/vvb13a/goaudit/service"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// auditsDetailMsg carries the fully hydrated audit (reports with issues) for
// the reports and issues panes. id guards against stale responses.
type auditsDetailMsg struct {
	id    string
	audit *domain.Audit
	err   error
}

type auditsState int

const (
	auditsListState auditsState = iota
	auditsRunningState
	auditsFormState
)

// Focused panes of the tenant view. The audit (tenant) itself is chosen by
// the root-level tenant switcher, so only its reports and issues panes are
// shown here.
const (
	paneReports = iota
	paneIssues
)

// auditOption mirrors one registry check in the audit run form.
type auditOption struct {
	name     string
	category domain.Category
	selected bool
}

// auditForm gathers the self-contained configuration of a new audit run:
// a name, the target URLs and the checks to run.
type auditForm struct {
	name    textinput.Model
	targets textarea.Model
	options []auditOption
	focus   int // 0 = name, 1 = targets, 2 = checks
	cursor  int // index into options while checks are focused
}

// AuditsModel renders the reports (URLs) and issues of the current tenant
// audit, and hosts the run form and progress of new audits.
type AuditsModel struct {
	deps         Deps
	state        auditsState
	form         auditForm
	progress     ProgressModel
	progressCh   chan ProgressMsg
	progressDone chan struct{}
	width        int
	height       int

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
		reportsTable: table.New(),
		issuesTable:  table.New(),
		progress:     NewProgressModel(),
	}
}

func (m AuditsModel) NavigationEnabled() bool {
	return m.state == auditsListState
}

// Running reports whether an audit run is currently in progress.
func (m AuditsModel) Running() bool {
	return m.state == auditsRunningState
}

// loadDetailCmd fetches the full audit for the given id.
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
		m.rebuildReportsTable()
		m.rebuildIssuesTable()
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
		if m.reportIdx >= len(msg.audit.Urls) {
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
	case auditsFormState:
		return m.updateForm(msg)
	}
	return m, nil
}

// ---- Tenant detail handling ----

// updateSplit routes keys to the focused pane: reports (URLs) on the left
// and issues on the right.
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
		case "left", "h":
			if m.focusPane > paneReports {
				m.focusPane--
			}
			return m, nil
		case "right", "l":
			if m.focusPane < paneIssues {
				m.focusPane++
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	switch m.focusPane {
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

// openDetail shows the given audit (tenant) as the current detail, loading it
// from the store when it is not already loaded.
func (m AuditsModel) openDetail(id string) (AuditsModel, tea.Cmd) {
	m.state = auditsListState
	m.issueDetailOpen = false
	if id != m.detailID {
		m.detailID = id
		m.detailLoading = true
		m.detailAudit = nil
		m.reportIdx = 0
	}
	m.focusPane = paneReports
	m.rebuildReportsTable()
	m.rebuildIssuesTable()
	if m.detailLoading {
		return m, m.loadDetailCmd(id)
	}
	return m, nil
}

// showTenant fills the panes with an already fully hydrated audit, without a
// round trip to the store. Used right after a run completes.
func (m AuditsModel) showTenant(a *domain.Audit) AuditsModel {
	m.state = auditsListState
	m.issueDetailOpen = false
	m.detailID = a.ID
	m.detailLoading = false
	m.detailAudit = a
	m.reportIdx = 0
	m.focusPane = paneReports
	m.rebuildReportsTable()
	m.rebuildIssuesTable()
	return m
}

// clearTenant drops the audit currently shown in the panes.
func (m AuditsModel) clearTenant() AuditsModel {
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

// beginNewAudit opens the run form so a new audit can be configured.
func (m AuditsModel) beginNewAudit() AuditsModel {
	m.form = newAuditForm(m.deps.Registry)
	m.state = auditsFormState
	return m
}

func (m AuditsModel) currentReport() *domain.AuditedUrl {
	if m.detailAudit == nil || m.reportIdx < 0 || m.reportIdx >= len(m.detailAudit.Urls) {
		return nil
	}
	return m.detailAudit.Urls[m.reportIdx]
}

// ---- Run form and execution ----

// newAuditForm builds the run form with every registered check preselected.
func newAuditForm(registry *service.CheckRegistry) auditForm {
	ti := textinput.New()
	ti.Placeholder = "e.g. Marketing Site Audit"
	ti.CharLimit = 80
	ti.Width = 50
	ti.Focus()

	ta := textarea.New()
	ta.Placeholder = "https://example.com\nhttps://example.com/pricing"
	ta.CharLimit = 8192
	ta.SetWidth(50)
	ta.SetHeight(6)

	var options []auditOption
	for _, c := range registry.All() {
		options = append(options, auditOption{
			name:     c.Info().Name,
			category: c.Info().Category,
			selected: true,
		})
	}

	return auditForm{
		name:    ti,
		targets: ta,
		options: options,
		focus:   0,
	}
}

func (m AuditsModel) updateForm(msg tea.Msg) (AuditsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.state = auditsListState
			return m, nil
		case "tab", "shift+tab", "backtab":
			m.form.focus = (m.form.focus + 1) % 3
			switch m.form.focus {
			case 0:
				m.form.name.Focus()
				m.form.targets.Blur()
			case 1:
				m.form.name.Blur()
				m.form.targets.Focus()
			default:
				m.form.name.Blur()
				m.form.targets.Blur()
			}
			return m, nil
		case "ctrl+s":
			return m.submitAuditForm()
		case "enter":
			if m.form.focus == 2 {
				return m.submitAuditForm()
			}
		case "up", "k", "down", "j", " ":
			// List navigation and toggling only apply while the checks list
			// is focused; anywhere else the keys reach the focused widget
			// (e.g. typing "k" or a space in the name or targets fields).
			if m.form.focus == 2 {
				switch msg.String() {
				case "up", "k":
					if m.form.cursor > 0 {
						m.form.cursor--
					}
				case "down", "j":
					if m.form.cursor < len(m.form.options)-1 {
						m.form.cursor++
					}
				case " ":
					if len(m.form.options) > 0 {
						opt := &m.form.options[m.form.cursor]
						opt.selected = !opt.selected
					}
				}
				return m, nil
			}
		}
	}

	var cmd tea.Cmd
	switch m.form.focus {
	case 0:
		m.form.name, cmd = m.form.name.Update(msg)
	case 1:
		m.form.targets, cmd = m.form.targets.Update(msg)
	}
	return m, cmd
}

// submitAuditForm validates the form, resolves the chosen checks and starts
// the audit run.
func (m AuditsModel) submitAuditForm() (AuditsModel, tea.Cmd) {
	name := strings.TrimSpace(m.form.name.Value())
	if name == "" {
		return m, NotifyDanger("Audit name is required")
	}

	var targets []string
	seen := make(map[string]struct{})
	for _, line := range strings.Split(m.form.targets.Value(), "\n") {
		u := strings.TrimSpace(line)
		if u == "" {
			continue
		}
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			u = "https://" + u
		}
		if _, exists := seen[u]; exists {
			continue
		}
		seen[u] = struct{}{}
		targets = append(targets, u)
	}
	if len(targets) == 0 {
		return m, NotifyDanger("Add at least one target URL")
	}

	var chosen []string
	for _, opt := range m.form.options {
		if opt.selected {
			chosen = append(chosen, opt.name)
		}
	}
	if len(chosen) == 0 {
		return m, NotifyDanger("Select at least one check")
	}

	checks, err := m.deps.Registry.Resolve(chosen)
	if err != nil {
		return m, NotifyDanger(fmt.Sprintf("Cannot run audit: %v", err))
	}

	cfg := effectiveConfig(m.deps, nil)
	return m.startAuditRun(name, "", targets, checks, cfg, "", fmt.Sprintf("Running audit '%s'", name))
}

// startRerun launches a new audit from the configuration stored on an
// existing one. Audits created before run configuration was stored inline
// fall back to the URLs of their previous reports and to all checks.
func (m AuditsModel) startRerun(a *domain.Audit) (AuditsModel, tea.Cmd) {
	name := a.Name
	description := a.Description
	targets := a.Targets
	checkNames := a.CheckNames

	if len(targets) == 0 || len(checkNames) == 0 {
		full, err := m.deps.AuditService.GetByID(context.Background(), a.ID)
		if err != nil {
			return m, NotifyDanger(fmt.Sprintf("Cannot rerun audit: %v", err))
		}
		if description == "" {
			description = full.Description
		}
		if len(targets) == 0 {
			for _, u := range full.Urls {
				// URLs that did not reappear in the latest run are skipped:
				// rerunning an audit re-checks the URLs it last audited.
				if u.State == domain.UrlStateMissing {
					continue
				}
				targets = append(targets, u.URL)
			}
		}
		if len(checkNames) == 0 {
			for _, c := range m.deps.Registry.All() {
				checkNames = append(checkNames, c.Info().Name)
			}
		}
	}
	if len(targets) == 0 {
		return m, NotifyDanger("Cannot rerun audit: no target URLs found")
	}

	checks, err := m.deps.Registry.Resolve(checkNames)
	if err != nil {
		return m, NotifyDanger(fmt.Sprintf("Cannot rerun audit: %v", err))
	}

	cfg := effectiveConfig(m.deps, a.Config)
	return m.startAuditRun(name, description, targets, checks, cfg, a.ID, fmt.Sprintf("Rerunning '%s'", name))
}

// startAuditRun launches the run: the audits view switches to its progress
// state and the runner executes every check against each target using the
// given effective configuration. replaceID carries the audit id to rerun in
// place ("" starts a brand-new audit).
func (m AuditsModel) startAuditRun(name, description string, targets []string, checks []domain.Check, cfg service.Config, replaceID, title string) (AuditsModel, tea.Cmd) {
	m.state = auditsRunningState
	m.progress = m.progress.Start(title, m.width)
	m.progressCh = make(chan ProgressMsg, 16)
	m.progressDone = make(chan struct{})

	return m, tea.Batch(
		newRunCmd(m.deps, name, description, targets, checks, cfg, replaceID, AuditsView, m.progressCh, m.progressDone),
		newProgressWaitCmd(m.progressCh, m.progressDone),
	)
}

// effectiveConfig resolves the engine configuration for an audit run: the
// app-level config is the fallback base and the audit's stored config JSON
// (raw, may be nil) is merged over it with per-key validation.
func effectiveConfig(deps Deps, raw []byte) service.Config {
	base := *service.DefaultConfig()
	if deps.ConfigManager != nil {
		base = deps.ConfigManager.Get()
	}
	return service.MergeConfig(base, raw)
}

func (m AuditsModel) finishRun() AuditsModel {
	m.state = auditsListState
	m.progressCh = nil
	m.progressDone = nil
	return m
}

// ---- Rendering ----

func (m AuditsModel) View() string {
	switch m.state {
	case auditsListState:
		content := m.contentView()
		if m.issueDetailOpen {
			return overlay(content, m.issueDetailView(), m.width, m.height)
		}
		return content
	case auditsRunningState:
		return m.progress.View()
	case auditsFormState:
		return overlay(m.contentView(), m.formView(), m.width, m.height)
	}
	return ""
}

func (m AuditsModel) contentView() string {
	if m.detailLoading {
		return m.splitView()
	}
	if m.detailAudit == nil {
		return m.emptyTenantView()
	}
	return m.splitView()
}

// splitView renders the two tenant panes side by side: URLs (reports) on the
// left and the issues of the selected report on the right, with a severity
// summary line on top.
func (m AuditsModel) splitView() string {
	paneW := m.width / 2
	if paneW < 20 || m.height < 5 {
		return "Terminal too small for the audit view."
	}

	boxH := m.height - 1
	innerW := paneW - 2

	left := m.paneView(m.leftPaneView(), innerW, boxH-2)
	right := m.paneView(m.rightPaneView(), innerW, boxH-2)

	leftBox := paneBox(left, paneW, boxH, m.focusPane == paneReports)
	rightBox := paneBox(right, paneW, boxH, m.focusPane == paneIssues)

	leftLines := strings.Split(leftBox, "\n")
	rightLines := strings.Split(rightBox, "\n")

	var b strings.Builder
	b.WriteString(m.severityWidget())
	b.WriteString("\n")
	for i := 0; i < boxH; i++ {
		b.WriteString(leftLines[i])
		b.WriteString(rightLines[i])
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// emptyTenantView is shown when no audit (tenant) is selected yet.
func (m AuditsModel) emptyTenantView() string {
	var b strings.Builder
	b.WriteString(helpStyle.Render("No audit selected."))
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("Press Ctrl+O to open the audit switcher — Enter picks an audit, 'n' starts a new one."))
	return centerLines(b.String(), m.width)
}

// centerLines horizontally centers each line within the given width.
func centerLines(content string, width int) string {
	var b strings.Builder
	for _, line := range strings.Split(content, "\n") {
		pad := width - lipgloss.Width(line)
		if pad < 0 {
			pad = 0
		}
		b.WriteString(strings.Repeat(" ", pad/2))
		b.WriteString(line)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m AuditsModel) leftPaneView() string {
	if m.detailLoading {
		return "Loading reports..."
	}
	if m.detailAudit == nil {
		return ""
	}
	if len(m.detailAudit.Urls) == 0 {
		return "No reports."
	}
	return m.reportsTable.View()
}

func (m AuditsModel) rightPaneView() string {
	if m.detailLoading || m.detailAudit == nil {
		return ""
	}
	if rep := m.currentReport(); rep == nil {
		return "Select a URL on the left to inspect its issues."
	} else if len(rep.Issues) == 0 {
		return "No issues."
	}
	return m.issuesTable.View()
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

// formView renders the fields of the new-audit run form: a name, one target
// URL per line and the checks to run.
func (m AuditsModel) formView() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Run a New Audit"))
	b.WriteString("\n\n")

	nameLabel := "Name:"
	if m.form.focus == 0 {
		nameLabel = labelStyle.Render("Name:")
	}
	b.WriteString(nameLabel + "\n" + m.form.name.View() + "\n\n")

	targetsLabel := "Target URLs (one per line):"
	if m.form.focus == 1 {
		targetsLabel = labelStyle.Render("Target URLs (one per line):")
	}
	b.WriteString(targetsLabel + "\n" + m.form.targets.View() + "\n\n")

	checksLabel := "Checks (Space to toggle):"
	if m.form.focus == 2 {
		checksLabel = labelStyle.Render("Checks (Space to toggle):")
	}
	b.WriteString(checksLabel + "\n")

	for i, opt := range m.form.options {
		cursor := "  "
		if m.form.focus == 2 && m.form.cursor == i {
			cursor = labelStyle.Render("> ")
		}
		checked := "[ ]"
		if opt.selected {
			checked = labelStyle.Render("[x]")
		}
		cat := helpStyle.Render(string(opt.category))
		b.WriteString(fmt.Sprintf("%s%s %-25s %s\n", cursor, checked, opt.name, cat))
	}
	return b.String()
}

func (m AuditsModel) Help() string {
	switch m.state {
	case auditsRunningState:
		return "Audit in progress  •  Ctrl+C: Quit"
	case auditsFormState:
		return "Tab: Switch  •  Space: Toggle  •  Ctrl+S: Run  •  Esc: Cancel"
	default:
		if m.issueDetailOpen {
			return "Esc: Close Details  •  q: Quit"
		}
		switch m.focusPane {
		case paneReports:
			return "→: Issues  •  ↑/↓: URL  •  o: Open in Browser  •  Ctrl+O: Audits  •  q: Quit"
		default:
			return "←: URLs  •  ↑/↓: Issue  •  Enter: Details  •  Ctrl+O: Audits  •  q: Quit"
		}
	}
}

// ---- Tables ----

func (m *AuditsModel) rebuildReportsTable() {
	paneW := m.width/2 - 2
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

	rows := make([]table.Row, 0, len(m.detailAudit.Urls))
	for _, rep := range m.detailAudit.Urls {
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
	paneW := m.width/2 - 2
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

func reportResult(u *domain.AuditedUrl) string {
	if failed := u.Summary.FailedCount(); failed > 0 {
		return fmt.Sprintf("%d fail", failed)
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

// ---- Exports (used by the tenant switcher) ----

// exportExcelCmd hydrates the full audit (reports with issues), renders it
// into an Excel workbook (reusing an existing one instead of regenerating)
// and opens the workbook with the default xlsx viewer.
func exportExcelCmd(deps Deps, a *domain.Audit) tea.Cmd {
	return func() tea.Msg {
		if deps.ExcelService == nil {
			return notifyMsg{notification: Notification{
				Kind: NotificationDanger,
				Text: "Excel export is not available",
			}}
		}

		full, err := deps.AuditService.GetByID(context.Background(), a.ID)
		if err != nil {
			return notifyMsg{notification: Notification{
				Kind: NotificationDanger,
				Text: fmt.Sprintf("Export failed: %v", err),
			}}
		}

		path, err := deps.ExcelService.ExportAudit(full)
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

// exportHTMLCmd hydrates the full audit (reports with issues), renders it
// into an HTML report (reusing an existing one instead of regenerating) and
// opens the report in the default browser.
func exportHTMLCmd(deps Deps, a *domain.Audit) tea.Cmd {
	return func() tea.Msg {
		if deps.HtmlService == nil {
			return notifyMsg{notification: Notification{
				Kind: NotificationDanger,
				Text: "HTML report is not available",
			}}
		}

		full, err := deps.AuditService.GetByID(context.Background(), a.ID)
		if err != nil {
			return notifyMsg{notification: Notification{
				Kind: NotificationDanger,
				Text: fmt.Sprintf("Export failed: %v", err),
			}}
		}

		path, err := deps.HtmlService.ExportAudit(full)
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
			Text: fmt.Sprintf("Opened audit report %s", path),
		}}
	}
}
