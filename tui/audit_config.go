package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/charmbracelet/lipgloss"

	"github.com/vvb13a/goaudit/domain"
	"github.com/vvb13a/goaudit/service"
)

// auditConfigLoadedMsg carries the stored configuration of the audit being
// edited. id guards against stale responses.
type auditConfigLoadedMsg struct {
	id    string
	audit *domain.Audit
	err   error
}

// auditConfigSavedMsg reports the outcome of saving an audit configuration.
type auditConfigSavedMsg struct {
	id   string
	name string
	err  error
}

// cfgField describes one editable engine setting of the run configuration.
type cfgField struct {
	title    string // label shown in the editor
	positive bool   // value must be a positive integer (delay allows zero)
}

// cfgFields lists the editable engine settings in editor order.
var cfgFields = []cfgField{
	{title: "Max concurrency", positive: true},
	{title: "Request delay (ms)", positive: false},
	{title: "HTTP timeout (s)", positive: true},
	{title: "Max sitemap depth", positive: true},
	{title: "Link cache TTL (min)", positive: true},
	{title: "User agent"},
}

// Focused panes of the config view: the audit fields on the left, the
// running checks on the right.
const (
	configFieldsPane = iota
	configChecksPane
)

// AuditConfigModel edits the full configuration of the current audit (tenant):
// name, description, target URLs, the engine configuration and the checks
// to run. The fields live in the left pane, the check list in the right.
type AuditConfigModel struct {
	deps    Deps
	auditID string
	loaded  bool
	name    textinput.Model
	desc    textarea.Model
	targets textarea.Model
	// cfgInputs holds one textinput per cfgField entry.
	cfgInputs []textinput.Model
	focus     int // 0..2 core fields, then the cfgFields entries
	width     int
	height    int

	// options holds every registered check, toggled from the right pane.
	options []auditOption
	cursor  int
	pane    int // configFieldsPane or configChecksPane
}

// configFieldCount is the total number of editable fields of the left pane.
func configFieldCount() int {
	return 3 + len(cfgFields)
}

func NewAuditConfigModel(deps Deps) AuditConfigModel {
	ti := textinput.New()
	ti.Placeholder = "e.g. Marketing Site Audit"
	ti.CharLimit = 80
	ti.Focus()

	da := textarea.New()
	da.Placeholder = "What is being audited and why?"
	da.CharLimit = 4096
	da.SetHeight(4)

	ta := textarea.New()
	ta.Placeholder = "https://example.com\nhttps://example.com/pricing"
	ta.CharLimit = 8192
	ta.SetHeight(6)

	cfg := make([]textinput.Model, len(cfgFields))
	for i, f := range cfgFields {
		ci := textinput.New()
		ci.CharLimit = 200
		if f.positive {
			ci.Placeholder = "positive integer"
		} else {
			ci.Placeholder = "integer >= 0"
		}
		cfg[i] = ci
	}

	return AuditConfigModel{
		deps:      deps,
		name:      ti,
		desc:      da,
		targets:   ta,
		cfgInputs: cfg,
		focus:     0,
		pane:      configFieldsPane,
	}
}

func (m AuditConfigModel) NavigationEnabled() bool {
	return false
}

// Track points the editor at the given audit and marks it stale so the next
// activation reloads the stored configuration.
func (m AuditConfigModel) Track(id string) AuditConfigModel {
	if m.auditID != id {
		m.loaded = false
		m.options = nil
		m.cursor = 0
	}
	m.auditID = id
	return m
}

func (m AuditConfigModel) loadCmd(id string) tea.Cmd {
	return func() tea.Msg {
		audit, err := m.deps.AuditService.GetConfig(context.Background(), id)
		return auditConfigLoadedMsg{id: id, audit: audit, err: err}
	}
}

