package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/vvb13a/goaudit/domain"
)

// centerLines horizontally centers each line within the given width.
func centerLines(content string, width int) string {
	var b strings.Builder
	for _, line := range strings.Split(content, "\n") {
		pad := width - lipgloss.Width(line)
		if pad < 0 {
			pad = 0
		}
		b.WriteString(strings.Repeat(" ", pad/2))
		b.WriteString(line)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// paneBox frames pane content with a rounded border; the focused pane glows
// with an accent-colored border while the others stay dim.
func paneBox(content string, width, height int, focused bool) string {
	innerW := width - 2
	if innerW < 4 {
		innerW = 4
	}

	color := lipgloss.Color("#444b6a")
	style := lipgloss.NewStyle().Foreground(color)
	if focused {
		style = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7aa2f7"))
	}

	innerRows := height - 2
	if innerRows < 1 {
		innerRows = 1
	}
	lines := strings.Split(content, "\n")
	for len(lines) < innerRows {
		lines = append(lines, "")
	}

	side := style.Render("│")
	var b strings.Builder
	b.WriteString(style.Render("╭" + strings.Repeat("─", innerW) + "╮"))
	b.WriteString("\n")
	for i := 0; i < innerRows; i++ {
		b.WriteString(side)
		b.WriteString(clipToWidth(lines[i], innerW))
		b.WriteString(side)
		b.WriteString("\n")
	}
	b.WriteString(style.Render("╰" + strings.Repeat("─", innerW) + "┘"))
	return b.String()
}

// clipToWidth truncates or pads a string to the given display width.
func clipToWidth(s string, w int) string {
	pad := w - lipgloss.Width(s)
	if pad >= 0 {
		return s + strings.Repeat(" ", pad)
	}
	runes := []rune(s)
	if w <= 1 {
		return "…"
	}
	return string(runes[:w-1]) + "…"
}

// clipCell truncates a string to max display characters, adding an ellipsis
// when content had to be cut.
func clipCell(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 1 {
		return "…"
	}
	return string(runes[:max-1]) + "…"
}

// wrapText soft-wraps long lines at the given width, keeping existing
// newlines.
func wrapText(s string, width int) string {
	if width < 20 {
		width = 20
	}
	var out strings.Builder
	for _, line := range strings.Split(s, "\n") {
		runes := []rune(line)
		for len(runes) > width {
			out.WriteString(string(runes[:width]))
			out.WriteString("\n")
			runes = runes[width:]
		}
		out.WriteString(string(runes))
		out.WriteString("\n")
	}
	return strings.TrimRight(out.String(), "\n")
}

func issueSeverityColor(severity domain.Severity) lipgloss.Color {
	switch severity {
	case domain.SeverityFatal, domain.SeverityError:
		return lipgloss.Color("9")
	case domain.SeverityWarning:
		return lipgloss.Color("3")
	case domain.SeverityNotice:
		return lipgloss.Color("39")
	case domain.SeverityInfo:
		return lipgloss.Color("45")
	default:
		return lipgloss.Color("10")
	}
}

// timeAgo renders a timestamp relative to now as "1 minute ago",
// "10 days ago", etc. When short is set it degrades to compact forms like
// "1m ago" that fit a narrow column.
func timeAgo(t time.Time, short bool) string {
	d := time.Since(t)
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		if short {
			return "now"
		}
		return "just now"
	case d < time.Hour:
		return ago(d, time.Minute, "minute", "m", short)
	case d < 24*time.Hour:
		return ago(d, time.Hour, "hour", "h", short)
	case d < 30*24*time.Hour:
		return ago(d, 24*time.Hour, "day", "d", short)
	case d < 365*24*time.Hour:
		return ago(d, 30*24*time.Hour, "month", "mo", short)
	default:
		return ago(d, 365*24*time.Hour, "year", "y", short)
	}
}

func ago(d, unit time.Duration, fullUnit, shortUnit string, short bool) string {
	n := int(d / unit)
	if n < 1 {
		n = 1
	}
	if short {
		return fmt.Sprintf("%d%s ago", n, shortUnit)
	}
	label := fullUnit
	if n != 1 {
		label += "s"
	}
	return fmt.Sprintf("%d %s ago", n, label)
}
