package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// Model is the Bubble Tea model for the TUI progress display.
type Model struct {
	tasks          []Task
	spinner        spinner.Model
	progress       progress.Model
	events         <-chan Event
	done           bool
	username       string
	windowWidth    int
	windowHeight   int
	rateLimited    bool
	rateLimitReset time.Time
}

// doneMsg signals that all events have been processed.
type doneMsg struct{}

// ModelOption is a functional option for configuring a Model.
type ModelOption func(*Model)

// WithTasks sets the tasks to display in the TUI.
func WithTasks(tasks []Task) ModelOption {
	return func(m *Model) {
		m.tasks = tasks
	}
}

// DefaultTasks returns the default task list for the main triage command.
func DefaultTasks() []Task {
	return []Task{
		NewTask(TaskAuth, "Authenticating"),
		NewTask(TaskFetch, "Fetching data"),
		NewTask(TaskEnrich, "Enriching items"),
		NewTask(TaskProcess, "Processing results"),
	}
}

// OrphanedTasks returns the task list for the orphaned command (no enrichment step).
func OrphanedTasks() []Task {
	return []Task{
		NewTask(TaskAuth, "Authenticating"),
		NewTask(TaskFetch, "Fetching data"),
		NewTask(TaskProcess, "Processing results"),
	}
}

// NewModel creates a new TUI model.
func NewModel(events <-chan Event, opts ...ModelOption) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	p := progress.New(
		progress.WithScaledGradient("#60a5fa", "#1e3a8a"),
		progress.WithWidth(25),
		progress.WithoutPercentage(),
	)

	m := Model{
		tasks:    DefaultTasks(),
		spinner:  s,
		progress: p,
		events:   events,
	}

	for _, opt := range opts {
		opt(&m)
	}

	return m
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

	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		return m, cmd

	case TaskEvent:
		var cmd tea.Cmd
		m, cmd = m.updateTask(msg)
		return m, tea.Batch(cmd, waitForEvent(m.events))

	case DoneEvent:
		m.done = true
		return m, tea.Quit

	case RateLimitEvent:
		m.rateLimited = msg.Limited
		m.rateLimitReset = msg.ResetAt
		return m, waitForEvent(m.events)

	case doneMsg:
		m.done = true
		return m, tea.Quit
	}

	return m, nil
}

// updateTask updates a task based on a TaskEvent.
func (m Model) updateTask(e TaskEvent) (Model, tea.Cmd) {
	var cmd tea.Cmd
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
				cmd = m.progress.SetPercent(e.Progress)
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
	return m, cmd
}

// View renders the model.
func (m Model) View() string {
	var s string

	// Render all tasks
	for _, task := range m.tasks {
		// Special handling for auth task to show username
		if task.ID == TaskAuth {
			switch task.Status {
			case StatusComplete:
				if m.username != "" {
					s += fmt.Sprintf("  %s Authenticated as %s\n", iconComplete, userStyle.Render(m.username))
					continue
				}
				fallthrough
			case StatusRunning:
				s += fmt.Sprintf("  %s Authenticating...\n", spinnerStyle.Render(m.spinner.View()))
			case StatusError:
				s += fmt.Sprintf("  %s Authenticating %s\n", iconError, errorStyle.Render(task.Error.Error()))
			default:
				s += task.View(m.spinner.View(), m.progress) + "\n"
			}
			continue
		}
		s += task.View(m.spinner.View(), m.progress) + "\n"
	}

	// Show rate limit warning if applicable
	if m.rateLimited {
		duration := time.Until(m.rateLimitReset).Round(time.Second)
		if duration > 0 {
			s += warnStyle.Render(fmt.Sprintf("\n  Rate limited - using cached data (resets in %s)\n", duration))
		}
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
