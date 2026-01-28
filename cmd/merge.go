package cmd

import (
	"fmt"

	"github.com/spiffcs/triage/internal/github"
)

// MergeResult contains the counts of items added during merge operations.
type MergeResult struct {
	ReviewPRsAdded      int
	AuthoredPRsAdded    int
	AssignedIssuesAdded int
}

// MergeAll merges all additional data sources into the notifications list.
// It returns the merged list and counts of items added from each source.
func MergeAll(notifications []github.Notification, reviewPRs, authoredPRs, assignedIssues []github.Notification) ([]github.Notification, MergeResult) {
	result := MergeResult{}

	if len(reviewPRs) > 0 {
		notifications, result.ReviewPRsAdded = MergeReviewRequests(notifications, reviewPRs)
	}
	if len(authoredPRs) > 0 {
		notifications, result.AuthoredPRsAdded = MergeAuthoredPRs(notifications, authoredPRs)
	}
	if len(assignedIssues) > 0 {
		notifications, result.AssignedIssuesAdded = MergeAssignedIssues(notifications, assignedIssues)
	}

	return notifications, result
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

// MergeReviewRequests adds review-requested PRs that aren't already in notifications.
// Returns the merged list and the count of newly added items.
func MergeReviewRequests(notifications []github.Notification, reviewPRs []github.Notification) ([]github.Notification, int) {
	return mergeNotifications(notifications, reviewPRs, github.SubjectPullRequest)
}

// MergeAuthoredPRs adds user's open PRs that aren't already in notifications.
// Returns the merged list and the count of newly added items.
func MergeAuthoredPRs(notifications []github.Notification, authoredPRs []github.Notification) ([]github.Notification, int) {
	return mergeNotifications(notifications, authoredPRs, github.SubjectPullRequest)
}

// MergeAssignedIssues adds user's assigned issues that aren't already in notifications.
// Returns the merged list and the count of newly added items.
func MergeAssignedIssues(notifications []github.Notification, assignedIssues []github.Notification) ([]github.Notification, int) {
	return mergeNotifications(notifications, assignedIssues, github.SubjectIssue)
}
