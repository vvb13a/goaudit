package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	modalBorderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#7f9cf5"))
)

// overlay draws a centered bordered panel with body on top of base, within a
// region of the given size. The surrounding base content stays visible.
func overlay(base, body string, width, height int) string {
	baseLines := strings.Split(base, "\n")
	for len(baseLines) < height {
		baseLines = append(baseLines, "")
	}
	if width <= 2 || len(baseLines) > height {
		if width <= 2 {
			return base
		}
		baseLines = baseLines[:height]
	}

	bodyLines := strings.Split(strings.TrimRight(body, "\n"), "\n")

	innerW := 0
	for _, l := range bodyLines {
		if w := lipgloss.Width(l); w > innerW {
			innerW = w
		}
	}
	maxInner := width - 6
	if innerW > maxInner {
		innerW = maxInner
	}
	if innerW < 1 {
		innerW = 1
	}

	boxW := innerW + 4
	boxH := len(bodyLines) + 2

	if boxH > height {
		boxH = height
	}
	top := (height - boxH) / 2
	if top < 0 {
		top = 0
	}
	leftPad := (width - boxW) / 2
	if leftPad < 0 {
		leftPad = 0
	}

	contentRow := func(text string) string {
		extra := innerW - lipgloss.Width(text)
		if extra < 0 {
			extra = 0
		}
		return "│ " + text + strings.Repeat(" ", extra) + " │"
	}

	for i := 0; i < boxH && top+i < height; i++ {
		var line string
		switch i {
		case 0:
			line = "┌" + strings.Repeat("─", boxW-2) + "┐"
		case boxH - 1:
			line = "└" + strings.Repeat("─", boxW-2) + "┘"
		default:
			line = contentRow(bodyLines[i-1])
		}

		rightPad := width - leftPad - lipgloss.Width(line)
		if rightPad < 0 {
			rightPad = 0
		}
		baseLines[top+i] = strings.Repeat(" ", leftPad) + modalBorderStyle.Render(line) + strings.Repeat(" ", rightPad)
	}

	return strings.Join(baseLines, "\n")
}
