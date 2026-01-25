package tui

import (
	"errors"
	"testing"
)

func TestTaskID(t *testing.T) {
	// Verify task IDs are distinct
	ids := []TaskID{TaskAuth, TaskFetch, TaskEnrich, TaskProcess}
	seen := make(map[TaskID]bool)

	for _, id := range ids {
		if seen[id] {
			t.Errorf("duplicate task ID: %d", id)
		}
		seen[id] = true
	}
}

func TestTaskStatus(t *testing.T) {
	// Verify statuses are distinct
	statuses := []TaskStatus{StatusPending, StatusRunning, StatusComplete, StatusError, StatusSkipped}
	seen := make(map[TaskStatus]bool)

	for _, status := range statuses {
		if seen[status] {
			t.Errorf("duplicate status: %d", status)
		}
		seen[status] = true
	}
}

func TestNewTask(t *testing.T) {
	task := NewTask(TaskFetch, "Fetching notifications")

	if task.ID != TaskFetch {
		t.Errorf("expected ID %d, got %d", TaskFetch, task.ID)
	}
	if task.Name != "Fetching notifications" {
		t.Errorf("expected name 'Fetching notifications', got %q", task.Name)
	}
	if task.Status != StatusPending {
		t.Errorf("expected status %d, got %d", StatusPending, task.Status)
	}
}

func TestTaskEvent(t *testing.T) {
	event := TaskEvent{
		Task:     TaskEnrich,
		Status:   StatusRunning,
		Message:  "10/20",
		Count:    10,
		Progress: 0.5,
	}

	// Verify it implements Event interface
	var _ Event = event

	if event.Task != TaskEnrich {
		t.Errorf("expected task %d, got %d", TaskEnrich, event.Task)
	}
	if event.Progress != 0.5 {
		t.Errorf("expected progress 0.5, got %f", event.Progress)
	}
}

func TestDoneEvent(t *testing.T) {
	event := DoneEvent{}

	// Verify it implements Event interface
	var _ Event = event
}

func TestSendEvent(t *testing.T) {
	ch := make(chan Event, 1)

	event := TaskEvent{Task: TaskAuth, Status: StatusComplete}
	SendEvent(ch, event)

	select {
	case received := <-ch:
		if te, ok := received.(TaskEvent); ok {
			if te.Task != TaskAuth {
				t.Errorf("expected task %d, got %d", TaskAuth, te.Task)
			}
		} else {
			t.Error("expected TaskEvent type")
		}
	default:
		t.Error("expected event in channel")
	}
}

func TestSendEventNilChannel(t *testing.T) {
	// Should not panic with nil channel
	SendEvent(nil, TaskEvent{})
}

func TestSendTaskEvent(t *testing.T) {
	ch := make(chan Event, 1)

	SendTaskEvent(ch, TaskProcess, StatusRunning,
		WithMessage("processing"),
		WithCount(42),
		WithProgress(0.75),
	)

	select {
	case received := <-ch:
		te, ok := received.(TaskEvent)
		if !ok {
			t.Fatal("expected TaskEvent type")
		}
		if te.Task != TaskProcess {
			t.Errorf("expected task %d, got %d", TaskProcess, te.Task)
		}
		if te.Message != "processing" {
			t.Errorf("expected message 'processing', got %q", te.Message)
		}
		if te.Count != 42 {
			t.Errorf("expected count 42, got %d", te.Count)
		}
		if te.Progress != 0.75 {
			t.Errorf("expected progress 0.75, got %f", te.Progress)
		}
	default:
		t.Error("expected event in channel")
	}
}

func TestWithError(t *testing.T) {
	ch := make(chan Event, 1)
	testErr := errors.New("test error")

	SendTaskEvent(ch, TaskFetch, StatusError, WithError(testErr))

	select {
	case received := <-ch:
		te, ok := received.(TaskEvent)
		if !ok {
			t.Fatal("expected TaskEvent type")
		}
		if te.Error != testErr {
			t.Errorf("expected error %v, got %v", testErr, te.Error)
		}
	default:
		t.Error("expected event in channel")
	}
}

func TestShouldUseTUI(t *testing.T) {
	// Just verify it returns a boolean and doesn't panic
	// The actual result depends on the environment (TTY, CI vars)
	result := ShouldUseTUI()
	_ = result // Use the result to avoid compiler warning
}

func TestStatusIcon(t *testing.T) {
	// Test that StatusIcon returns non-empty strings for all statuses
	statuses := []TaskStatus{StatusPending, StatusRunning, StatusComplete, StatusError, StatusSkipped}

	for _, status := range statuses {
		icon := StatusIcon(status, ">")
		if icon == "" {
			t.Errorf("StatusIcon returned empty string for status %d", status)
		}
	}
}
