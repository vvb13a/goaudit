package tui

import (
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))

	labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	helpStyle  = lipgloss.NewStyle().Faint(true)

	// tenantLabel prefixes the current tenant in the header bar.
	tenantLabel = helpStyle.Render("Audit: ")

	// tenantNameStyle renders the tenant (audit) name in the header bar.
	tenantNameStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7aa2f7"))

	pillActiveStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#5A56E0")).
			Padding(0, 2)

	pillInactiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#DDDDDD")).
				Background(lipgloss.Color("#3F3F3F")).
				Padding(0, 2)

	// chromeSpaceStyle paints background-colored spaces, used to stretch the
	// header/footer bars edge to edge.
	chromeSpaceStyle = lipgloss.NewStyle().Background(lipgloss.Color("#262626"))
)

func tableStyle() table.Styles {
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
