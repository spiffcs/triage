package priority

import (
	"github.com/hal/github-prio/internal/github"
)

// Category represents a priority category
type Category string

const (
	CategoryUrgent      Category = "urgent"
	CategoryImportant   Category = "important"
	CategoryLowHanging  Category = "low-hanging"
	CategoryFYI         Category = "fyi"
)

// CategoryDisplay returns a human-readable category name
func (c Category) Display() string {
	switch c {
	case CategoryUrgent:
		return "Urgent"
	case CategoryImportant:
		return "Important"
	case CategoryLowHanging:
		return "Quick Win"
	case CategoryFYI:
		return "FYI"
	default:
		return string(c)
	}
}

// PriorityLevel represents the computed priority level
type PriorityLevel int

const (
	PriorityLow    PriorityLevel = 1
	PriorityMedium PriorityLevel = 2
	PriorityHigh   PriorityLevel = 3
	PriorityUrgent PriorityLevel = 4
)

// Display returns a human-readable priority level
func (p PriorityLevel) Display() string {
	switch p {
	case PriorityUrgent:
		return "URGENT"
	case PriorityHigh:
		return "HIGH"
	case PriorityMedium:
		return "MEDIUM"
	case PriorityLow:
		return "LOW"
	default:
		return "UNKNOWN"
	}
}

// PrioritizedItem wraps a notification with priority information
type PrioritizedItem struct {
	Notification github.Notification `json:"notification"`
	Score        int                 `json:"score"`
	Priority     PriorityLevel       `json:"priority"`
	Category     Category            `json:"category"`
	ActionNeeded string              `json:"actionNeeded"`

	// LLM analysis (optional)
	Analysis *LLMAnalysis `json:"analysis,omitempty"`
}

// LLMAnalysis contains Claude's analysis of the item
type LLMAnalysis struct {
	Summary        string   `json:"summary"`
	ActionNeeded   string   `json:"actionNeeded"`
	EffortEstimate string   `json:"effortEstimate"` // quick, medium, large
	Blockers       []string `json:"blockers,omitempty"`
	Tags           []string `json:"tags,omitempty"`
}
