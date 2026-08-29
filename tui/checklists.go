package tui

import (
	"fmt"
	"strings"

	"github.com/vvb13a/goaudit/data"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
)

func BuildChecklistsTable(checklists []*data.Checklist) table.Model {
	columns := []table.Column{
		{Title: "Default", Width: 10},
		{Title: "Checklist Name", Width: 26},
		{Title: "Checks Count", Width: 16},
		{Title: "Created", Width: 18},
	}

	var rows []table.Row
	for _, c := range checklists {
		status := " "
		if c.IsDefault {
			status = "● [Default]"
		}

		rows = append(rows, table.Row{
			status,
			c.Name,
			fmt.Sprintf("%d checks", len(c.CheckNames)),
			c.CreatedAt.Format("2006-01-02 15:04"),
		})
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(10),
	)
	t.SetStyles(TableStyle())
	return t
}

type CheckOption struct {
	Check    data.Check
	Selected bool
}

type CreateChecklistForm struct {
	ID        string // Populated if editing existing checklist
	NameInput textinput.Model
	Options   []CheckOption
	Cursor    int
	InName    bool
}

func NewCreateChecklistForm(allChecks []data.Check) CreateChecklistForm {
	ti := textinput.New()
	ti.Placeholder = "e.g. SEO & Security Checklist"
	ti.CharLimit = 50
	ti.Width = 35
	ti.Focus()

	var options []CheckOption
	for _, c := range allChecks {
		options = append(options, CheckOption{
			Check:    c,
			Selected: true,
		})
	}

	return CreateChecklistForm{
		NameInput: ti,
		Options:   options,
		InName:    true,
	}
}

func NewEditChecklistForm(allChecks []data.Check, chk *data.Checklist) CreateChecklistForm {
	ti := textinput.New()
	ti.SetValue(chk.Name)
	ti.CharLimit = 50
	ti.Width = 35
	ti.Focus()

	var options []CheckOption
	for _, c := range allChecks {
		options = append(options, CheckOption{
			Check:    c,
			Selected: chk.HasCheck(c.Name()),
		})
	}

	return CreateChecklistForm{
		ID:        chk.ID,
		NameInput: ti,
		Options:   options,
		InName:    true,
	}
}

func (m Model) renderChecklistsTab() string {
	var body strings.Builder
	if len(m.checklists) == 0 {
		body.WriteString(BaseStyle.Render("No checklists found. Press 'n' to create your first checklist!"))
	} else {
		body.WriteString(BaseStyle.Render(m.checklistsTable.View()))
	}
	if m.statusMsg != "" {
		body.WriteString("\n" + m.statusMsg)
	}
	body.WriteString("\n" + HelpStyle.Render("Space: Set Default • →/Enter: Edit • n: New • d: Delete • Tab: Switch Tab • q: Quit"))
	return body.String()
}

func (m Model) renderChecklistFormView() string {
	var body strings.Builder

	title := "⚙️  Create Checklist"
	if m.checklistForm.ID != "" {
		title = "✏️  Edit Checklist"
	}
	body.WriteString(TitleStyle.Render(title))
	body.WriteString("\n\n")

	nameLabel := "Checklist Name:"
	if m.checklistForm.InName {
		nameLabel = SuccessStyle.Render("➤ Checklist Name:")
	}
	body.WriteString(nameLabel + "\n" + m.checklistForm.NameInput.View() + "\n\n")

	checksLabel := "Select Checks (Space to toggle):"
	if !m.checklistForm.InName {
		checksLabel = SuccessStyle.Render("➤ Select Checks (Space to toggle):")
	}
	body.WriteString(checksLabel + "\n")

	for i, opt := range m.checklistForm.Options {
		cursor := "  "
		if !m.checklistForm.InName && m.checklistForm.Cursor == i {
			cursor = "➤ "
		}
		checked := "[ ]"
		if opt.Selected {
			checked = "[✓]"
		}
		body.WriteString(fmt.Sprintf("%s%s %-25s [%s]\n", cursor, checked, opt.Check.Name(), opt.Check.Checklist()))
	}

	body.WriteString("\n" + HelpStyle.Render("Tab: Switch Focus • Space: Toggle • Ctrl+S/Enter: Save • Esc: Cancel without saving"))
	return body.String()
}
