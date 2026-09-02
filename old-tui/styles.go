package tui

import (
	"github.com/vvb13a/goaudit/data"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
)

var (
	BaseStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)

	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#5A56E0")).
			Padding(0, 1).
			MarginBottom(1)

	HelpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			MarginTop(1)

	SuccessStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#04B575")).
			Bold(true)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF5F87")).
			Bold(true)

	ActiveTabStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#5A56E0")).
			Padding(0, 2).
			MarginRight(1)

	InactiveTabStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("246")).
				Background(lipgloss.Color("236")).
				Padding(0, 2).
				MarginRight(1)

	badgeBase = lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1).
			MarginRight(1)

	FatalBadgeStyle   = badgeBase.Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#880808"))
	ErrorBadgeStyle   = badgeBase.Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#E03131"))
	WarningBadgeStyle = badgeBase.Foreground(lipgloss.Color("#1A1A1A")).Background(lipgloss.Color("#FAB005"))
	NoticeBadgeStyle  = badgeBase.Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#1971C2"))
	InfoBadgeStyle    = badgeBase.Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#495057"))
	SuccessBadgeStyle = badgeBase.Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#2F9E44"))
)

func TableStyle() table.Styles {
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(true)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(true)
	return s
}

func SeverityIcon(issue data.Issue) string {
	switch issue.Severity {
	case data.SeverityFatal, data.SeverityError:
		return "❌"
	case data.SeverityWarning:
		return "⚠️"
	case data.SeverityNotice:
		return "🔷"
	case data.SeverityInfo:
		return "ℹ️"
	case data.SeveritySuccess:
		return "✅"
	default:
		if issue.Passed {
			return "✅"
		}
		return "⚠️"
	}
}
