package tui

import (
	"fmt"
	"strings"
)

// Task represents a single task in the TUI progress display.
type Task struct {
	ID       TaskID
	Name     string
	Status   TaskStatus
	Message  string
	Count    int
	Progress float64
	Error    error
}

// NewTask creates a new task with the given ID and name.
func NewTask(id TaskID, name string) Task {
	return Task{
		ID:     id,
		Name:   name,
		Status: StatusPending,
	}
}

// View renders the task as a string.
func (t Task) View(spinnerFrame string) string {
	icon := StatusIcon(t.Status, spinnerFrame)

	var name string
	if t.Status == StatusPending {
		name = taskDimStyle.Render(t.Name)
	} else {
		name = taskNameStyle.Render(t.Name)
	}

	line := fmt.Sprintf("  %s %s", icon, name)

	// Add progress bar if we have progress
	if t.Status == StatusRunning && t.Progress > 0 {
		bar := renderProgressBar(t.Progress, 20)
		percent := int(t.Progress * 100)
		line += fmt.Sprintf(" %s %d%%", bar, percent)
		if t.Message != "" {
			line += " " + messageStyle.Render(fmt.Sprintf("(%s)", t.Message))
		}
	} else if t.Message != "" {
		line += " " + messageStyle.Render(t.Message)
	}

	// Add count if available
	if t.Count > 0 && t.Message == "" {
		line += " " + messageStyle.Render(fmt.Sprintf("(%d)", t.Count))
	}

	// Add error if present
	if t.Error != nil {
		line += " " + errorStyle.Render(t.Error.Error())
	}

	return line
}

// renderProgressBar creates a simple progress bar.
func renderProgressBar(progress float64, width int) string {
	filled := min(int(progress*float64(width)), width)

	bar := strings.Repeat("━", filled)
	empty := strings.Repeat("░", width-filled)

	return progressBarStyle.Render(bar) + progressEmptyStyle.Render(empty)
}
