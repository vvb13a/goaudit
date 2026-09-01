package tui

import (
	"fmt"
	"strings"

	"github.com/vvb13a/goaudit/data"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
)

func BuildAuditsTable(audits []*data.Audit) table.Model {
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
	for _, a := range audits {
		rows = append(rows, table.Row{
			a.PlanName,
			a.ChecklistName,
			fmt.Sprintf("%d", a.TotalEndpoints),
			fmt.Sprintf("%d", a.PassedCount),
			fmt.Sprintf("%d", a.FailedCount),
			string(a.HighestSeverity),
			a.Duration.String(),
			a.StartedAt.Format("01-02 15:04"),
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

func (m Model) renderAuditsTab() string {
	var body strings.Builder
	if len(m.audits) == 0 {
		body.WriteString(BaseStyle.Render("No audits found. Press 'n' for a quick URL audit, or switch to 'Plans' with Tab!"))
	} else {
		body.WriteString(BaseStyle.Render(m.auditsTable.View()))
	}
	if m.statusMsg != "" {
		body.WriteString("\n" + m.statusMsg)
	}
	body.WriteString("\n" + HelpStyle.Render("n: Quick URL • r: Rerun • d: Delete • Tab: Switch Tab • Enter: View Issues • q: Quit"))
	return body.String()
}

func (m Model) renderPromptView() string {
	var body strings.Builder
	body.WriteString(TitleStyle.Render("🚀 Quick Audit"))
	body.WriteString("\n\n")
	body.WriteString("Enter Target URL:\n\n")
	body.WriteString(m.textInput.View())
	body.WriteString("\n\n" + HelpStyle.Render("Enter: Start • Esc: Cancel"))
	return body.String()
}
