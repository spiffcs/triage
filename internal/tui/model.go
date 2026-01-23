package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// Model is the Bubble Tea model for the TUI progress display.
type Model struct {
	tasks        []Task
	spinner      spinner.Model
	progress     progress.Model
	events       <-chan Event
	done         bool
	username     string
	windowWidth  int
	windowHeight int
}

// doneMsg signals that all events have been processed.
type doneMsg struct{}

// NewModel creates a new TUI model.
func NewModel(events <-chan Event) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	p := progress.New(
		progress.WithScaledGradient("#60a5fa", "#1e3a8a"),
		progress.WithWidth(25),
		progress.WithoutPercentage(),
	)

	// Task order matches actual execution order in cmd/list.go
	// Using simplified parallel flow for better performance
	tasks := []Task{
		NewTask(TaskAuth, "Authenticating"),
		NewTask(TaskFetch, "Fetching data"),
		NewTask(TaskEnrich, "Enriching items"),
		NewTask(TaskProcess, "Processing results"),
	}

	return Model{
		tasks:    tasks,
		spinner:  s,
		progress: p,
		events:   events,
	}
}

// Init initializes the model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		waitForEvent(m.events),
	)
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.windowWidth = msg.Width
		m.windowHeight = msg.Height
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case TaskEvent:
		m = m.updateTask(msg)
		return m, waitForEvent(m.events)

	case DoneEvent:
		m.done = true
		return m, tea.Quit

	case doneMsg:
		m.done = true
		return m, tea.Quit
	}

	return m, nil
}

// updateTask updates a task based on a TaskEvent.
func (m Model) updateTask(e TaskEvent) Model {
	for i := range m.tasks {
		if m.tasks[i].ID == e.Task {
			m.tasks[i].Status = e.Status
			if e.Message != "" {
				m.tasks[i].Message = e.Message
			}
			if e.Count > 0 {
				m.tasks[i].Count = e.Count
			}
			if e.Progress > 0 {
				m.tasks[i].Progress = e.Progress
			}
			if e.Error != nil {
				m.tasks[i].Error = e.Error
			}
			// Capture username from auth complete event
			if e.Task == TaskAuth && e.Status == StatusComplete && e.Message != "" {
				m.username = e.Message
			}
			break
		}
	}
	return m
}

// View renders the model.
func (m Model) View() string {
	var s string

	// Render all tasks
	for _, task := range m.tasks {
		// Special handling for auth task to show username
		if task.ID == TaskAuth {
			if task.Status == StatusComplete && m.username != "" {
				s += fmt.Sprintf("  %s Authenticated as %s\n", iconComplete, userStyle.Render(m.username))
			} else if task.Status == StatusRunning {
				s += fmt.Sprintf("  %s Authenticating...\n", spinnerStyle.Render(m.spinner.View()))
			} else if task.Status == StatusError {
				s += fmt.Sprintf("  %s Authenticating %s\n", iconError, errorStyle.Render(task.Error.Error()))
			} else {
				s += task.View(m.spinner.View(), m.progress) + "\n"
			}
			continue
		}
		s += task.View(m.spinner.View(), m.progress) + "\n"
	}

	// Only show cancel hint while running
	if !m.done {
		s += footerStyle.Render("\n  Press Ctrl+C to cancel")
	}
	s += "\n"

	return s
}

// waitForEvent creates a command that waits for the next event.
func waitForEvent(events <-chan Event) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-events
		if !ok {
			return doneMsg{}
		}
		return event
	}
}
