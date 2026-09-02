package tui

import (
	"fmt"
	"strings"

	"github.com/vvb13a/goaudit/data"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
)

func BuildPlansTable(plans []*data.Plan) table.Model {
	columns := []table.Column{
		{Title: "Plan Name", Width: 26},
		{Title: "URLs Count", Width: 12},
		{Title: "Sample URL", Width: 32},
		{Title: "Created", Width: 18},
	}

	var rows []table.Row
	for _, p := range plans {
		sampleURL := "-"
		if len(p.URLs) > 0 {
			sampleURL = p.URLs[0]
		}

		rows = append(rows, table.Row{
			p.Name,
			fmt.Sprintf("%d URLs", len(p.URLs)),
			sampleURL,
			p.CreatedAt.Format("2006-01-02 15:04"),
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

type CreatePlanForm struct {
	ID        string // Populated if editing existing plan
	NameInput textinput.Model
	URLsArea  textarea.Model
	InName    bool
}

func NewCreatePlanForm() CreatePlanForm {
	ti := textinput.New()
	ti.Placeholder = "e.g. Marketing & Checkout Pages"
	ti.CharLimit = 50
	ti.Width = 40
	ti.Focus()

	ta := textarea.New()
	ta.Placeholder = "https://example.com\nhttps://example.com/pricing\nhttps://example.com/checkout"
	ta.CharLimit = 4096
	ta.SetWidth(50)
	ta.SetHeight(6)

	return CreatePlanForm{
		NameInput: ti,
		URLsArea:  ta,
		InName:    true,
	}
}

func NewEditPlanForm(p *data.Plan) CreatePlanForm {
	form := NewCreatePlanForm()
	form.ID = p.ID
	form.NameInput.SetValue(p.Name)
	form.URLsArea.SetValue(strings.Join(p.URLs, "\n"))
	return form
}

func (m Model) renderPlansTab() string {
	var body strings.Builder
	if len(m.plans) == 0 {
		body.WriteString(BaseStyle.Render("No plans defined yet. Press 'n' to define your first target plan!"))
	} else {
		body.WriteString(BaseStyle.Render(m.plansTable.View()))
	}
	if m.statusMsg != "" {
		body.WriteString("\n" + m.statusMsg)
	}
	body.WriteString("\n" + HelpStyle.Render("r: Run Plan • →/Enter: Edit Plan • n: New • d: Delete • Tab: Switch Tab • q: Quit"))
	return body.String()
}

func (m Model) renderPlanFormView() string {
	var body strings.Builder

	title := "🎯 Define Target Plan"
	if m.planForm.ID != "" {
		title = "✏️  Edit Target Plan"
	}
	body.WriteString(TitleStyle.Render(title))
	body.WriteString("\n\n")

	nameLabel := "Plan Name:"
	if m.planForm.InName {
		nameLabel = SuccessStyle.Render("➤ Plan Name:")
	}
	body.WriteString(nameLabel + "\n" + m.planForm.NameInput.View() + "\n\n")

	urlsLabel := "Target URLs (one per line):"
	if !m.planForm.InName {
		urlsLabel = SuccessStyle.Render("➤ Target URLs (one per line):")
	}
	body.WriteString(urlsLabel + "\n" + m.planForm.URLsArea.View() + "\n\n")

	body.WriteString(HelpStyle.Render("Tab: Switch Focus • Ctrl+S: Save Plan • Esc: Cancel without saving"))
	return body.String()
}
