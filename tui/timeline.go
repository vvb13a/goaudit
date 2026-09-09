package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/vvb13a/goaudit/domain"
)

// timelineLoadedMsg carries the run snapshots of the current audit. id
// guards against stale responses.
type timelineLoadedMsg struct {
	id        string
	snapshots []*domain.AuditSnapshot
	err       error
}

// maxTimelineSnapshots caps how many runs the timeline lists.
const maxTimelineSnapshots = 1000

// TimelineModel renders the run history of the current audit (tenant) as a
// timeline of audit snapshots, newest first: one row per run with its
// timestamp, score, URL counts, issue counts, critical issues (fatal+error)
// and duration. URL and issue cells read "total (+new)", omitting the new
// part when nothing is new.
type TimelineModel struct {
	deps    Deps
	auditID string
	loaded  bool

	snapshots []*domain.AuditSnapshot
	table     table.Model
	width     int
	height    int
}

func NewTimelineModel(deps Deps) TimelineModel {
	return TimelineModel{
		deps:  deps,
		table: table.New(),
	}
}

// NavigationEnabled lets the tab leave and re-enter freely (list only).
func (m TimelineModel) NavigationEnabled() bool {
	return true
}

// Track points the tab at the given audit and marks it stale so the next
// activation reloads its timeline.
func (m TimelineModel) Track(id string) TimelineModel {
	if m.auditID != id {
		m.loaded = false
		m.snapshots = nil
		m.table = table.New()
	}
	m.auditID = id
	return m
}

func (m TimelineModel) loadCmd(id string) tea.Cmd {
	return func() tea.Msg {
		snapshots, err := m.deps.AuditService.ListSnapshots(context.Background(), id, maxTimelineSnapshots)
		return timelineLoadedMsg{id: id, snapshots: snapshots, err: err}
	}
}

// release drops the loaded snapshots so the memory is returned before the
// tab sits in the background. The next activation reloads them.
func (m TimelineModel) release() TimelineModel {
	m.loaded = false
	m.snapshots = nil
	m.table = table.New()
	return m
}

func (m TimelineModel) Update(msg tea.Msg) (TimelineModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.rebuildTable()
		return m, nil

	case timelineLoadedMsg:
		if msg.id != m.auditID {
			return m, nil
		}
		m.loaded = true
		if msg.err != nil {
			return m, NotifyDanger(fmt.Sprintf("Failed to load timeline: %v", msg.err))
		}
		m.snapshots = msg.snapshots
		m.rebuildTable()
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "q" {
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// ---- Rendering ----

func (m TimelineModel) View() string {
	if m.auditID == "" {
		return centerLines(helpStyle.Render("No audit selected. Press Ctrl+O to open the audit switcher."), m.width)
	}
	if !m.loaded {
		return "Loading timeline..."
	}
	if len(m.snapshots) == 0 {
		return centerLines(helpStyle.Render("No runs for this audit yet."), m.width)
	}
	return m.table.View()
}

func (m TimelineModel) Help() string {
	if m.auditID == "" {
		return "Ctrl+O: Audits"
	}
	return "↑/↓: Run  •  URL & issue cells: total (+new)  •  Ctrl+O: Audits  •  q: Quit"
}

// ---- Table ----

func (m *TimelineModel) rebuildTable() {
	if m.auditID == "" {
		m.table = table.New()
		return
	}

	inner := m.width - 2
	if inner < 30 {
		inner = 30
	}

	// Column budgets, dropping columns as the terminal narrows: duration
	// first, then the issue counts, then the URL counts. Time, score and
	// the criticals always survive.
	timeW, scoreW, urlW, issuesW, critW, durW := 13, 7, 11, 13, 9, 10
	showIssues := inner >= 62
	showDuration := inner >= 78
	showFull := inner >= 92
	if !showIssues {
		urlW = 0
	}
	if !showFull {
		issuesW, durW = 0, 0
	} else if !showDuration {
		durW = 0
	}

	var columns []table.Column
	columns = append(columns,
		table.Column{Title: "Time", Width: timeW},
		table.Column{Title: "Score", Width: scoreW},
	)
	if urlW > 0 {
		columns = append(columns, table.Column{Title: "URLs", Width: urlW})
	}
	if issuesW > 0 {
		columns = append(columns, table.Column{Title: "Issues", Width: issuesW})
	}
	columns = append(columns, table.Column{Title: "Criticals", Width: critW})
	if durW > 0 {
		columns = append(columns, table.Column{Title: "Duration", Width: durW})
	}

	rows := make([]table.Row, 0, len(m.snapshots))
	// Snapshots are listed newest first, so the very last row is the audit's
	// first run. On that run everything is new by definition, so the (+new)
	// hint would be noise and is omitted.
	firstIdx := len(m.snapshots) - 1
	for i, s := range m.snapshots {
		row := table.Row{clipCell(timelineTime(s.CreatedAt), timeW-1), fmt.Sprintf("%.1f", s.Overview.Score)}
		if urlW > 0 {
			row = append(row, totalWithNew(s.Overview.TotalURLs, s.URLStates.New, i != firstIdx))
		}
		if issuesW > 0 {
			row = append(row, totalWithNew(s.Overview.TotalIssues, s.LifecycleCounts.New, i != firstIdx))
		}
		row = append(row, fmt.Sprintf("%d", s.SeverityCounts.Fatal+s.SeverityCounts.Error))
		if durW > 0 {
			duration := "–"
			if s.Timings.Total > 0 {
				duration = formatDuration(s.Timings.Total)
			}
			row = append(row, clipCell(duration, durW-1))
		}
		rows = append(rows, row)
	}

	height := m.height - 2
	if m.height <= 0 {
		height = 10
	}
	if height < 1 {
		height = 1
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(height),
	)
	t.SetStyles(tableStyle())
	t.SetWidth(inner)
	m.table = t
}

// timelineTime renders the snapshot timestamp in the terminal's local zone.
func timelineTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("Jan 02 15:04")
}

// totalWithNew renders a count as "total (+new)", omitting the new part
// when nothing is new or when there is no previous run to compare against
// (showNew false).
func totalWithNew(total, newCount int, showNew bool) string {
	if showNew && newCount > 0 {
		return fmt.Sprintf("%d (+%d)", total, newCount)
	}
	return fmt.Sprintf("%d", total)
}
