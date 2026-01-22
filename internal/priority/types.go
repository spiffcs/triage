package priority

import (
	"github.com/hal/priority/internal/github"
)

// Category represents the severity/importance level (not displayed in table)
type Category int

const (
	CategoryLow    Category = 1
	CategoryMedium Category = 2
	CategoryHigh   Category = 3
	CategoryUrgent Category = 4
)

// Display returns a human-readable category name
func (c Category) Display() string {
	switch c {
	case CategoryUrgent:
		return "Urgent"
	case CategoryHigh:
		return "High"
	case CategoryMedium:
		return "Medium"
	case CategoryLow:
		return "Low"
	default:
		return "Unknown"
	}
}

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
	Category     Category            `json:"category"`
	ActionNeeded string              `json:"actionNeeded"`
}
