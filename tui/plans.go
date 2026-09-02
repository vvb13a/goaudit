package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/vvb13a/goaudit/domain"
	"github.com/vvb13a/goaudit/service"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type plansLoadedMsg struct {
	plans []*domain.Plan
	err   error
}

type plansState int

const (
	plansListState plansState = iota
	plansFormState
	plansDeleteState
)

type planForm struct {
	id          string
	name        textinput.Model
	urls        textarea.Model
	nameFocused bool
}

type PlansModel struct {
	svc        *service.PlanService
	state      plansState
	plans      []*domain.Plan
	table      table.Model
	form       planForm
	loaded     bool
	status     string
	deleteID   string
	deleteName string
	width      int
	height     int
}

func NewPlansModel(svc *service.PlanService) PlansModel {
	return PlansModel{
		svc:   svc,
		state: plansListState,
		table: table.New(),
	}
}

func (m PlansModel) Init() tea.Cmd {
	return m.loadCmd()
}

func (m PlansModel) Loaded() bool {
	return m.loaded
}

func (m PlansModel) NavigationEnabled() bool {
	return m.state == plansListState
}

func (m PlansModel) loadCmd() tea.Cmd {
	return func() tea.Msg {
		plans, err := m.svc.List(context.Background())
		return plansLoadedMsg{plans: plans, err: err}
	}
}

func (m PlansModel) Update(msg tea.Msg) (PlansModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.rebuildTable()
		return m, nil

	case plansLoadedMsg:
		m.loaded = true
		if msg.err != nil {
			m.status = fmt.Sprintf("Failed to load plans: %v", msg.err)
			return m, nil
		}
		m.plans = msg.plans
		m.rebuildTable()
		return m, nil
	}

	switch m.state {
	case plansListState:
		return m.updateList(msg)
	case plansFormState:
		return m.updateForm(msg)
	case plansDeleteState:
		return m.updateDelete(msg)
	}
	return m, nil
}

func (m PlansModel) updateList(msg tea.Msg) (PlansModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q":
			return m, tea.Quit
		case "n", "c":
			m.form = newPlanForm(nil)
			m.state = plansFormState
			return m, nil
		case "e", "l", "right", "enter":
			idx := m.table.Cursor()
			if idx < len(m.plans) {
				m.form = newPlanForm(m.plans[idx])
				m.state = plansFormState
			}
			return m, nil
		case "d", "x":
			idx := m.table.Cursor()
			if idx < len(m.plans) {
				m.deleteID = m.plans[idx].ID
				m.deleteName = m.plans[idx].Name
				m.state = plansDeleteState
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m PlansModel) updateForm(msg tea.Msg) (PlansModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.state = plansListState
			m.status = ""
			return m, nil
		case "tab", "shift+tab", "backtab":
			m.form.nameFocused = !m.form.nameFocused
			if m.form.nameFocused {
				m.form.name.Focus()
				m.form.urls.Blur()
			} else {
				m.form.name.Blur()
				m.form.urls.Focus()
			}
			return m, nil
		case "ctrl+s":
			return m.saveForm()
		}
	}

	var cmd tea.Cmd
	if m.form.nameFocused {
		m.form.name, cmd = m.form.name.Update(msg)
	} else {
		m.form.urls, cmd = m.form.urls.Update(msg)
	}
	return m, cmd
}

func (m PlansModel) saveForm() (PlansModel, tea.Cmd) {
	name := strings.TrimSpace(m.form.name.Value())
	if name == "" {
		m.status = "Plan name is required"
		return m, nil
	}

	var rawURLs []string
	for _, line := range strings.Split(m.form.urls.Value(), "\n") {
		u := strings.TrimSpace(line)
		if u == "" {
			continue
		}
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			u = "https://" + u
		}
		rawURLs = append(rawURLs, u)
	}
	if len(rawURLs) == 0 {
		m.status = "Add at least one target URL"
		return m, nil
	}

	plan := &domain.Plan{ID: m.form.id, Name: name, URLs: rawURLs}

	var err error
	if m.form.id == "" {
		err = m.svc.Create(context.Background(), plan)
	} else {
		err = m.svc.Update(context.Background(), plan)
	}
	if err != nil {
		m.status = fmt.Sprintf("Save failed: %v", err)
		return m, nil
	}

	verb := "created"
	if m.form.id != "" {
		verb = "updated"
	}
	m.status = fmt.Sprintf("Plan '%s' %s with %d URLs!", plan.Name, verb, len(plan.URLs))
	m.state = plansListState
	return m, m.loadCmd()
}

