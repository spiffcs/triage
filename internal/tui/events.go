package tui

// TaskID identifies a task in the TUI progress display.
type TaskID int

const (
	TaskAuth    TaskID = iota // Authenticating with GitHub
	TaskFetch                 // Fetching data (notifications, review PRs, authored PRs in parallel)
	TaskEnrich                // Enriching all items with details
	TaskProcess               // Scoring, filtering, and processing results
)

// TaskStatus represents the current status of a task.
type TaskStatus int

const (
	StatusPending TaskStatus = iota
	StatusRunning
	StatusComplete
	StatusError
	StatusSkipped
)

// Event is the interface for all TUI events.
type Event interface {
	isEvent()
}

// TaskEvent represents an update to a task's status.
type TaskEvent struct {
	Task     TaskID
	Status   TaskStatus
	Message  string  // Optional message (e.g., "12/30" for progress)
	Count    int     // Count of items (e.g., notifications fetched)
	Progress float64 // Progress from 0.0 to 1.0
	Error    error   // Error if status is StatusError
}

func (TaskEvent) isEvent() {}

// DoneEvent signals that all work is complete.
type DoneEvent struct{}

func (DoneEvent) isEvent() {}
