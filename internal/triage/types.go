package triage

import (
	"github.com/hal/triage/internal/github"
)

// PriorityLevel represents the action priority (displayed in table)
type PriorityLevel string

const (
	PriorityUrgent    PriorityLevel = "urgent"
	PriorityImportant PriorityLevel = "important"
	PriorityQuickWin  PriorityLevel = "quick-win"
	PriorityFYI       PriorityLevel = "fyi"
)

// Display returns a human-readable priority level
func (p PriorityLevel) Display() string {
	switch p {
	case PriorityUrgent:
		return "Urgent"
	case PriorityImportant:
		return "Important"
	case PriorityQuickWin:
		return "Quick Win"
	case PriorityFYI:
		return "FYI"
	default:
		return string(p)
	}
}

// PrioritizedItem wraps a notification with priority information
type PrioritizedItem struct {
	Notification github.Notification `json:"notification"`
	Score        int                 `json:"score"`
	Priority     PriorityLevel       `json:"priority"`
	ActionNeeded string              `json:"actionNeeded"`
}