// paneInner returns the inner width of the given pane for the current width.
func (m AuditConfigModel) paneInner(pane int) int {
	if pane == configFieldsPane {
		leftW := m.width * 2 / 5
		if leftW > 70 {
			leftW = 70
		}
		if leftW < 30 {
			leftW = 30
		}
		return leftW - 2
	}
	return m.width - m.leftPaneWidth() - 2
}

// leftPaneWidth is the outer width of the fields pane.
func (m AuditConfigModel) leftPaneWidth() int {
	return m.paneInner(configFieldsPane) + 2
}

// applyWidths sizes the field widgets to the fields pane.
func (m *AuditConfigModel) applyWidths() {
	inner := m.paneInner(configFieldsPane) - 4
	if inner < 12 {
		inner = 12
	}
	m.name.Width = inner
	m.desc.SetWidth(inner)
	m.targets.SetWidth(inner)
	for i := range m.cfgInputs {
		m.cfgInputs[i].Width = inner
	}
}

func (m AuditConfigModel) Update(msg tea.Msg) (AuditConfigModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.applyWidths()
		return m, nil

	case auditConfigLoadedMsg:
		if msg.id != m.auditID {
			return m, nil
		}
		m.loaded = true
		if msg.err != nil {
			return m, NotifyDanger(fmt.Sprintf("Failed to load audit: %v", msg.err))
		}
		m.name.SetValue(msg.audit.Name)
		m.desc.SetValue(msg.audit.Description)
		m.targets.SetValue(strings.Join(msg.audit.Targets, "\n"))
		m.applyConfig(effectiveConfig(m.deps, msg.audit.Config))
		m.options = m.buildOptions(msg.audit.CheckNames)
		if m.cursor >= len(m.options) {
			m.cursor = len(m.options) - 1
		}
		if m.cursor < 0 {
			m.cursor = 0
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "left", "h":
			if m.pane == configChecksPane {
				m.pane = configFieldsPane
			}
			return m, nil
		case "right", "l":
			if m.pane == configFieldsPane {
				m.pane = configChecksPane
			}
			return m, nil
		case "ctrl+s":
			return m.save()
		}

		// Check list keys only apply while the checks pane is focused.
		if m.pane == configChecksPane {
			switch msg.String() {
			case "up", "k":
				if m.cursor > 0 {
					m.cursor--
				}
				return m, nil
			case "down", "j":
				if m.cursor < len(m.options)-1 {
					m.cursor++
				}
				return m, nil
			case " ":
				if len(m.options) > 0 {
					opt := &m.options[m.cursor]
					opt.selected = !opt.selected
				}
				return m, nil
			case "enter":
				return m.save()
			}
			return m, nil
		}

		// Field cycling only applies to the fields pane.
		switch msg.String() {
		case "tab", "shift+tab", "backtab":
			m.focus = (m.focus + 1) % configFieldCount()
			m.applyFocus()
			return m, nil
		}
	}

	if m.pane == configChecksPane {
		return m, nil
	}
	var cmd tea.Cmd
	switch {
	case m.focus == 0:
		m.name, cmd = m.name.Update(msg)
	case m.focus == 1:
		m.desc, cmd = m.desc.Update(msg)
	case m.focus == 2:
		m.targets, cmd = m.targets.Update(msg)
	default:
		idx := m.focus - 3
		if idx >= 0 && idx < len(m.cfgInputs) {
			m.cfgInputs[idx], cmd = m.cfgInputs[idx].Update(msg)
		}
	}
	return m, cmd
}

// applyFocus focuses the widget of the current field, blurring the others.
func (m *AuditConfigModel) applyFocus() {
	switch m.focus {
	case 0:
		m.name.Focus()
	case 1:
		m.desc.Focus()
	case 2:
		m.targets.Focus()
	default:
		idx := m.focus - 3
		if idx >= 0 && idx < len(m.cfgInputs) {
			m.cfgInputs[idx].Focus()
		}
	}
}

