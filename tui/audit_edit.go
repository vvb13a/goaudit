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

// auditConfigSavedMsg reports the outcome of saving an audit configuration
// (from the audit editor or the checks editor).
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

// AuditEditModel edits the configuration of the current audit (tenant):
// name, description, target URLs and the engine configuration. The check
// selection has its own view.
type AuditEditModel struct {
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
	// checkNames is preserved untouched when saving the config fields.
	checkNames []string
}

// fieldCount is the total number of editable fields (core + engine config).
func fieldCount() int {
	return 3 + len(cfgFields)
}

func NewAuditEditModel(deps Deps) AuditEditModel {
	ti := textinput.New()
	ti.Placeholder = "e.g. Marketing Site Audit"
	ti.CharLimit = 80
	ti.Width = 60
	ti.Focus()

	da := textarea.New()
	da.Placeholder = "What is being audited and why?"
	da.CharLimit = 4096
	da.SetWidth(60)
	da.SetHeight(4)

	ta := textarea.New()
	ta.Placeholder = "https://example.com\nhttps://example.com/pricing"
	ta.CharLimit = 8192
	ta.SetWidth(60)
	ta.SetHeight(6)

	cfg := make([]textinput.Model, len(cfgFields))
	for i, f := range cfgFields {
		ci := textinput.New()
		ci.CharLimit = 200
		ci.Width = 60
		if f.positive {
			ci.Placeholder = "positive integer"
		} else {
			ci.Placeholder = "integer >= 0"
		}
		cfg[i] = ci
	}

	return AuditEditModel{
		deps:      deps,
		name:      ti,
		desc:      da,
		targets:   ta,
		cfgInputs: cfg,
		focus:     0,
	}
}

func (m AuditEditModel) NavigationEnabled() bool {
	return false
}

// Track points the editor at the given audit and marks its fields stale so
// the next activation reloads the stored configuration.
func (m AuditEditModel) Track(id string) AuditEditModel {
	if m.auditID != id {
		m.loaded = false
	}
	m.auditID = id
	return m
}

func (m AuditEditModel) loadCmd(id string) tea.Cmd {
	return func() tea.Msg {
		audit, err := m.deps.AuditService.GetConfig(context.Background(), id)
		return auditConfigLoadedMsg{id: id, audit: audit, err: err}
	}
}

func (m AuditEditModel) Update(msg tea.Msg) (AuditEditModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case auditConfigLoadedMsg:
		if msg.id != m.auditID {
			return m, nil
		}
		m.loaded = true
		if msg.err != nil {
			return m, NotifyDanger(fmt.Sprintf("Failed to load audit: %v", msg.err))
		}
		m.checkNames = msg.audit.CheckNames
		m.name.SetValue(msg.audit.Name)
		m.desc.SetValue(msg.audit.Description)
		m.targets.SetValue(strings.Join(msg.audit.Targets, "\n"))
		m.applyConfig(effectiveConfig(m.deps, msg.audit.Config))
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "shift+tab", "backtab":
			m.focus = (m.focus + 1) % fieldCount()
			m.applyFocus()
			return m, nil
		case "ctrl+s":
			return m.save()
		}
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
func (m *AuditEditModel) applyFocus() {
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
func (m *AuditEditModel) applyConfig(cfg service.Config) {
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

// readConfig parses the config inputs into a typed configuration, rejecting
// invalid values with a user-facing message.
func (m AuditEditModel) readConfig() (service.Config, string) {
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

// save validates the fields and persists the audit configuration.
func (m AuditEditModel) save() (AuditEditModel, tea.Cmd) {
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

	cfg, problem := m.readConfig()
	if problem != "" {
		return m, NotifyDanger(problem)
	}
	configRaw, err := json.Marshal(cfg)
	if err != nil {
		return m, NotifyDanger(fmt.Sprintf("Cannot encode config: %v", err))
	}

	description := m.desc.Value()
	id := m.auditID
	checkNames := append([]string(nil), m.checkNames...)
	return m, func() tea.Msg {
		err := m.deps.AuditService.UpdateConfig(context.Background(), id, name, description, targets, checkNames, configRaw)
		return auditConfigSavedMsg{id: id, name: name, err: err}
	}
}

func (m AuditEditModel) View() string {
	if m.auditID == "" {
		return centerLines(helpStyle.Render("No audit selected. Press Ctrl+O to open the audit switcher."), m.width)
	}
	if !m.loaded {
		return "Loading audit..."
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render("Edit Audit"))
	b.WriteString("\n\n")

	nameLabel := "Name:"
	if m.focus == 0 {
		nameLabel = labelStyle.Render("Name:")
	}
	b.WriteString(nameLabel + "\n" + m.name.View() + "\n\n")

	descLabel := "Description:"
	if m.focus == 1 {
		descLabel = labelStyle.Render("Description:")
	}
	b.WriteString(descLabel + "\n" + m.desc.View() + "\n\n")

	targetsLabel := "Target URLs (one per line):"
	if m.focus == 2 {
		targetsLabel = labelStyle.Render("Target URLs (one per line):")
	}
	b.WriteString(targetsLabel + "\n" + m.targets.View() + "\n\n")

	configLabel := "Engine Configuration:"
	b.WriteString(helpStyle.Render(configLabel))
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

func (m AuditEditModel) Help() string {
	if m.auditID == "" {
		return "Ctrl+O: Audits"
	}
	return "Tab: Switch  •  Ctrl+S: Save  •  Esc: Back  •  Ctrl+O: Audits"
}
