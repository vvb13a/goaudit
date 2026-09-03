package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// historyMaxEntries bounds the session history kept in memory; oldest
// entries are dropped first.
const historyMaxEntries = 500

// Column metrics of the timeline rows: "15:04:05" plus a fixed-width badge
// column.
const (
	historyTimeW   = 8
	historyBadgeW  = 9
	historyIndentW = 2
)

type historyEntry struct {
	at   time.Time
	kind NotificationKind
	text string
}

// HistoryModel records every notification emitted during the current
// session and renders them as a timestamped timeline, newest first.
type HistoryModel struct {
	entries []historyEntry
	width   int
	height  int
}

func NewHistoryModel() HistoryModel {
	return HistoryModel{}
}

func (m HistoryModel) Loaded() bool            { return true }
func (m HistoryModel) Init() tea.Cmd           { return nil }
func (m HistoryModel) NavigationEnabled() bool { return true }

// Push appends a notification to the session history.
func (m HistoryModel) Push(n Notification) HistoryModel {
	m.entries = append(m.entries, historyEntry{at: time.Now(), kind: n.Kind, text: n.Text})
	if len(m.entries) > historyMaxEntries {
		m.entries = append([]historyEntry(nil), m.entries[len(m.entries)-historyMaxEntries:]...)
	}
	return m
}

func (m HistoryModel) Update(msg tea.Msg) (HistoryModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		if msg.String() == "q" {
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m HistoryModel) Help() string {
	return "Session notification history  •  q: Quit"
}

func (m HistoryModel) View() string {
	if m.width <= 0 {
		return ""
	}

	if len(m.entries) == 0 {
		return m.emptyView()
	}

	msgW := m.width - historyIndentW*2 - historyTimeW - 2 - historyBadgeW - 2
	if msgW < 10 {
		msgW = 10
	}

	var b strings.Builder
	for i := len(m.entries) - 1; i >= 0; i-- {
		e := m.entries[i]

		prefix := strings.Repeat(" ", historyIndentW) +
			historyTimeStyle.Render(e.at.Format("15:04:05")) +
			"  " +
			historyBadge(e.kind) +
			"  "

		lines := strings.Split(wrapText(e.text, msgW), "\n")
		for j, line := range lines {
			if j > 0 {
				b.WriteString(strings.Repeat(" ", lipgloss.Width(prefix)))
			} else {
				b.WriteString(prefix)
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// emptyView centers a hint vertically when nothing happened yet.
func (m HistoryModel) emptyView() string {
	if m.height <= 0 {
		return ""
	}
	var b strings.Builder
	pad := (m.height - 1) / 2
	for i := 0; i < pad; i++ {
		b.WriteString("\n")
	}
	b.WriteString(strings.Repeat(" ", historyIndentW))
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("No notifications yet this session."))
	return b.String()
}

var historyTimeStyle = lipgloss.NewStyle().Faint(true)

// historyBadge renders the notification kind as a colored, fixed-width
// label so the timeline columns stay aligned.
func historyBadge(kind NotificationKind) string {
	label := strings.ToUpper(string(kind))
	if pad := historyBadgeW - len(label); pad > 0 {
		label += strings.Repeat(" ", pad)
	}
	style := lipgloss.NewStyle().Bold(true).Foreground(notificationColor(kind))
	return style.Render(label)
}
