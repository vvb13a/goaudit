package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/vvb13a/goaudit/domain"
	"github.com/vvb13a/goaudit/service"
)

// dashboardLoadedMsg carries the aggregate state of the current audit for
// the dashboard: its run metadata (score, duration, config) and the metric
// counts derived from its URL and issue rows, together with the snapshot of
// its previous run (nil when this is the first run). id guards against stale
// responses.
type dashboardLoadedMsg struct {
	id       string
	audit    *domain.Audit
	metrics  *domain.AuditMetrics
	checks   int
	previous *domain.AuditSnapshot
	err      error
}

// auditCreatedMsg reports the outcome of creating a new audit without
// running it.
type auditCreatedMsg struct {
	audit *domain.Audit
	err   error
}

type dashboardState int

const (
	dashboardListState dashboardState = iota
	dashboardRunningState
	dashboardFormState
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

// DashboardModel is the landing tab of the app. It renders the metrics of
// the current audit (tenant) as stacked widgets — overview totals, audited
// URL states, severity and lifecycle distributions and run timing — and
// hosts the run form and progress of new audits.
type DashboardModel struct {
	deps         Deps
	state        dashboardState
	form         auditForm
	progress     ProgressModel
	progressCh   chan ProgressMsg
	progressDone chan struct{}
	width        int
	height       int

	auditID string
	loading bool
	audit   *domain.Audit
	metrics *domain.AuditMetrics
	checks  int

	// previous is the snapshot of the run before the current one, used to
	// annotate the metrics with deltas.
	previous *domain.AuditSnapshot
}

func NewDashboardModel(deps Deps) DashboardModel {
	return DashboardModel{
		deps:     deps,
		state:    dashboardListState,
		progress: NewProgressModel(),
	}
}

func (m DashboardModel) NavigationEnabled() bool {
	return m.state == dashboardListState
}

// Running reports whether an audit run is currently in progress.
func (m DashboardModel) Running() bool {
	return m.state == dashboardRunningState
}

func (m DashboardModel) loadCmd(id string) tea.Cmd {
	return func() tea.Msg {
		// The dashboard aggregates the audit from the store instead of
		// hydrating its URL and issue rows: the record carries the run
		// metadata, the metric query the counts and the snapshots the
		// previous-run deltas.
		audit, err := m.deps.AuditService.GetConfig(context.Background(), id)
		if err != nil {
			return dashboardLoadedMsg{id: id, err: err}
		}
		metrics, err := m.deps.AuditService.DashboardMetrics(context.Background(), id)
		if err != nil {
			return dashboardLoadedMsg{id: id, err: err}
		}
		checks := len(audit.CheckNames)
		if checks == 0 {
			if n, err := m.deps.AuditService.DistinctIssueChecks(context.Background(), id); err == nil {
				checks = n
			}
		}
		var previous *domain.AuditSnapshot
		if snapshots, err := m.deps.AuditService.ListSnapshots(context.Background(), id, 2); err == nil && len(snapshots) > 1 {
			previous = snapshots[1]
		}
		return dashboardLoadedMsg{id: id, audit: audit, metrics: metrics, checks: checks, previous: previous}
	}
}

func (m DashboardModel) Update(msg tea.Msg) (DashboardModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case dashboardLoadedMsg:
		if msg.id != m.auditID {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			return m, NotifyDanger(fmt.Sprintf("Failed to load audit metrics: %v", msg.err))
		}
		m.audit = msg.audit
		m.metrics = msg.metrics
		m.checks = msg.checks
		m.previous = msg.previous
		return m, nil

	case ProgressMsg:
		if m.state != dashboardRunningState {
			return m, nil
		}
		var cmd tea.Cmd
		m.progress, cmd = m.progress.Update(msg)
		return m, tea.Batch(cmd, newProgressWaitCmd(m.progressCh, m.progressDone))

	case progress.FrameMsg:
		if m.state == dashboardRunningState {
			var cmd tea.Cmd
			m.progress, cmd = m.progress.Update(msg)
			return m, cmd
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "q":
			return m, tea.Quit
		case "esc":
			if m.state == dashboardFormState {
				m.state = dashboardListState
			}
		}
	}

	if m.state == dashboardFormState {
		return m.updateForm(msg)
	}
	return m, nil
}

