package triage

import (
	"github.com/spiffcs/triage/internal/model"
)

// PriorityLevel represents the action priority (displayed in table)
type PriorityLevel string

const (
	PriorityUrgent    PriorityLevel = "urgent"
	PriorityQuickWin  PriorityLevel = "quick-win"
	PriorityImportant PriorityLevel = "important"
	PriorityOrphaned  PriorityLevel = "orphaned"
	PriorityNotable   PriorityLevel = "notable"
	PriorityFYI       PriorityLevel = "fyi"
)

// Display returns a human-readable priority level
func (p PriorityLevel) Display() string {
	switch p {
	case PriorityUrgent:
		return "Urgent"
	case PriorityQuickWin:
		return "Quick Win"
	case PriorityImportant:
		return "Important"
	case PriorityOrphaned:
		return "Orphaned"
	case PriorityNotable:
		return "Notable"
	case PriorityFYI:
		return "FYI"
	default:
		return string(p)
	}
}

// PrioritizedItem wraps a notification with priority information
type PrioritizedItem struct {
	Notification model.Item    `json:"notification"`
	Score        int           `json:"score"`
	Priority     PriorityLevel `json:"priority"`
	ActionNeeded string        `json:"actionNeeded"`
}
