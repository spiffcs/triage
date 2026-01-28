// Package triage provides notification prioritization functionality.
package triage

import "github.com/spiffcs/triage/internal/github"

// Prioritizer defines the interface for notification prioritization.
// This interface enables mocking the prioritization engine in unit tests.
type Prioritizer interface {
	// Prioritize scores and sorts notifications by priority.
	Prioritize(notifications []github.Notification) []PrioritizedItem
}

// Scorer defines the interface for scoring individual notifications.
// This interface enables testing scoring logic independently.
type Scorer interface {
	// Score calculates the priority score for a notification.
	Score(n *github.Notification) int

	// DeterminePriority determines the display priority based on
	// the notification and its score.
	DeterminePriority(n *github.Notification, score int) PriorityLevel

	// DetermineAction suggests what action the user should take.
	DetermineAction(n *github.Notification) string
}

// Ensure Engine implements Prioritizer interface.
var _ Prioritizer = (*Engine)(nil)

// Ensure Heuristics implements Scorer interface.
var _ Scorer = (*Heuristics)(nil)
