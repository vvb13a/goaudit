package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// Tab pairs a top-level view with the label shown in the navigation bar.
type Tab struct {
	ID    ViewID
	Label string
}

// NavModel renders the tab bar at the top of the application and tracks which
// top-level view is active. Navigation (tab / shift+tab / number keys) is
// handled by the root model.
type NavModel struct {
	tabs   []Tab
	active int
}

func NewNavModel(tabs []Tab) NavModel {
	return NavModel{tabs: tabs}
}

func (m NavModel) Active() ViewID {
	if len(m.tabs) == 0 {
		return -1
	}
	return m.tabs[m.active].ID
}

func (m NavModel) Select(id ViewID) NavModel {
	for i, t := range m.tabs {
		if t.ID == id {
			m.active = i
			break
		}
	}
	return m
}

func (m NavModel) SelectIndex(idx int) NavModel {
	if idx >= 0 && idx < len(m.tabs) {
		m.active = idx
	}
	return m
}

func (m NavModel) Next() NavModel {
	if len(m.tabs) > 0 {
		m.active = (m.active + 1) % len(m.tabs)
	}
	return m
}

func (m NavModel) Prev() NavModel {
	if len(m.tabs) > 0 {
		m.active = (m.active - 1 + len(m.tabs)) % len(m.tabs)
	}
	return m
}

func (m NavModel) View() string {
	var parts []string
	for i, t := range m.tabs {
		label := fmt.Sprintf("%d: %s", i+1, t.Label)
		if i == m.active {
			parts = append(parts, activeTabStyle.Render(label))
		} else {
			parts = append(parts, inactiveTabStyle.Render(label))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}
