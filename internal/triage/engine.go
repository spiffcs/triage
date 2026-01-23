package triage

import (
	"sort"

	"github.com/spiffcs/triage/config"
	"github.com/spiffcs/triage/internal/github"
)

// Engine orchestrates the prioritization process
type Engine struct {
	heuristics *Heuristics
}

// NewEngine creates a new priority engine with the given weights and labels
func NewEngine(currentUser string, weights config.ScoreWeights, quickWinLabels []string) *Engine {
	return &Engine{
		heuristics: NewHeuristics(currentUser, weights, quickWinLabels),
	}
}

// Prioritize scores and sorts notifications by priority
func (e *Engine) Prioritize(notifications []github.Notification) []PrioritizedItem {
	items := make([]PrioritizedItem, 0, len(notifications))

	for _, n := range notifications {
		score := e.heuristics.Score(&n)
		priority := e.heuristics.DeterminePriority(&n)
		action := e.heuristics.DetermineAction(&n)

		items = append(items, PrioritizedItem{
			Notification: n,
			Score:        score,
			Priority:     priority,
			ActionNeeded: action,
		})
	}

	// Sort by priority first, then by score descending within each priority
	priorityOrder := map[PriorityLevel]int{
		PriorityUrgent:    0,
		PriorityImportant: 1,
		PriorityQuickWin:  2,
		PriorityFYI:       3,
	}
	sort.Slice(items, func(i, j int) bool {
		pi, pj := priorityOrder[items[i].Priority], priorityOrder[items[j].Priority]
		if pi != pj {
			return pi < pj
		}
		return items[i].Score > items[j].Score
	})

	return items
}

// FilterByPriority filters items by a specific priority level
func FilterByPriority(items []PrioritizedItem, targetPriority PriorityLevel) []PrioritizedItem {
	filtered := make([]PrioritizedItem, 0)
	for _, item := range items {
		if item.Priority == targetPriority {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// FilterByReason filters items by notification reason
func FilterByReason(items []PrioritizedItem, reasons []github.NotificationReason) []PrioritizedItem {
	if len(reasons) == 0 {
		return items
	}

	reasonSet := make(map[github.NotificationReason]bool)
	for _, r := range reasons {
		reasonSet[r] = true
	}

	filtered := make([]PrioritizedItem, 0)
	for _, item := range items {
		if reasonSet[item.Notification.Reason] {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// FilterOutMerged removes notifications for merged PRs
func FilterOutMerged(items []PrioritizedItem) []PrioritizedItem {
	filtered := make([]PrioritizedItem, 0, len(items))
	for _, item := range items {
		// Skip if it's a merged PR
		if item.Notification.Details != nil && item.Notification.Details.Merged {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

// FilterOutClosed removes notifications for closed issues/PRs
func FilterOutClosed(items []PrioritizedItem) []PrioritizedItem {
	filtered := make([]PrioritizedItem, 0, len(items))
	for _, item := range items {
		if item.Notification.Details != nil {
			state := item.Notification.Details.State
			if state == "closed" || state == "merged" {
				continue
			}
		}
		filtered = append(filtered, item)
	}
	return filtered
}

// FilterByType filters items by subject type (pr, issue)
func FilterByType(items []PrioritizedItem, subjectType github.SubjectType) []PrioritizedItem {
	filtered := make([]PrioritizedItem, 0)
	for _, item := range items {
		if item.Notification.Subject.Type == subjectType {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// FilterByRepo filters items by repository name (owner/repo)
func FilterByRepo(items []PrioritizedItem, repo string) []PrioritizedItem {
	if repo == "" {
		return items
	}

	filtered := make([]PrioritizedItem, 0)
	for _, item := range items {
		if item.Notification.Repository.FullName == repo {
			filtered = append(filtered, item)
		}
	}
	return filtered
}
