package tui

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spiffcs/triage/config"
	"github.com/spiffcs/triage/internal/resolved"
	"github.com/spiffcs/triage/internal/triage"
	"golang.org/x/term"
)

// Run starts the TUI and blocks until it completes.
func Run(events <-chan Event) error {
	model := NewModel(events)
	// Don't use alt screen - render inline
	p := tea.NewProgram(model)
	_, err := p.Run()
	return err
}

// ShouldUseTUI returns true if the TUI should be used based on environment.
func ShouldUseTUI() bool {
	// Check if stdout is a TTY
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return false
	}

	// Check for CI environment variables
	ciVars := []string{
		"CI",
		"GITHUB_ACTIONS",
		"JENKINS_URL",
		"TRAVIS",
		"CIRCLECI",
		"GITLAB_CI",
		"BUILDKITE",
	}

	for _, v := range ciVars {
		if os.Getenv(v) != "" {
			return false
		}
	}

	return true
}

// SendEvent sends an event to the channel in a non-blocking manner.
func SendEvent(ch chan<- Event, e Event) {
	if ch == nil {
		return
	}
	select {
	case ch <- e:
	default:
		// Non-blocking send - drop event if channel is full
	}
}

// SendTaskEvent is a convenience function for sending task events.
func SendTaskEvent(ch chan<- Event, task TaskID, status TaskStatus, opts ...TaskEventOption) {
	e := TaskEvent{
		Task:   task,
		Status: status,
	}
	for _, opt := range opts {
		opt(&e)
	}
	SendEvent(ch, e)
}

// TaskEventOption is a functional option for TaskEvent.
type TaskEventOption func(*TaskEvent)

// WithMessage sets the message on a TaskEvent.
func WithMessage(msg string) TaskEventOption {
	return func(e *TaskEvent) {
		e.Message = msg
	}
}

// WithCount sets the count on a TaskEvent.
func WithCount(count int) TaskEventOption {
	return func(e *TaskEvent) {
		e.Count = count
	}
}

// WithProgress sets the progress on a TaskEvent.
func WithProgress(progress float64) TaskEventOption {
	return func(e *TaskEvent) {
		e.Progress = progress
	}
}

// WithError sets the error on a TaskEvent.
func WithError(err error) TaskEventOption {
	return func(e *TaskEvent) {
		e.Error = err
	}
}

// RunListUI starts the interactive list UI for triaging items
func RunListUI(items []triage.PrioritizedItem, store *resolved.Store, weights config.ScoreWeights, currentUser string, opts ...ListOption) error {
	model := NewListModel(items, store, weights, currentUser, opts...)
	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