func (m PlansModel) updateDelete(msg tea.Msg) (PlansModel, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch key.String() {
	case "y", "Y":
		if err := m.svc.Delete(context.Background(), m.deleteID); err != nil {
			m.status = fmt.Sprintf("Delete failed: %v", err)
		} else {
			m.status = fmt.Sprintf("Deleted plan '%s'", m.deleteName)
		}
		m.state = plansListState
		return m, m.loadCmd()
	default:
		m.deleteID = ""
		m.deleteName = ""
		m.state = plansListState
		return m, nil
	}
}

func newPlanForm(p *domain.Plan) planForm {
	ti := textinput.New()
	ti.Placeholder = "e.g. Marketing & Checkout Pages"
	ti.CharLimit = 50
	ti.Width = 40
	ti.Focus()

	ta := textarea.New()
	ta.Placeholder = "https://example.com\nhttps://example.com/pricing"
	ta.CharLimit = 4096
	ta.SetWidth(50)
	ta.SetHeight(6)

	form := planForm{
		name:        ti,
		urls:        ta,
		nameFocused: true,
	}
	if p != nil {
		form.id = p.ID
		form.name.SetValue(p.Name)
		form.urls.SetValue(strings.Join(p.URLs, "\n"))
	}
	return form
}

func (m PlansModel) View() string {
	var b strings.Builder
	switch m.state {
	case plansListState:
		b.WriteString(m.listView())
	case plansFormState:
		b.WriteString(m.formView())
	case plansDeleteState:
		b.WriteString(m.deleteView())
	}
	return b.String()
}

func (m PlansModel) listView() string {
	var b strings.Builder

	if !m.loaded {
		b.WriteString("Loading plans...")
	} else if len(m.plans) == 0 {
		b.WriteString("No plans defined yet. Press 'n' to define your first target plan!")
	} else {
		b.WriteString(m.table.View())
	}

	if m.status != "" {
		b.WriteString("\n\n")
		b.WriteString(statusLine(m.status))
	}

	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("n: New Plan  •  Enter: Edit  •  d: Delete  •  q: Quit"))
	return b.String()
}

func (m PlansModel) formView() string {
	var b strings.Builder

	title := "Define Target Plan"
	if m.form.id != "" {
		title = "Edit Target Plan"
	}
	b.WriteString(titleStyle.Render(title))
	b.WriteString("\n\n")

	nameLabel := "Plan Name:"
	if m.form.nameFocused {
		nameLabel = labelStyle.Render("Plan Name:")
	}
	b.WriteString(nameLabel + "\n" + m.form.name.View() + "\n\n")

	urlsLabel := "Target URLs (one per line):"
	if !m.form.nameFocused {
		urlsLabel = labelStyle.Render("Target URLs (one per line):")
	}
	b.WriteString(urlsLabel + "\n" + m.form.urls.View() + "\n")

	if m.status != "" {
		b.WriteString("\n" + statusLine(m.status))
	}

	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("Tab: Switch Focus  •  Ctrl+S: Save  •  Esc: Cancel"))
	return b.String()
}

func (m PlansModel) deleteView() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Delete plan '%s'? This cannot be undone.", m.deleteName))
	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("y: Delete  •  any other key: Cancel"))
	return b.String()
}

func (m *PlansModel) rebuildTable() {
	columns := []table.Column{
		{Title: "Plan Name", Width: 26},
		{Title: "URLs", Width: 8},
		{Title: "Sample URL", Width: 34},
		{Title: "Created", Width: 18},
	}

	rows := make([]table.Row, 0, len(m.plans))
	for _, p := range m.plans {
		sampleURL := "-"
		if len(p.URLs) > 0 {
			sampleURL = p.URLs[0]
		}
		rows = append(rows, table.Row{
			p.Name,
			fmt.Sprintf("%d", len(p.URLs)),
			sampleURL,
			p.CreatedAt.Format("2006-01-02 15:04"),
		})
	}

	height := 10
	if m.height > 0 {
		height = m.height - 7
	}
	if height < 3 {
		height = 3
	}
	if height > 30 {
		height = 30
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(height),
	)
	t.SetStyles(tableStyle())
	if m.width > 0 {
		t.SetWidth(m.width - 2)
	}
	m.table = t
}