// applyConfig fills the config inputs from an effective configuration.
func (m *AuditConfigModel) applyConfig(cfg service.Config) {
	values := []string{
		strconv.Itoa(cfg.MaxConcurrency),
		strconv.Itoa(cfg.RequestDelayMs),
		strconv.Itoa(cfg.HTTPTimeoutSec),
		strconv.Itoa(cfg.MaxSitemapDepth),
		strconv.Itoa(cfg.LinkCacheTTLMin),
		cfg.UserAgent,
	}
	for i, v := range values {
		if i < len(m.cfgInputs) {
			m.cfgInputs[i].SetValue(v)
		}
	}
}

// buildOptions renders every registered check, preselected from the audit's
// stored check names. Audits without stored names (legacy rows) default to
// all checks.
func (m AuditConfigModel) buildOptions(checkNames []string) []auditOption {
	selected := make(map[string]struct{}, len(checkNames))
	for _, n := range checkNames {
		selected[n] = struct{}{}
	}
	useAll := len(checkNames) == 0

	var options []auditOption
	for _, c := range m.deps.Registry.All() {
		name := c.Info().Name
		options = append(options, auditOption{
			name:     name,
			category: c.Info().Category,
			selected: useAll,
		})
		if _, ok := selected[name]; ok {
			options[len(options)-1].selected = true
		}
	}
	return options
}

// readConfig parses the config inputs into a typed configuration, rejecting
// invalid values with a user-facing message.
func (m AuditConfigModel) readConfig() (service.Config, string) {
	cfg := *service.DefaultConfig()

	parseInt := func(idx int) (int, bool) {
		raw := strings.TrimSpace(m.cfgInputs[idx].Value())
		n, err := strconv.Atoi(raw)
		if err != nil {
			return 0, false
		}
		return n, true
	}

	if n, ok := parseInt(0); !ok || n <= 0 {
		return cfg, cfgFields[0].title + " must be a positive integer"
	} else {
		cfg.MaxConcurrency = n
	}
	if n, ok := parseInt(1); !ok || n < 0 {
		return cfg, cfgFields[1].title + " must be an integer >= 0"
	} else {
		cfg.RequestDelayMs = n
	}
	if n, ok := parseInt(2); !ok || n <= 0 {
		return cfg, cfgFields[2].title + " must be a positive integer"
	} else {
		cfg.HTTPTimeoutSec = n
	}
	if n, ok := parseInt(3); !ok || n <= 0 {
		return cfg, cfgFields[3].title + " must be a positive integer"
	} else {
		cfg.MaxSitemapDepth = n
	}
	if n, ok := parseInt(4); !ok || n <= 0 {
		return cfg, cfgFields[4].title + " must be a positive integer"
	} else {
		cfg.LinkCacheTTLMin = n
	}

	ua := strings.TrimSpace(m.cfgInputs[5].Value())
	if ua == "" {
		return cfg, "User agent must not be empty"
	}
	cfg.UserAgent = ua
	return cfg, ""
}

