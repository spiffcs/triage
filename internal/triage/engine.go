package triage

import (
	"sort"
	"time"

	"github.com/spiffcs/triage/config"
	"github.com/spiffcs/triage/internal/constants"
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
		priority := e.heuristics.DeterminePriority(&n, score)
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
		PriorityNotable:   3,
		PriorityFYI:       4,
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
	filtered := make([]PrioritizedItem, 0, len(items))
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

	reasonSet := make(map[github.NotificationReason]bool, len(reasons))
	for _, r := range reasons {
		reasonSet[r] = true
	}

	filtered := make([]PrioritizedItem, 0, len(items))
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
	filtered := make([]PrioritizedItem, 0, len(items))
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

	filtered := make([]PrioritizedItem, 0, len(items))
	for _, item := range items {
		if item.Notification.Repository.FullName == repo {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// ResolvedStore is an interface for checking if items should be shown
type ResolvedStore interface {
	ShouldShow(notificationID string, currentUpdatedAt time.Time) bool
}

// FilterResolved filters out items that have been resolved and haven't had new activity
func FilterResolved(items []PrioritizedItem, store ResolvedStore) []PrioritizedItem {
	if store == nil {
		return items
	}

	filtered := make([]PrioritizedItem, 0, len(items))
	for _, item := range items {
		if store.ShouldShow(item.Notification.ID, item.Notification.UpdatedAt) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// FilterByExcludedAuthors removes items authored by users in the exclude list.
// This is useful for filtering out bot accounts like dependabot, renovate, etc.
func FilterByExcludedAuthors(items []PrioritizedItem, excludedAuthors []string) []PrioritizedItem {
	if len(excludedAuthors) == 0 {
		return items
	}

	// Build a set for O(1) lookup
	excludeSet := make(map[string]bool, len(excludedAuthors))
	for _, author := range excludedAuthors {
		excludeSet[author] = true
	}

	filtered := make([]PrioritizedItem, 0, len(items))
	for _, item := range items {
		// Skip items without details (can't determine author)
		if item.Notification.Details == nil {
			filtered = append(filtered, item)
			continue
		}

		// Skip if author is in the exclude list
		if excludeSet[item.Notification.Details.Author] {
			continue
		}

		filtered = append(filtered, item)
	}
	return filtered
}

// FilterByGreenCI keeps only PRs with passing CI status.
// Issues are excluded since they don't have CI.
func FilterByGreenCI(items []PrioritizedItem) []PrioritizedItem {
	filtered := make([]PrioritizedItem, 0, len(items))
	for _, item := range items {
		// Exclude non-PRs (issues don't have CI)
		if item.Notification.Subject.Type != github.SubjectPullRequest {
			continue
		}
		// Exclude PRs without details (can't determine CI status)
		if item.Notification.Details == nil {
			continue
		}
		// Keep PRs with successful CI
		if item.Notification.Details.CIStatus == constants.CIStatusSuccess {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// FilterOutUnenriched removes PR and Issue notifications that couldn't be enriched.
// This typically indicates the item is deleted, inaccessible, or the user lost access.
// Non-PR/Issue types (Release, Discussion) are kept since they don't require enrichment.
func FilterOutUnenriched(items []PrioritizedItem) []PrioritizedItem {
	filtered := make([]PrioritizedItem, 0, len(items))
	for _, item := range items {
		subjectType := item.Notification.Subject.Type

		// Keep non-PR/Issue types - they don't have enrichment
		if subjectType != github.SubjectPullRequest && subjectType != github.SubjectIssue {
			filtered = append(filtered, item)
			continue
		}

		// Keep PR/Issue items that were successfully enriched
		if item.Notification.Details != nil {
			filtered = append(filtered, item)
		}
	}
	return filtered
}
