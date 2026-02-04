// Package triage provides notification prioritization functionality.
package triage

import (
	"github.com/spiffcs/triage/internal/model"
)

// Prioritizer defines the interface for notification prioritization.
// This interface enables mocking the prioritization engine in unit tests.
type Prioritizer interface {
	// Prioritize scores and sorts notifications by priority.
	Prioritize(items []model.Item) []PrioritizedItem
}

// Scorer defines the interface for scoring individual notifications.
// This interface enables testing scoring logic independently.
type Scorer interface {
	// Score calculates the priority score for a notification.
	Score(n *model.Item) int

	// Priority determines the display priority based on
	// the notification and its score.
	Priority(n *model.Item, score int) PriorityLevel

	// Action suggests what action the user should take.
	Action(n *model.Item) string
}

// Ensure Engine implements Prioritizer interface.
var _ Prioritizer = (*Engine)(nil)

// Ensure Heuristics implements Scorer interface.
var _ Scorer = (*Heuristics)(nil)
