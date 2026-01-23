package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Status icons
	iconPending  = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("○")
	iconComplete = lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Render("✓")
	iconError    = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("✗")
	iconSkipped  = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("○")

	// Styles
	taskNameStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	taskDimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	messageStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	spinnerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("86"))

	progressBarStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("86"))

	progressEmptyStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240"))

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			MarginTop(1)

	userStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("220")).
			Bold(true)
)

// StatusIcon returns the appropriate icon for a task status.
func StatusIcon(status TaskStatus, spinnerFrame string) string {
	switch status {
	case StatusPending:
		return iconPending
	case StatusRunning:
		return spinnerStyle.Render(spinnerFrame)
	case StatusComplete:
		return iconComplete
	case StatusError:
		return iconError
	case StatusSkipped:
		return iconSkipped
	default:
		return iconPending
	}
}
