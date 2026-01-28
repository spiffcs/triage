package cmd

import (
	"fmt"

	"github.com/spiffcs/triage/internal/github"
)

// mergeResult contains the counts of items added during merge operations.
type mergeResult struct {
	ReviewPRsAdded      int
	AuthoredPRsAdded    int
	AssignedIssuesAdded int
	OrphanedAdded       int
}

// mergeAll merges all additional data sources into the notifications list.
// It returns the merged list and counts of items added from each source.
func mergeAll(notifications []github.Notification, reviewPRs, authoredPRs, assignedIssues, orphaned []github.Notification) ([]github.Notification, mergeResult) {
	result := mergeResult{}

	if len(reviewPRs) > 0 {
		notifications, result.ReviewPRsAdded = mergeReviewRequests(notifications, reviewPRs)
	}
	if len(authoredPRs) > 0 {
		notifications, result.AuthoredPRsAdded = mergeAuthoredPRs(notifications, authoredPRs)
	}
	if len(assignedIssues) > 0 {
		notifications, result.AssignedIssuesAdded = mergeAssignedIssues(notifications, assignedIssues)
	}
	if len(orphaned) > 0 {
		notifications, result.OrphanedAdded = mergeOrphaned(notifications, orphaned)
	}

	return notifications, result
}

// mergeOrphaned adds orphaned contributions that aren't already in notifications.
// Returns the merged list and the count of newly added items.
func mergeOrphaned(notifications []github.Notification, orphaned []github.Notification) ([]github.Notification, int) {
	// Orphaned items can be either PRs or issues, so we need to check both types
	// Build sets of existing identifiers for all items
	existing := make(map[string]bool)
	existingURLs := make(map[string]bool)

	for _, n := range notifications {
		if n.Subject.URL != "" {
			existingURLs[n.Subject.URL] = true
		}
		if n.Details != nil {
			key := fmt.Sprintf("%s#%d", n.Repository.FullName, n.Details.Number)
			existing[key] = true
		}
	}

	// Add items that aren't already in the list
	added := 0
	for _, item := range orphaned {
		if item.Details == nil {
			continue
		}

		key := fmt.Sprintf("%s#%d", item.Repository.FullName, item.Details.Number)
		if existing[key] || existingURLs[item.Subject.URL] {
			continue
		}

		notifications = append(notifications, item)
		existing[key] = true
		added++
	}

	return notifications, added
}

// mergeNotifications adds items that aren't already in the notifications list.
// It filters existing notifications by subjectType and checks for duplicates
// by repo#number and Subject.URL. Returns the merged list and count of added items.
func mergeNotifications(
	notifications []github.Notification,
	newItems []github.Notification,
	subjectType github.SubjectType,
) ([]github.Notification, int) {
	// Build sets of existing identifiers for items matching the subject type
	existing := make(map[string]bool)
	existingURLs := make(map[string]bool)

	for _, n := range notifications {
		if n.Subject.Type == subjectType {
			if n.Subject.URL != "" {
				existingURLs[n.Subject.URL] = true
			}
			if n.Details != nil {
				key := fmt.Sprintf("%s#%d", n.Repository.FullName, n.Details.Number)
				existing[key] = true
			}
		}
	}

	// Add items that aren't already in the list
	added := 0
	for _, item := range newItems {
		if item.Details == nil {
			continue
		}

		key := fmt.Sprintf("%s#%d", item.Repository.FullName, item.Details.Number)
		if existing[key] || existingURLs[item.Subject.URL] {
			continue
		}

		notifications = append(notifications, item)
		existing[key] = true
		added++
	}

	return notifications, added
}

// mergeReviewRequests adds review-requested PRs that aren't already in notifications.
// Returns the merged list and the count of newly added items.
func mergeReviewRequests(notifications []github.Notification, reviewPRs []github.Notification) ([]github.Notification, int) {
	return mergeNotifications(notifications, reviewPRs, github.SubjectPullRequest)
}

// mergeAuthoredPRs adds user's open PRs that aren't already in notifications.
// Returns the merged list and the count of newly added items.
func mergeAuthoredPRs(notifications []github.Notification, authoredPRs []github.Notification) ([]github.Notification, int) {
	return mergeNotifications(notifications, authoredPRs, github.SubjectPullRequest)
}

// mergeAssignedIssues adds user's assigned issues that aren't already in notifications.
// Returns the merged list and the count of newly added items.
func mergeAssignedIssues(notifications []github.Notification, assignedIssues []github.Notification) ([]github.Notification, int) {
	return mergeNotifications(notifications, assignedIssues, github.SubjectIssue)
}