// openDetail points the dashboard at the given audit and loads its metrics
// when it is not already loaded.
func (m DashboardModel) openDetail(id string) (DashboardModel, tea.Cmd) {
	m.state = dashboardListState
	if id != m.auditID {
		m.auditID = id
		m.loading = true
		m.audit = nil
		m.metrics = nil
		m.checks = 0
		m.previous = nil
	}
	if m.loading {
		return m, m.loadCmd(id)
	}
	return m, nil
}

// clearTenant drops the audit currently shown on the dashboard.
func (m DashboardModel) clearTenant() DashboardModel {
	m.auditID = ""
	m.loading = false
	m.audit = nil
	m.metrics = nil
	m.checks = 0
	m.previous = nil
	m.state = dashboardListState
	return m
}

// release drops the loaded dashboard state so the memory is returned before
// the tab sits in the background. The next activation reloads it.
func (m DashboardModel) release() DashboardModel {
	m.loading = true
	m.audit = nil
	m.metrics = nil
	m.checks = 0
	m.previous = nil
	return m
}

// beginNewAudit opens the run form so a new audit can be configured.
func (m DashboardModel) beginNewAudit() DashboardModel {
	m.form = newAuditForm(m.deps.Registry)
	m.state = dashboardFormState
	return m
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

func (m DashboardModel) updateForm(msg tea.Msg) (DashboardModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.state = dashboardListState
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
func (m DashboardModel) submitAuditForm() (DashboardModel, tea.Cmd) {
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

	// Creating an audit only persists its configuration: the first run is
	// started deliberately afterwards (r in the audit switcher).
	m.state = dashboardListState
	cfg := effectiveConfig(m.deps, nil)
	configRaw, err := json.Marshal(cfg)
	if err != nil {
		return m, NotifyDanger(fmt.Sprintf("Cannot encode config: %v", err))
	}
	idName := name
	return m, func() tea.Msg {
		audit, err := m.deps.AuditService.CreateBlank(context.Background(), idName, "", targets, chosen, configRaw)
		return auditCreatedMsg{audit: audit, err: err}
	}
}

// startRerun launches a new audit from the configuration stored on an
// existing one. Audits created before run configuration was stored inline
// fall back to the URLs of their previous run and to all checks.
func (m DashboardModel) startRerun(a *domain.Audit) (DashboardModel, tea.Cmd) {
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

// startAuditRun launches the run: the dashboard switches to its progress
// state and the runner executes every check against each target using the
// given effective configuration. replaceID carries the audit id to rerun in
// place ("" starts a brand-new audit).
func (m DashboardModel) startAuditRun(name, description string, targets []string, checks []domain.Check, cfg service.Config, replaceID, title string) (DashboardModel, tea.Cmd) {
	m.state = dashboardRunningState
	m.progress = m.progress.Start(title, m.width)
	m.progressCh = make(chan ProgressMsg, 16)
	m.progressDone = make(chan struct{})

	return m, tea.Batch(
		newRunCmd(m.deps, name, description, targets, checks, cfg, replaceID, DashboardView, m.progressCh, m.progressDone),
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

func (m DashboardModel) finishRun() DashboardModel {
	m.state = dashboardListState
	m.progressCh = nil
	m.progressDone = nil
	return m
}

// ---- Rendering ----

func (m DashboardModel) View() string {
	switch m.state {
	case dashboardListState:
		return m.contentView()
	case dashboardRunningState:
		return m.progress.View()
	case dashboardFormState:
		return overlay(m.contentView(), m.formView(), m.width, m.height)
	}
	return ""
}

func (m DashboardModel) contentView() string {
	if m.loading {
		return "Loading audit metrics..."
	}
	if m.audit == nil {
		return m.emptyTenantView()
	}
	return m.metricsView()
}

// emptyTenantView is shown when no audit (tenant) is selected yet.
func (m DashboardModel) emptyTenantView() string {
	var b strings.Builder
	b.WriteString(helpStyle.Render("No audit selected."))
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("Press Ctrl+O to open the audit switcher — Enter picks an audit, 'n' starts a new one."))
	return centerLines(b.String(), m.width)
}

// formView renders the fields of the new-audit run form: a name, one target
// URL per line and the checks to run.
func (m DashboardModel) formView() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("New Audit"))
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

// metricsView stacks the metric widgets of the current audit on top of an
// audit header: the run totals, the audited URL states, the severity and
// lifecycle distributions and the run timing, each under its own title and
// description.
func (m DashboardModel) metricsView() string {
	w := m.width
	if w < 20 {
		w = 20
	}

	var b strings.Builder
	b.WriteString(m.auditHeader(w))
	b.WriteString("\n\n")

	sections := []struct {
		title       string
		description string
		accent      lipgloss.Color
		metrics     []widgetMetric
		diagram     string
	}{
		{
			title:       "Overview",
			description: "Total audited URLs, issues found and checks executed in this run.",
			accent:      lipgloss.Color("#7aa2f7"),
			metrics:     m.overviewMetrics(),
		},
		{
			title:       "URL States",
			description: "How the URLs relate to the previous run: first seen, still audited or missing from the latest run.",
			accent:      lipgloss.Color("#2ac3de"),
			metrics:     m.urlStateMetrics(),
		},
		{
			title:       "Severity",
			description: "Issues of the latest run grouped by severity, from fatal to pass.",
			accent:      lipgloss.Color("#e0af68"),
			metrics:     m.severityMetrics(),
			diagram:     m.severityChart(w),
		},
		{
			title:       "Lifecycles",
			description: "How every issue evolved between the previous and the latest run.",
			accent:      lipgloss.Color("#bb9af7"),
			metrics:     m.lifecycleMetrics(),
			diagram:     m.lifecycleChart(w),
		},
		{
			title:       "Timing",
			description: "Wall-clock duration of the run, and its share per audited URL and per issue.",
			accent:      lipgloss.Color("#9ece6a"),
			metrics:     m.timingMetrics(),
		},
	}

	for i, s := range sections {
		b.WriteString(metricsSection(w, s.accent, s.title, s.description, s.metrics, s.diagram))
		if i < len(sections)-1 {
			b.WriteString("\n\n")
		}
	}
	return b.String()
}

// auditHeader renders the audit name and its description on top of the
// dashboard.
func (m DashboardModel) auditHeader(width int) string {
	a := m.audit
	if width < 20 {
		width = 20
	}
	var b strings.Builder

	name := strings.TrimSpace(a.Name)
	if name == "" {
		name = "Untitled Audit"
	}
	b.WriteString(lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		Render(clipToWidth(name, width)))

	if description := strings.TrimSpace(a.Description); description != "" {
		b.WriteString("\n")
		b.WriteString(helpStyle.Render(wrapText(description, width)))
	}
	return b.String()
}

// metricsSection renders one dashboard widget: its title and description on
// top, the metric boxes below and an optional diagram under them.
func metricsSection(width int, accent lipgloss.Color, title, description string, metrics []widgetMetric, diagram string) string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().
		Bold(true).
		Foreground(accent).
		Render(strings.ToUpper(title)))
	b.WriteString("\n")

	if description != "" {
		descStyle := lipgloss.NewStyle().Faint(true)
		for _, line := range strings.Split(wrapText(description, width), "\n") {
			b.WriteString(descStyle.Render(line))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	if boxed := metricsWidget(metrics, width); boxed != "" {
		b.WriteString(boxed)
	}
	if diagram != "" {
		if metrics != nil {
			b.WriteString("\n\n")
		}
		b.WriteString(diagram)
	}
	return strings.TrimRight(b.String(), "\n")
}

// ---- Metrics ----

// metricBox pairs a metric widget value with its label and color.
type metricBox struct {
	label string
	color lipgloss.Color
}

// metricsOf pairs widget boxes with their values by position.
func metricsOf(boxes []metricBox, values ...string) []widgetMetric {
	metrics := make([]widgetMetric, 0, len(boxes))
	for i, b := range boxes {
		value := ""
		if i < len(values) {
			value = values[i]
		}
		metrics = append(metrics, widgetMetric{
			label: b.label,
			value: value,
			color: b.color,
		})
	}
	return metrics
}

// metrics must be loaded before any of the metric helpers below run; the
// view only reaches them when m.metrics is set.

// overviewMetrics are the hero totals: audited URLs, issues, checks and the
// overall audit score.
func (m DashboardModel) overviewMetrics() []widgetMetric {
	urls := m.metrics.TotalURLs()
	issues := m.metrics.TotalIssues()
	checks := m.checks
	var urlsDelta, issuesDelta, checksDelta, scoreDelta string
	if prev := m.previous; prev != nil {
		urlsDelta = intDelta(urls, prev.Overview.TotalURLs)
		issuesDelta = intDelta(issues, prev.Overview.TotalIssues)
		checksDelta = intDelta(checks, prev.Overview.TotalChecks)
		scoreDelta = floatDelta(m.audit.Score, prev.Overview.Score)
	}

	score := m.audit.Score
	boxes := []metricBox{
		{label: "Total URLs", color: lipgloss.Color("#7aa2f7")},
		{label: "Total Issues", color: lipgloss.Color("#9ece6a")},
		{label: "Total Checks", color: lipgloss.Color("#e0af68")},
		{label: "Score", color: auditScoreColor(score)},
	}
	return metricsOf(boxes,
		withDelta(fmt.Sprintf("%d", urls), urlsDelta),
		withDelta(fmt.Sprintf("%d", issues), issuesDelta),
		withDelta(fmt.Sprintf("%d", checks), checksDelta),
		withDelta(fmt.Sprintf("%.1f", score), scoreDelta),
	)
}

// auditScoreColor maps the audit score (0-100) to a health color.
func auditScoreColor(score float64) lipgloss.Color {
	switch {
	case score >= 90:
		return lipgloss.Color("#9ece6a")
	case score >= 70:
		return lipgloss.Color("#e0af68")
	case score >= 50:
		return lipgloss.Color("#ff9e64")
	default:
		return lipgloss.Color("#f7768e")
	}
}

// urlStateMetrics counts the audited URLs by their lifecycle state.
func (m DashboardModel) urlStateMetrics() []widgetMetric {
	states := m.metrics.URLStates
	var newDelta, activeDelta, missingDelta string
	if prev := m.previous; prev != nil {
		newDelta = intDelta(states.New, prev.URLStates.New)
		activeDelta = intDelta(states.Active, prev.URLStates.Active)
		missingDelta = intDelta(states.Missing, prev.URLStates.Missing)
	}
	boxes := []metricBox{
		{label: "New", color: lipgloss.Color("#2ac3de")},
		{label: "Active", color: lipgloss.Color("#9ece6a")},
		{label: "Missing", color: lipgloss.Color("#f7768e")},
	}
	return metricsOf(boxes,
		withDelta(fmt.Sprintf("%d", states.New), newDelta),
		withDelta(fmt.Sprintf("%d", states.Active), activeDelta),
		withDelta(fmt.Sprintf("%d", states.Missing), missingDelta),
	)
}

// severityMetrics tallies every issue of the audit by severity.
func (m DashboardModel) severityMetrics() []widgetMetric {
	counts := m.metrics.Severity
	fatal, errs, warns := counts.Fatal, counts.Error, counts.Warning
	notices, infos, passes := counts.Notice, counts.Info, counts.Success

	var fatalDelta, errorDelta, warnDelta, noticeDelta, infoDelta, passDelta string
	if prev := m.previous; prev != nil {
		p := prev.SeverityCounts
		fatalDelta = intDelta(fatal, p.Fatal)
		errorDelta = intDelta(errs, p.Error)
		warnDelta = intDelta(warns, p.Warning)
		noticeDelta = intDelta(notices, p.Notice)
		infoDelta = intDelta(infos, p.Info)
		passDelta = intDelta(passes, p.Success)
	}
	boxes := []metricBox{
		{label: "FATAL", color: issueSeverityColor(domain.SeverityFatal)},
		{label: "ERROR", color: issueSeverityColor(domain.SeverityError)},
		{label: "WARN", color: issueSeverityColor(domain.SeverityWarning)},
		{label: "NOTICE", color: issueSeverityColor(domain.SeverityNotice)},
		{label: "INFO", color: issueSeverityColor(domain.SeverityInfo)},
		{label: "PASS", color: issueSeverityColor(domain.SeveritySuccess)},
	}
	return metricsOf(boxes,
		withDelta(fmt.Sprintf("%d", fatal), fatalDelta),
		withDelta(fmt.Sprintf("%d", errs), errorDelta),
		withDelta(fmt.Sprintf("%d", warns), warnDelta),
		withDelta(fmt.Sprintf("%d", notices), noticeDelta),
		withDelta(fmt.Sprintf("%d", infos), infoDelta),
		withDelta(fmt.Sprintf("%d", passes), passDelta),
	)
}

// barChartRow is one row of a metric bar chart: a label, its count, the bar
// color and, when a previous run is known, its signed change. moreIsGood
// controls how the delta is colored.
type barChartRow struct {
	label      string
	count      int
	color      lipgloss.Color
	delta      int
	hasDelta   bool
	moreIsGood bool
}

// barChart renders a horizontal bar chart: one row per metric, every bar
// scaled against the largest count so the shape of the distribution is
// visible at a glance. Rows with a previous run also carry their signed
// change, colored green when the move is good and red when it is bad.
func barChart(rows []barChartRow, width int) string {
	if len(rows) == 0 {
		return ""
	}

	maxCount, labelW, deltaW := 0, 0, 0
	for _, r := range rows {
		if r.count > maxCount {
			maxCount = r.count
		}
		if len(r.label) > labelW {
			labelW = len(r.label)
		}
		if r.hasDelta {
			if d := len(fmt.Sprintf("(%+d)", r.delta)); r.delta != 0 && d > deltaW {
				deltaW = d
			}
		}
	}
	valueW := len(fmt.Sprintf("%d", maxCount))
	if valueW < 1 {
		valueW = 1
	}

	// One space before the count plus, when present, a space and the signed
	// delta column.
	deltaSpace := 0
	if deltaW > 0 {
		deltaSpace = 1 + deltaW
	}

	const gap = 2
	barW := width - labelW - gap - valueW - deltaSpace - 1
	if barW < 4 {
		barW = 4
	}

	var b strings.Builder
	for i, r := range rows {
		barLen := 0
		if maxCount > 0 {
			barLen = r.count * barW / maxCount
			// Keep non-zero counts visible even when they round to zero.
			if r.count > 0 && barLen == 0 {
				barLen = 1
			}
		}
		bar := lipgloss.NewStyle().
			Foreground(r.color).
			Render(strings.Repeat("█", barLen))
		b.WriteString(fmt.Sprintf("%-*s", labelW, r.label))
		b.WriteString(strings.Repeat(" ", gap))
		b.WriteString(bar)
		b.WriteString(strings.Repeat(" ", barW-barLen+1))
		b.WriteString(fmt.Sprintf("%*d", valueW, r.count))

		if deltaSpace > 0 {
			delta := ""
			if r.delta != 0 {
				delta = fmt.Sprintf("(%+d)", r.delta)
			}
			style := lipgloss.NewStyle()
			if r.delta != 0 {
				style = style.Foreground(deltaColor(r.delta, r.moreIsGood))
			}
			b.WriteString(" ")
			b.WriteString(fmt.Sprintf("%*s", deltaW, style.Render(delta)))
		}

		if i < len(rows)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// deltaColor colors a run-over-run change: green when the move is good and
// red when it is bad, neutral for no change.
func deltaColor(delta int, moreIsGood bool) lipgloss.Color {
	good := delta < 0
	if moreIsGood {
		good = delta > 0
	}
	if good {
		return lipgloss.Color("#9ece6a")
	}
	return lipgloss.Color("#f7768e")
}

// severityChart renders the severity distribution of the latest run as a bar
// chart with per-severity run-over-run deltas.
func (m DashboardModel) severityChart(width int) string {
	counts := m.metrics.Severity
	defs := []struct {
		label string
		sev   domain.Severity
	}{
		{"FATAL", domain.SeverityFatal},
		{"ERROR", domain.SeverityError},
		{"WARN", domain.SeverityWarning},
		{"NOTICE", domain.SeverityNotice},
		{"INFO", domain.SeverityInfo},
		{"PASS", domain.SeveritySuccess},
	}

	rows := make([]barChartRow, 0, len(defs))
	for _, d := range defs {
		r := barChartRow{
			label:      d.label,
			count:      counts.Count(d.sev),
			color:      issueSeverityColor(d.sev),
			moreIsGood: d.sev == domain.SeveritySuccess,
		}
		if m.previous != nil {
			r.hasDelta = true
			r.delta = r.count - m.previous.SeverityCounts.Count(d.sev)
		}
		rows = append(rows, r)
	}
	return barChart(rows, width)
}

// lifecycleChart renders the lifecycle distribution of the latest run as a
// bar chart with per-lifecycle run-over-run deltas.
func (m DashboardModel) lifecycleChart(width int) string {
	counts := m.metrics.Lifecycle
	defs := []struct {
		label      string
		get        func(domain.SnapshotLifecycles) int
		color      lipgloss.Color
		moreIsGood bool
	}{
		{"NEW", func(c domain.SnapshotLifecycles) int { return c.New }, lipgloss.Color("#2ac3de"), false},
		{"OPEN", func(c domain.SnapshotLifecycles) int { return c.Open }, lipgloss.Color("#e0af68"), false},
		{"RESURFACED", func(c domain.SnapshotLifecycles) int { return c.Resurfaced }, lipgloss.Color("#ff9e64"), false},
		{"RESOLVED", func(c domain.SnapshotLifecycles) int { return c.Resolved }, lipgloss.Color("#73daca"), true},
		{"IMPROVED", func(c domain.SnapshotLifecycles) int { return c.Improved }, lipgloss.Color("#9ece6a"), true},
		{"DEGRADED", func(c domain.SnapshotLifecycles) int { return c.Degraded }, lipgloss.Color("#f7768e"), false},
		{"PASSED", func(c domain.SnapshotLifecycles) int { return c.Passed }, lipgloss.Color("#7aa2f7"), true},
	}

	rows := make([]barChartRow, 0, len(defs))
	for _, d := range defs {
		r := barChartRow{
			label:      d.label,
			count:      d.get(counts),
			color:      d.color,
			moreIsGood: d.moreIsGood,
		}
		if m.previous != nil {
			r.hasDelta = true
			r.delta = r.count - d.get(m.previous.LifecycleCounts)
		}
		rows = append(rows, r)
	}
	return barChart(rows, width)
}

// lifecycleMetrics tallies every issue of the audit by its lifecycle state.
func (m DashboardModel) lifecycleMetrics() []widgetMetric {
	counts := m.metrics.Lifecycle
	lifecycleDeltas := func(prev domain.SnapshotLifecycles) []string {
		return []string{
			intDelta(counts.New, prev.New),
			intDelta(counts.Open, prev.Open),
			intDelta(counts.Resurfaced, prev.Resurfaced),
			intDelta(counts.Resolved, prev.Resolved),
			intDelta(counts.Improved, prev.Improved),
			intDelta(counts.Degraded, prev.Degraded),
			intDelta(counts.Passed, prev.Passed),
		}
	}
	var deltas []string
	if m.previous != nil {
		deltas = lifecycleDeltas(m.previous.LifecycleCounts)
	}
	if deltas == nil {
		deltas = make([]string, 7)
	}

	boxes := []metricBox{
		{label: "NEW", color: lipgloss.Color("#2ac3de")},
		{label: "OPEN", color: lipgloss.Color("#e0af68")},
		{label: "RESURFACED", color: lipgloss.Color("#ff9e64")},
		{label: "RESOLVED", color: lipgloss.Color("#73daca")},
		{label: "IMPROVED", color: lipgloss.Color("#9ece6a")},
		{label: "DEGRADED", color: lipgloss.Color("#f7768e")},
		{label: "PASSED", color: lipgloss.Color("#7aa2f7")},
	}
	return metricsOf(boxes,
		withDelta(fmt.Sprintf("%d", counts.New), deltas[0]),
		withDelta(fmt.Sprintf("%d", counts.Open), deltas[1]),
		withDelta(fmt.Sprintf("%d", counts.Resurfaced), deltas[2]),
		withDelta(fmt.Sprintf("%d", counts.Resolved), deltas[3]),
		withDelta(fmt.Sprintf("%d", counts.Improved), deltas[4]),
		withDelta(fmt.Sprintf("%d", counts.Degraded), deltas[5]),
		withDelta(fmt.Sprintf("%d", counts.Passed), deltas[6]),
	)
}

// dashValue renders a metric value or a dash when it is not defined.
func dashValue(v string) string {
	if v == "" {
		return "–"
	}
	return v
}

// timingMetrics reports the wall-clock duration of the run, amortized per
// audited URL and per issue.
func (m DashboardModel) timingMetrics() []widgetMetric {
	urls := m.metrics.TotalURLs()
	issues := m.metrics.TotalIssues()
	total := m.audit.Duration

	curPerURL := time.Duration(0)
	if urls > 0 {
		curPerURL = total / time.Duration(urls)
	}
	curPerIssue := time.Duration(0)
	if issues > 0 {
		curPerIssue = total / time.Duration(issues)
	}

	totalDur := ""
	if total > 0 {
		totalDur = formatDuration(total)
	}
	perURL := ""
	if curPerURL > 0 {
		perURL = formatDuration(curPerURL)
	}
	perIssue := ""
	if curPerIssue > 0 {
		perIssue = formatDuration(curPerIssue)
	}

	var deltaTotal, deltaPerURL, deltaPerIssue string
	if prev := m.previous; prev != nil {
		deltaTotal = durationDelta(total, prev.Timings.Total)
		deltaPerURL = durationDelta(curPerURL, prev.Timings.PerURL)
		deltaPerIssue = durationDelta(curPerIssue, prev.Timings.PerIssue)
	}

	boxes := []metricBox{
		{label: "Total Duration", color: lipgloss.Color("#7aa2f7")},
		{label: "Per URL", color: lipgloss.Color("#bb9af7")},
		{label: "Per Issue", color: lipgloss.Color("#e0af68")},
	}
	return metricsOf(boxes,
		withDelta(dashValue(totalDur), deltaTotal),
		withDelta(dashValue(perURL), deltaPerURL),
		withDelta(dashValue(perIssue), deltaPerIssue),
	)
}

// withDelta appends a signed delta to a metric value, e.g. "30 (-20)".
func withDelta(value, delta string) string {
	if delta == "" {
		return value
	}
	return value + " " + delta
}

// intDelta renders the signed difference of a count against its previous
// run value; equal values render nothing.
func intDelta(current, previous int) string {
	if current == previous {
		return ""
	}
	return fmt.Sprintf("(%+d)", current-previous)
}

// floatDelta renders the signed difference of a score against its previous
// run value; equal values render nothing.
func floatDelta(current, previous float64) string {
	if current == previous {
		return ""
	}
	return fmt.Sprintf("(%+.1f)", current-previous)
}

// durationDelta renders the signed difference of a duration against its
// previous run value; equal values render nothing.
func durationDelta(current, previous time.Duration) string {
	if current == previous {
		return ""
	}
	sign := "+"
	delta := current - previous
	if delta < 0 {
		sign = "-"
		delta = -delta
	}
	return fmt.Sprintf("(%s%s)", sign, formatDuration(delta))
}

func (m DashboardModel) Help() string {
	switch m.state {
	case dashboardRunningState:
		return "Audit in progress  •  Ctrl+C: Quit"
	case dashboardFormState:
		return "Tab: Switch  •  Space: Toggle  •  Ctrl+S: Create  •  Esc: Cancel"
	default:
		return "Ctrl+O: Audits  •  q: Quit"
	}
}
