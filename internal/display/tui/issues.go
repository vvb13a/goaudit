package tui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/vvb13a/goaudit/data"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
)

func GetSortedIssues(a *data.Audit) []data.Issue {
	var allIssues []data.Issue
	for _, r := range a.Reports {
		allIssues = append(allIssues, r.Issues...)
	}

	sort.SliceStable(allIssues, func(i, j int) bool {
		return allIssues[i].Severity.Weight() > allIssues[j].Severity.Weight()
	})
	return allIssues
}

func BuildIssuesTable(issues []data.Issue) table.Model {
	columns := []table.Column{
		{Title: "State", Width: 6},
		{Title: "Severity", Width: 10},
		{Title: "Checklist", Width: 14},
		{Title: "Message", Width: 55},
	}

	var rows []table.Row
	for _, issue := range issues {
		rows = append(rows, table.Row{
			SeverityIcon(issue),
			string(issue.Severity),
			issue.Checklist,
			issue.Message,
		})
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(12),
	)
	t.SetStyles(TableStyle())
	return t
}

func (m Model) renderIssuesView() string {
	var body strings.Builder
	header := fmt.Sprintf("🔍 Audit Issues: %s (%s)", m.selectedAudit.PlanName, m.selectedAudit.ChecklistName)
	body.WriteString(TitleStyle.Render(header))
	body.WriteString("\n\n")

	body.WriteString(renderSeverityWidget(m.currentIssues))
	body.WriteString("\n\n")
	body.WriteString(BaseStyle.Render(m.issuesTable.View()))
	body.WriteString("\n" + HelpStyle.Render("↑/↓: Navigate • Enter: View Full Details • Esc/b: Back • q: Quit"))
	return body.String()
}

func (m Model) renderIssueDetailView() string {
	if m.selectedIssue == nil {
		return ""
	}

	issue := *m.selectedIssue
	var body strings.Builder

	header := fmt.Sprintf("🔍 Issue Detail — [%s] %s", strings.ToUpper(string(issue.Severity)), issue.CheckName)
	body.WriteString(TitleStyle.Render(header))
	body.WriteString("\n\n")

	var card strings.Builder
	card.WriteString(fmt.Sprintf("Status:     %s %s\n", SeverityIcon(issue), strings.ToUpper(string(issue.Severity))))
	card.WriteString(fmt.Sprintf("Checklist:  %s\n", issue.Checklist))
	card.WriteString(fmt.Sprintf("Check Name: %s\n\n", issue.CheckName))
	card.WriteString("Message:\n" + issue.Message + "\n")

	if len(issue.Details) > 0 {
		card.WriteString("\nDetails & Evidence:\n")
		if jsonBytes, err := json.MarshalIndent(issue.Details, "", "  "); err == nil {
			card.WriteString(string(jsonBytes) + "\n")
		}
	}

	body.WriteString(BaseStyle.Render(card.String()))
	body.WriteString("\n" + HelpStyle.Render("Esc/Backspace/b: Back to Issues • q: Quit"))
	return body.String()
}

func renderSeverityWidget(issues []data.Issue) string {
	counts := make(map[data.Severity]int)
	for _, issue := range issues {
		counts[issue.Severity]++
	}

	badges := []string{
		FatalBadgeStyle.Render(fmt.Sprintf("FATAL: %d", counts[data.SeverityFatal])),
		ErrorBadgeStyle.Render(fmt.Sprintf("ERROR: %d", counts[data.SeverityError])),
		WarningBadgeStyle.Render(fmt.Sprintf("WARN: %d", counts[data.SeverityWarning])),
		NoticeBadgeStyle.Render(fmt.Sprintf("NOTICE: %d", counts[data.SeverityNotice])),
		InfoBadgeStyle.Render(fmt.Sprintf("INFO: %d", counts[data.SeverityInfo])),
		SuccessBadgeStyle.Render(fmt.Sprintf("PASS: %d", counts[data.SeveritySuccess])),
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, badges...)
}
