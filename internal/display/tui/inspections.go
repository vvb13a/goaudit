package tui

import (
	"fmt"
	"strings"

	"github.com/vvb13a/goaudit/data"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
)

func BuildInspectionsTable(inspections []*data.Inspection) table.Model {
	columns := []table.Column{
		{Title: "Plan / Target", Width: 26},
		{Title: "Checklist", Width: 18},
		{Title: "URLs", Width: 6},
		{Title: "Pass", Width: 6},
		{Title: "Fail", Width: 6},
		{Title: "Highest", Width: 10},
		{Title: "Duration", Width: 10},
		{Title: "Time", Width: 16},
	}

	var rows []table.Row
	for _, insp := range inspections {
		rows = append(rows, table.Row{
			insp.PlanName,
			insp.ChecklistName,
			fmt.Sprintf("%d", insp.TotalEndpoints),
			fmt.Sprintf("%d", insp.PassedCount),
			fmt.Sprintf("%d", insp.FailedCount),
			string(insp.HighestSeverity),
			insp.Duration.String(),
			insp.StartedAt.Format("01-02 15:04"),
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

func NewURLInput() textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "https://example.com"
	ti.CharLimit = 256
	ti.Width = 40
	return ti
}

func (m Model) renderInspectionsTab() string {
	var body strings.Builder
	if len(m.inspections) == 0 {
		body.WriteString(BaseStyle.Render("No inspections found. Press 'n' for a quick URL inspection, or switch to 'Plans' with Tab!"))
	} else {
		body.WriteString(BaseStyle.Render(m.inspectionsTable.View()))
	}
	if m.statusMsg != "" {
		body.WriteString("\n" + m.statusMsg)
	}
	body.WriteString("\n" + HelpStyle.Render("n: Quick URL • r: Rerun • d: Delete • Tab: Switch Tab • Enter: View Issues • q: Quit"))
	return body.String()
}

func (m Model) renderPromptView() string {
	var body strings.Builder
	body.WriteString(TitleStyle.Render("🚀 Quick Inspection"))
	body.WriteString("\n\n")
	body.WriteString("Enter Target URL:\n\n")
	body.WriteString(m.textInput.View())
	body.WriteString("\n\n" + HelpStyle.Render("Enter: Start • Esc: Cancel"))
	return body.String()
}
