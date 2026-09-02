package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// NotificationKind classifies a notification so it can be styled
// consistently across the whole application.
type NotificationKind string

const (
	NotificationInfo    NotificationKind = "info"
	NotificationSuccess NotificationKind = "success"
	NotificationWarning NotificationKind = "warning"
	NotificationDanger  NotificationKind = "danger"
)

// Notification is a user-facing status message.
type Notification struct {
	Kind NotificationKind
	Text string
}

// notifyMsg carries a Notification from wherever it was emitted up to the
// root model, which forwards it to the NotificationModel.
type notifyMsg struct {
	notification Notification
}

// Notify returns a command that emits a notification of the given kind.
func Notify(kind NotificationKind, text string) tea.Cmd {
	return func() tea.Msg {
		return notifyMsg{notification: Notification{Kind: kind, Text: text}}
	}
}

func NotifyInfo(text string) tea.Cmd    { return Notify(NotificationInfo, text) }
func NotifySuccess(text string) tea.Cmd { return Notify(NotificationSuccess, text) }
func NotifyWarning(text string) tea.Cmd { return Notify(NotificationWarning, text) }
func NotifyDanger(text string) tea.Cmd  { return Notify(NotificationDanger, text) }

// NotificationModel shows the most recently emitted notification, rendered
// in the top-right corner of the application.
type NotificationModel struct {
	current *Notification
}

func NewNotificationModel() NotificationModel {
	return NotificationModel{}
}

func (m NotificationModel) Push(n Notification) NotificationModel {
	m.current = &n
	return m
}

func (m NotificationModel) Clear() NotificationModel {
	m.current = nil
	return m
}

func (m NotificationModel) IsActive() bool {
	return m.current != nil
}

func (m NotificationModel) View(maxWidth int) string {
	if m.current == nil {
		return ""
	}

	kind := m.current.Kind
	label := strings.ToUpper(string(kind))
	color := notificationColor(kind)

	budget := maxWidth - len(label) - 1
	if budget < 10 {
		budget = 10
	}
	text := truncateText(m.current.Text, budget)

	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(color)
	textStyle := lipgloss.NewStyle().Foreground(color)
	return labelStyle.Render(label) + " " + textStyle.Render(text)
}

func notificationColor(kind NotificationKind) lipgloss.Color {
	switch kind {
	case NotificationSuccess:
		return lipgloss.Color("10")
	case NotificationWarning:
		return lipgloss.Color("3")
	case NotificationDanger:
		return lipgloss.Color("9")
	default:
		return lipgloss.Color("39")
	}
}

func truncateText(text string, max int) string {
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	if max <= 1 {
		return "…"
	}
	return string(runes[:max-1]) + "…"
}