// save validates the fields and check selection, then persists the whole
// audit configuration.
func (m AuditConfigModel) save() (AuditConfigModel, tea.Cmd) {
	if m.auditID == "" {
		return m, NotifyDanger("No audit selected")
	}

	name := strings.TrimSpace(m.name.Value())
	if name == "" {
		return m, NotifyDanger("Audit name is required")
	}

	var targets []string
	seen := make(map[string]struct{})
	for _, line := range strings.Split(m.targets.Value(), "\n") {
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
	for _, opt := range m.options {
		if opt.selected {
			chosen = append(chosen, opt.name)
		}
	}
	if len(chosen) == 0 {
		return m, NotifyDanger("Select at least one check")
	}

	cfg, problem := m.readConfig()
	if problem != "" {
		return m, NotifyDanger(problem)
	}
	configRaw, err := json.Marshal(cfg)
	if err != nil {
		return m, NotifyDanger(fmt.Sprintf("Cannot encode config: %v", err))
	}

	id := m.auditID
	description := m.desc.Value()
	return m, func() tea.Msg {
		err := m.deps.AuditService.UpdateConfig(context.Background(), id, name, description, targets, chosen, configRaw)
		return auditConfigSavedMsg{id: id, name: name, err: err}
	}
}

// ---- Rendering ----

func (m AuditConfigModel) View() string {
	if m.auditID == "" {
		return centerLines(helpStyle.Render("No audit selected. Press Ctrl+O to open the audit switcher."), m.width)
	}
	if !m.loaded {
		return "Loading configuration..."
	}
	if m.width < 46 || m.height < 10 {
		return "Terminal too small for the config view."
	}

	fieldsInner := m.paneInner(configFieldsPane)
	checksInner := m.paneInner(configChecksPane)
	rows := m.height - 2
	if rows < 1 {
		rows = 1
	}

	left := m.padLines(m.fieldsView(), fieldsInner, rows)
	right := m.padLines(m.checksView(), checksInner, rows)

	leftBox := paneBox(left, fieldsInner+2, m.height, m.pane == configFieldsPane)
	rightBox := paneBox(right, checksInner+2, m.height, m.pane == configChecksPane)

	leftLines := strings.Split(leftBox, "\n")
	rightLines := strings.Split(rightBox, "\n")
	var b strings.Builder
	for i := 0; i < m.height; i++ {
		b.WriteString(leftLines[i])
		b.WriteString(rightLines[i])
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// fieldsView renders the audit fields of the left pane.
func (m AuditConfigModel) fieldsView() string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Render("Audit Configuration"))
	b.WriteString("\n\n")

	label := func(text string, focused bool) string {
		if focused {
			return labelStyle.Render(text)
		}
		return text
	}

	b.WriteString(label("Name:", m.focus == 0) + "\n" + m.name.View() + "\n\n")
	b.WriteString(label("Description:", m.focus == 1) + "\n" + m.desc.View() + "\n\n")
	b.WriteString(label("Target URLs (one per line):", m.focus == 2) + "\n" + m.targets.View() + "\n\n")

	b.WriteString(helpStyle.Render("Engine Configuration:"))
	b.WriteString("\n")
	for i, f := range cfgFields {
		rowLabel := f.title + ":"
		if m.focus == i+3 {
			rowLabel = labelStyle.Render(rowLabel)
		}
		b.WriteString(rowLabel + "\n" + m.cfgInputs[i].View() + "\n")
	}
	return b.String()
}

// checksView renders the running checks of the right pane.
func (m AuditConfigModel) checksView() string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Render("Running Checks"))
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("Space toggles the selected check."))
	b.WriteString("\n\n")

	for i, opt := range m.options {
		cursor := "  "
		if m.pane == configChecksPane && i == m.cursor {
			cursor = labelStyle.Render("> ")
		}
		checked := "[ ]"
		if opt.selected {
			checked = labelStyle.Render("[x]")
		}
		cat := helpStyle.Render(string(opt.category))
		b.WriteString(fmt.Sprintf("%s%s %-25s %s\n", cursor, checked, opt.name, cat))
	}

	selected := 0
	for _, opt := range m.options {
		if opt.selected {
			selected++
		}
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render(fmt.Sprintf("%d of %d checks selected.", selected, len(m.options))))
	return b.String()
}

// padLines pads or truncates content lines to exactly the pane inner size.
func (m AuditConfigModel) padLines(content string, w, height int) string {
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

func (m AuditConfigModel) Help() string {
	if m.auditID == "" {
		return "Ctrl+O: Audits"
	}
	switch m.pane {
	case configChecksPane:
		return "←: Fields  •  ↑/↓: Move  •  Space: Toggle  •  Enter/Ctrl+S: Save  •  Esc: Back  •  Ctrl+O: Audits"
	default:
		return "→: Checks  •  Tab: Field  •  Ctrl+S: Save  •  Esc: Back  •  Ctrl+O: Audits"
	}
}
