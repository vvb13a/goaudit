package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/vvb13a/goaudit/domain"
	"github.com/vvb13a/goaudit/service"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type checklistsLoadedMsg struct {
	checklists []*domain.Checklist
	err        error
}

type checklistsState int

const (
	checklistsListState checklistsState = iota
	checklistsFormState
	checklistsDeleteState
)

type checklistOption struct {
	name     string
	category domain.Category
	selected bool
}

type checklistForm struct {
	id      string
	name    textinput.Model
	options []checklistOption
	cursor  int
	inName  bool
}

type ChecklistsModel struct {
	deps       Deps
	state      checklistsState
	checklists []*domain.Checklist
	table      table.Model
	form       checklistForm
	loaded     bool
	deleteID   string
	deleteName string
	width      int
	height     int
}

func NewChecklistsModel(deps Deps) ChecklistsModel {
	return ChecklistsModel{
		deps:  deps,
		state: checklistsListState,
		table: table.New(),
	}
}

func (m ChecklistsModel) Init() tea.Cmd {
	return m.loadCmd()
}

func (m ChecklistsModel) Loaded() bool {
	return m.loaded
}

func (m ChecklistsModel) NavigationEnabled() bool {
	return m.state == checklistsListState
}

func (m ChecklistsModel) loadCmd() tea.Cmd {
	return func() tea.Msg {
		checklists, err := m.deps.ChecklistService.List(context.Background())
		return checklistsLoadedMsg{checklists: checklists, err: err}
	}
}

func (m ChecklistsModel) Update(msg tea.Msg) (ChecklistsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.rebuildTable()
		return m, nil

	case checklistsLoadedMsg:
		m.loaded = true
		if msg.err != nil {
			return m, NotifyDanger(fmt.Sprintf("Failed to load checklists: %v", msg.err))
		}
		m.checklists = msg.checklists
		m.rebuildTable()
		return m, nil
	}

	switch m.state {
	case checklistsListState:
		return m.updateList(msg)
	case checklistsFormState:
		return m.updateForm(msg)
	case checklistsDeleteState:
		return m.updateDelete(msg)
	}
	return m, nil
}

