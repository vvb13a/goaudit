package tui

import (
	"context"
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

// dashboardLoadedMsg carries the fully hydrated audit whose metrics are
// shown on the dashboard. id guards against stale responses.
type dashboardLoadedMsg struct {
	id    string
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
		audit, err := m.deps.AuditService.GetByID(context.Background(), id)
		return dashboardLoadedMsg{id: id, audit: audit, err: err}
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
	}
	if m.loading {
		return m, m.loadCmd(id)
	}
	return m, nil
}

// showTenant fills the dashboard with an already fully hydrated audit,
// without a round trip to the store. Used right after a run completes.
func (m DashboardModel) showTenant(a *domain.Audit) DashboardModel {
	m.state = dashboardListState
	m.auditID = a.ID
	m.loading = false
	m.audit = a
	return m
}

// clearTenant drops the audit currently shown on the dashboard.
func (m DashboardModel) clearTenant() DashboardModel {
	m.auditID = ""
	m.loading = false
	m.audit = nil
	m.state = dashboardListState
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

	checks, err := m.deps.Registry.Resolve(chosen)
	if err != nil {
		return m, NotifyDanger(fmt.Sprintf("Cannot run audit: %v", err))
	}

	cfg := effectiveConfig(m.deps, nil)
	return m.startAuditRun(name, "", targets, checks, cfg, "", fmt.Sprintf("Running audit '%s'", name))
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
		},
		{
			title:       "Lifecycles",
			description: "How every issue evolved between the previous and the latest run.",
			accent:      lipgloss.Color("#bb9af7"),
			metrics:     m.lifecycleMetrics(),
		},
		{
			title:       "Timing",
			description: "Wall-clock duration of the run, and its share per audited URL and per issue.",
			accent:      lipgloss.Color("#9ece6a"),
			metrics:     m.timingMetrics(),
		},
	}

	for i, s := range sections {
		b.WriteString(metricsSection(w, s.accent, s.title, s.description, s.metrics))
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
// top and the metric boxes below.
func metricsSection(width int, accent lipgloss.Color, title, description string, metrics []widgetMetric) string {
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

// totalIssues counts the issues of every audited URL of the audit.
func (m DashboardModel) totalIssues() int {
	total := 0
	for _, u := range m.audit.Urls {
		total += len(u.Issues)
	}
	return total
}

// totalChecks returns the number of checks that ran, falling back to the
// distinct checks observed in the issues for audits without stored check
// names.
func (m DashboardModel) totalChecks() int {
	if n := len(m.audit.CheckNames); n > 0 {
		return n
	}
	seen := make(map[string]struct{})
	for _, u := range m.audit.Urls {
		for _, iss := range u.Issues {
			seen[iss.CheckName] = struct{}{}
		}
	}
	return len(seen)
}

func (m DashboardModel) urlStateCounts() (newURLs, activeURLs, missingURLs int) {
	for _, u := range m.audit.Urls {
		switch u.State {
		case domain.UrlStateNew:
			newURLs++
		case domain.UrlStateActive:
			activeURLs++
		case domain.UrlStateMissing:
			missingURLs++
		}
	}
	return newURLs, activeURLs, missingURLs
}

func (m DashboardModel) severityCounts() map[domain.Severity]int {
	counts := make(map[domain.Severity]int)
	for _, u := range m.audit.Urls {
		for _, iss := range u.Issues {
			counts[iss.Severity]++
		}
	}
	return counts
}

func (m DashboardModel) lifecycleCounts() map[domain.IssueLifecycle]int {
	counts := make(map[domain.IssueLifecycle]int)
	for _, u := range m.audit.Urls {
		for _, iss := range u.Issues {
			counts[iss.Lifecycle]++
		}
	}
	return counts
}

// overviewMetrics are the hero totals: audited URLs, issues, checks and the
// overall audit score.
func (m DashboardModel) overviewMetrics() []widgetMetric {
	boxes := []metricBox{
		{label: "Total URLs", color: lipgloss.Color("#7aa2f7")},
		{label: "Total Issues", color: lipgloss.Color("#9ece6a")},
		{label: "Total Checks", color: lipgloss.Color("#e0af68")},
		{label: "Score", color: auditScoreColor(m.audit.Score)},
	}
	return metricsOf(boxes,
		fmt.Sprintf("%d", len(m.audit.Urls)),
		fmt.Sprintf("%d", m.totalIssues()),
		fmt.Sprintf("%d", m.totalChecks()),
		fmt.Sprintf("%.1f", m.audit.Score),
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
	newURLs, activeURLs, missingURLs := m.urlStateCounts()
	boxes := []metricBox{
		{label: "New", color: lipgloss.Color("#2ac3de")},
		{label: "Active", color: lipgloss.Color("#9ece6a")},
		{label: "Missing", color: lipgloss.Color("#f7768e")},
	}
	return metricsOf(boxes,
		fmt.Sprintf("%d", newURLs),
		fmt.Sprintf("%d", activeURLs),
		fmt.Sprintf("%d", missingURLs),
	)
}

// severityMetrics tallies every issue of the audit by severity.
func (m DashboardModel) severityMetrics() []widgetMetric {
	counts := m.severityCounts()
	boxes := []metricBox{
		{label: "FATAL", color: issueSeverityColor(domain.SeverityFatal)},
		{label: "ERROR", color: issueSeverityColor(domain.SeverityError)},
		{label: "WARN", color: issueSeverityColor(domain.SeverityWarning)},
		{label: "NOTICE", color: issueSeverityColor(domain.SeverityNotice)},
		{label: "INFO", color: issueSeverityColor(domain.SeverityInfo)},
		{label: "PASS", color: issueSeverityColor(domain.SeveritySuccess)},
	}
	return metricsOf(boxes,
		fmt.Sprintf("%d", counts[domain.SeverityFatal]),
		fmt.Sprintf("%d", counts[domain.SeverityError]),
		fmt.Sprintf("%d", counts[domain.SeverityWarning]),
		fmt.Sprintf("%d", counts[domain.SeverityNotice]),
		fmt.Sprintf("%d", counts[domain.SeverityInfo]),
		fmt.Sprintf("%d", counts[domain.SeveritySuccess]),
	)
}

// lifecycleMetrics tallies every issue of the audit by its lifecycle state.
func (m DashboardModel) lifecycleMetrics() []widgetMetric {
	counts := m.lifecycleCounts()
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
		fmt.Sprintf("%d", counts[domain.LifecycleNew]),
		fmt.Sprintf("%d", counts[domain.LifecycleOpen]),
		fmt.Sprintf("%d", counts[domain.LifecycleResurfaced]),
		fmt.Sprintf("%d", counts[domain.LifecycleResolved]),
		fmt.Sprintf("%d", counts[domain.LifecycleImproved]),
		fmt.Sprintf("%d", counts[domain.LifecycleDegraded]),
		fmt.Sprintf("%d", counts[domain.LifecyclePassed]),
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
	urls := len(m.audit.Urls)
	issues := m.totalIssues()
	total := m.audit.Duration

	perURL := ""
	if urls > 0 && total > 0 {
		perURL = formatDuration(total / time.Duration(urls))
	}
	perIssue := ""
	if issues > 0 && total > 0 {
		perIssue = formatDuration(total / time.Duration(issues))
	}
	totalDur := ""
	if total > 0 {
		totalDur = formatDuration(total)
	}

	boxes := []metricBox{
		{label: "Total Duration", color: lipgloss.Color("#7aa2f7")},
		{label: "Per URL", color: lipgloss.Color("#bb9af7")},
		{label: "Per Issue", color: lipgloss.Color("#e0af68")},
	}
	return metricsOf(boxes,
		dashValue(totalDur),
		dashValue(perURL),
		dashValue(perIssue),
	)
}

func (m DashboardModel) Help() string {
	switch m.state {
	case dashboardRunningState:
		return "Audit in progress  •  Ctrl+C: Quit"
	case dashboardFormState:
		return "Tab: Switch  •  Space: Toggle  •  Ctrl+S: Run  •  Esc: Cancel"
	default:
		return "Ctrl+O: Audits  •  q: Quit"
	}
}
