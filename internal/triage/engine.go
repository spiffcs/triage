package triage

import (
	"sort"
	"time"

	"github.com/spiffcs/triage/config"
	"github.com/spiffcs/triage/internal/model"
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
func (e *Engine) Prioritize(items []model.Item) []PrioritizedItem {
	pItems := make([]PrioritizedItem, 0, len(items))

	for _, n := range items {
		score := e.heuristics.Score(&n)
		priority := e.heuristics.Priority(&n, score)
		action := e.heuristics.Action(&n)

		pItems = append(pItems, PrioritizedItem{
			Item:         n,
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

	sort.Slice(pItems, func(i, j int) bool {
		pi, pj := priorityOrder[pItems[i].Priority], priorityOrder[pItems[j].Priority]
		if pi != pj {
			return pi < pj
		}
		return pItems[i].Score > pItems[j].Score
	})

	return pItems
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
func FilterByReason(items []PrioritizedItem, reasons []model.ItemReason) []PrioritizedItem {
	if len(reasons) == 0 {
		return items
	}

	reasonSet := make(map[model.ItemReason]bool, len(reasons))
	for _, r := range reasons {
		reasonSet[r] = true
	}

	filtered := make([]PrioritizedItem, 0, len(items))
	for _, item := range items {
		if reasonSet[item.Reason] {
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
		if pr := item.PRDetails(); pr != nil && pr.Merged {
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
		state := item.State
		if state == "closed" || state == "merged" {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

// FilterByType filters items by subject type (pr, issue)
func FilterByType(items []PrioritizedItem, subjectType model.SubjectType) []PrioritizedItem {
	filtered := make([]PrioritizedItem, 0, len(items))
	for _, item := range items {
		if item.Subject.Type == subjectType {
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
		if item.Repository.FullName == repo {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// ResolvedChecker is an interface for checking if items should be shown
type ResolvedChecker interface {
	ShouldShow(notificationID string, currentUpdatedAt time.Time) bool
}

// FilterResolved filters out items that have been resolved and haven't had new activity
func FilterResolved(items []PrioritizedItem, store ResolvedChecker) []PrioritizedItem {
	if store == nil {
		return items
	}

	filtered := make([]PrioritizedItem, 0, len(items))
	for _, item := range items {
		if store.ShouldShow(item.ID, item.UpdatedAt) {
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
		// Skip items without author (can't determine author)
		if item.Author == "" {
			filtered = append(filtered, item)
			continue
		}

		// Skip if author is in the exclude list
		if excludeSet[item.Author] {
			continue
		}

		filtered = append(filtered, item)
	}
	return filtered
}

// FilterByExcludedRepos removes items from repositories in the exclude list.
func FilterByExcludedRepos(items []PrioritizedItem, excludedRepos []string) []PrioritizedItem {
	if len(excludedRepos) == 0 {
		return items
	}

	// Build a set for O(1) lookup
	excludeSet := make(map[string]bool, len(excludedRepos))
	for _, repo := range excludedRepos {
		excludeSet[repo] = true
	}

	filtered := make([]PrioritizedItem, 0, len(items))
	for _, item := range items {
		if excludeSet[item.Repository.FullName] {
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
		if item.Type != model.ItemTypePullRequest {
			continue
		}
		// Exclude PRs without PR details (can't determine CI status)
		pr := item.PRDetails()
		if pr == nil {
			continue
		}
		// Keep PRs with successful CI
		if pr.CIStatus == model.CIStatusSuccess {
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
		subjectType := item.Subject.Type

		// Keep non-PR/Issue types - they don't have enrichment
		if subjectType != model.SubjectPullRequest && subjectType != model.SubjectIssue {
			filtered = append(filtered, item)
			continue
		}

		// Keep PR/Issue items that were successfully enriched
		if item.Details != nil {
			filtered = append(filtered, item)
		}
	}
	return filtered
}
