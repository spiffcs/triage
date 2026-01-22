package priority

import (
	"sort"

	"github.com/hal/priority/internal/github"
)

// Engine orchestrates the prioritization process
type Engine struct {
	heuristics *Heuristics
}

// NewEngine creates a new priority engine
func NewEngine(currentUser string) *Engine {
	return &Engine{
		heuristics: NewHeuristics(currentUser),
	}
}

// Prioritize scores and sorts notifications by priority
func (e *Engine) Prioritize(notifications []github.Notification) []PrioritizedItem {
	items := make([]PrioritizedItem, 0, len(notifications))

	for _, n := range notifications {
		score := e.heuristics.Score(&n)
		priority := e.heuristics.DeterminePriority(&n)
		action := e.heuristics.DetermineAction(&n)
		category := DetermineCategory(score)

		items = append(items, PrioritizedItem{
			Notification: n,
			Score:        score,
			Priority:     priority,
			Category:     category,
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

// FilterByCategory filters items to a specific category
func FilterByCategory(items []PrioritizedItem, category Category) []PrioritizedItem {
	filtered := make([]PrioritizedItem, 0)
	for _, item := range items {
		if item.Category == category {
			filtered = append(filtered, item)
		}
	}
	return filtered
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

// Summary provides an overview of prioritized items
type Summary struct {
	Total       int            `json:"total"`
	ByCategory  map[Category]int `json:"byCategory"`
	ByPriority  map[PriorityLevel]int `json:"byPriority"`
	TopUrgent   []PrioritizedItem `json:"topUrgent"`
	QuickWins   []PrioritizedItem `json:"quickWins"`
}

// Summarize creates a summary of the prioritized items
func Summarize(items []PrioritizedItem) Summary {
	summary := Summary{
		Total:      len(items),
		ByCategory: make(map[Category]int),
		ByPriority: make(map[PriorityLevel]int),
	}

	for _, item := range items {
		summary.ByCategory[item.Category]++
		summary.ByPriority[item.Priority]++
	}

	// Top urgent items
	for _, item := range items {
		if item.Priority == PriorityUrgent && len(summary.TopUrgent) < 5 {
			summary.TopUrgent = append(summary.TopUrgent, item)
		}
	}

	// Quick wins
	for _, item := range items {
		if item.Priority == PriorityQuickWin && len(summary.QuickWins) < 5 {
			summary.QuickWins = append(summary.QuickWins, item)
		}
	}

	return summary
}