func (m ChecklistsModel) updateList(msg tea.Msg) (ChecklistsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q":
			return m, tea.Quit
		case "n":
			m.form = newChecklistForm(m.deps.Registry, nil)
			m.state = checklistsFormState
			return m, nil
		case "e", "l", "right", "enter":
			idx := m.table.Cursor()
			if idx < len(m.checklists) {
				m.form = newChecklistForm(m.deps.Registry, m.checklists[idx])
				m.state = checklistsFormState
			}
			return m, nil
		case "d", "x":
			idx := m.table.Cursor()
			if idx < len(m.checklists) {
				m.deleteID = m.checklists[idx].ID
				m.deleteName = m.checklists[idx].Name
				m.state = checklistsDeleteState
			}
			return m, nil
		case " ":
			idx := m.table.Cursor()
			if idx < len(m.checklists) {
				selected := m.checklists[idx]
				var notice tea.Cmd
				if err := m.deps.ChecklistService.SetActive(context.Background(), selected.ID); err != nil {
					notice = NotifyDanger(fmt.Sprintf("Set active failed: %v", err))
				} else {
					notice = NotifySuccess(fmt.Sprintf("Active checklist set to '%s'", selected.Name))
				}
				return m, tea.Batch(m.loadCmd(), notice)
			}
		}
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m ChecklistsModel) updateForm(msg tea.Msg) (ChecklistsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.state = checklistsListState
			return m, nil
		case "tab", "shift+tab", "backtab":
			m.form.inName = !m.form.inName
			if m.form.inName {
				m.form.name.Focus()
			} else {
				m.form.name.Blur()
			}
			return m, nil
		case "ctrl+s":
			return m.saveForm()
		case "enter":
			if !m.form.inName {
				return m.saveForm()
			}
		case "up", "k":
			if !m.form.inName && m.form.cursor > 0 {
				m.form.cursor--
			}
			return m, nil
		case "down", "j":
			if !m.form.inName && m.form.cursor < len(m.form.options)-1 {
				m.form.cursor++
			}
			return m, nil
		case " ":
			if !m.form.inName && len(m.form.options) > 0 {
				opt := &m.form.options[m.form.cursor]
				opt.selected = !opt.selected
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	if m.form.inName {
		m.form.name, cmd = m.form.name.Update(msg)
	}
	return m, cmd
}

func (m ChecklistsModel) saveForm() (ChecklistsModel, tea.Cmd) {
	name := strings.TrimSpace(m.form.name.Value())
	if name == "" {
		return m, NotifyDanger("Checklist name is required")
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

	hasActive := false
	for _, cl := range m.checklists {
		if cl.IsActive {
			hasActive = true
			break
		}
	}

	checklist := &domain.Checklist{
		ID:         m.form.id,
		Name:       name,
		CheckNames: chosen,
		IsActive:   m.form.id == "" && !hasActive,
	}

	var err error
	if m.form.id == "" {
		err = m.deps.ChecklistService.Create(context.Background(), checklist)
	} else {
		err = m.deps.ChecklistService.Update(context.Background(), checklist)
	}
	if err != nil {
		return m, NotifyDanger(fmt.Sprintf("Save failed: %v", err))
	}

	verb := "created"
	if m.form.id != "" {
		verb = "updated"
	}
	m.state = checklistsListState
	return m, tea.Batch(
		m.loadCmd(),
		NotifySuccess(fmt.Sprintf("Checklist '%s' %s with %d checks!", checklist.Name, verb, len(chosen))),
	)
}

func (m ChecklistsModel) updateDelete(msg tea.Msg) (ChecklistsModel, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch key.String() {
	case "y", "Y":
		var notice tea.Cmd
		if err := m.deps.ChecklistService.Delete(context.Background(), m.deleteID); err != nil {
			notice = NotifyDanger(fmt.Sprintf("Delete failed: %v", err))
		} else {
			notice = NotifySuccess(fmt.Sprintf("Deleted checklist '%s'", m.deleteName))
		}
		m.state = checklistsListState
		return m, tea.Batch(m.loadCmd(), notice)
	default:
		m.deleteID = ""
		m.deleteName = ""
		m.state = checklistsListState
		return m, nil
	}
}

func newChecklistForm(registry *service.CheckRegistry, cl *domain.Checklist) checklistForm {
	ti := textinput.New()
	ti.Placeholder = "e.g. SEO & Security Checklist"
	ti.CharLimit = 50
	ti.Width = 35
	ti.Focus()

	var options []checklistOption
	for _, c := range registry.All() {
		selected := true
		if cl != nil {
			selected = cl.HasCheck(c.Info().Name)
		}
		options = append(options, checklistOption{
			name:     c.Info().Name,
			category: c.Info().Category,
			selected: selected,
		})
	}

	form := checklistForm{
		name:    ti,
		options: options,
		inName:  true,
	}
	if cl != nil {
		form.id = cl.ID
		form.name.SetValue(cl.Name)
	}
	return form
}

func (m ChecklistsModel) View() string {
	switch m.state {
	case checklistsListState:
		return m.listView()
	case checklistsFormState:
		return overlay(m.listView(), m.formView(), m.width, m.height)
	case checklistsDeleteState:
		return overlay(m.listView(), m.deleteView(), m.width, m.height)
	}
	return m.listView()
}

func (m ChecklistsModel) listView() string {
	if !m.loaded {
		return "Loading checklists..."
	}
	if len(m.checklists) == 0 {
		return "No checklists found. Press 'n' to create your first checklist!"
	}
	return m.table.View()
}

func (m ChecklistsModel) formView() string {
	var b strings.Builder

	title := "Create Checklist"
	if m.form.id != "" {
		title = "Edit Checklist"
	}
	b.WriteString(titleStyle.Render(title))
	b.WriteString("\n\n")

	nameLabel := "Checklist Name:"
	if m.form.inName {
		nameLabel = labelStyle.Render("Checklist Name:")
	}
	b.WriteString(nameLabel + "\n" + m.form.name.View() + "\n\n")

	checksLabel := "Select Checks (Space to toggle):"
	if !m.form.inName {
		checksLabel = labelStyle.Render("Select Checks (Space to toggle):")
	}
	b.WriteString(checksLabel + "\n")

	for i, opt := range m.form.options {
		cursor := "  "
		if !m.form.inName && m.form.cursor == i {
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

func (m ChecklistsModel) deleteView() string {
	return fmt.Sprintf("Delete checklist '%s'? This cannot be undone.", m.deleteName)
}

func (m ChecklistsModel) Help() string {
	switch m.state {
	case checklistsFormState:
		return "Tab: Switch Focus  •  Space: Toggle  •  Ctrl+S/Enter: Save  •  Esc: Cancel"
	case checklistsDeleteState:
		return "y: Delete  •  any other key: Cancel"
	default:
		return "Space: Set Active  •  Enter: Edit  •  n: New  •  d: Delete  •  q: Quit"
	}
}

func (m *ChecklistsModel) rebuildTable() {
	columns := []table.Column{
		{Title: "Active", Width: 8},
		{Title: "Checklist Name", Width: 28},
		{Title: "Checks", Width: 10},
		{Title: "Created", Width: 18},
	}

	rows := make([]table.Row, 0, len(m.checklists))
	for _, cl := range m.checklists {
		active := ""
		if cl.IsActive {
			active = "active"
		}
		rows = append(rows, table.Row{
			active,
			cl.Name,
			fmt.Sprintf("%d", len(cl.CheckNames)),
			cl.CreatedAt.Format("2006-01-02 15:04"),
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
	if m.width > 0 {
		t.SetWidth(m.width - 2)
	}
	m.table = t
}
