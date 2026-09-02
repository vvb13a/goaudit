package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Horizontal padding applied inside the header and footer bars.
const chromePadding = 2

// FooterModel renders the bottom bar of the application: a full-width strip
// with a context-specific key hint on the left and global navigation hints
// right-aligned.
type FooterModel struct {
	width int
	left  string
	right string
}

var footerBarStyle = lipgloss.NewStyle().
	Background(lipgloss.Color("#202020")).
	Foreground(lipgloss.Color("#B8B8B8"))

func NewFooterModel() FooterModel {
	return FooterModel{}
}

func (m FooterModel) Init() tea.Cmd {
	return nil
}

func (m FooterModel) Update(msg tea.Msg) (FooterModel, tea.Cmd) {
	if msg, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = msg.Width
	}
	return m, nil
}

func (m FooterModel) WithContent(left, right string) FooterModel {
	m.left = left
	m.right = right
	return m
}

func (m FooterModel) View() string {
	if m.width <= 0 {
		return ""
	}

	pad := strings.Repeat(" ", chromePadding)

	line := pad + m.left

	if m.right != "" {
		gap := m.width - 2*chromePadding - lipgloss.Width(m.left) - lipgloss.Width(m.right)
		if gap > 0 {
			line += strings.Repeat(" ", gap)
		} else {
			line += "  "
		}
		line += m.right
	}

	// Stretch the bar edge to edge, preserving internal padding on the right.
	remaining := m.width - lipgloss.Width(line)
	if remaining > 0 {
		line += strings.Repeat(" ", remaining)
	}

	return footerBarStyle.Render(line)
}
