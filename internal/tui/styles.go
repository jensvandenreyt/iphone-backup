package tui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	meStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	senderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	cursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	pageStyle   = lipgloss.NewStyle().Margin(1, 2)
	logStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("241")).Padding(0, 1)
)
