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
	case PriorityNotable:
		return "Notable"
	case PriorityFYI:
		return "FYI"
	default:
		return string(p)
	}
}

// PrioritizedItem wraps an item with priority information
type PrioritizedItem struct {
	Item         model.Item    `json:"item"`
	Score        int           `json:"score"`
	Priority     PriorityLevel `json:"priority"`
	ActionNeeded string        `json:"actionNeeded"`
}
