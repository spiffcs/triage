package github

import (
	"fmt"
	"strings"
	"sync"

	"github.com/google/go-github/v57/github"
	"github.com/spiffcs/triage/internal/log"
)

// EnrichmentProgress tracks enrichment progress
type EnrichmentProgress struct {
	Total     int
	Completed int64
	Errors    int64
}

// EnrichNotification fetches additional details for a notification
func (c *Client) EnrichNotification(n *Notification) error {
	if n.Subject.URL == "" {
		return nil
	}

	// Extract owner/repo from repository
	parts := strings.Split(n.Repository.FullName, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid repository name: %s", n.Repository.FullName)
	}
	owner, repo := parts[0], parts[1]

	// Extract issue/PR number from URL
	number, err := ExtractIssueNumber(n.Subject.URL)
	if err != nil {
		return err
	}

	switch n.Subject.Type {
	case SubjectPullRequest:
		return c.enrichPullRequest(n, owner, repo, number)
	case SubjectIssue:
		return c.enrichIssue(n, owner, repo, number)
	default:
		// Skip enrichment for other types
		return nil
	}
}

// EnrichNotifications enriches multiple notifications sequentially (for small batches)
func (c *Client) EnrichNotifications(notifications []Notification) error {
	return c.EnrichNotificationsWithProgress(notifications, nil)
}

// EnrichNotificationsWithProgress enriches notifications with progress callback
func (c *Client) EnrichNotificationsWithProgress(notifications []Notification, onProgress func(completed, total int)) error {
	total := len(notifications)
	for i := range notifications {
		if err := c.EnrichNotification(&notifications[i]); err != nil {
			log.Warn("failed to enrich notification", "id", notifications[i].ID, "error", err)
		}
		if onProgress != nil {
			onProgress(i+1, total)
		}
	}
	return nil
}

// EnrichNotificationsConcurrent enriches notifications using GraphQL batch queries with caching.
// Uses GraphQL API (separate quota from Core API) for efficient batch enrichment.
// Returns the number of cache hits and any error.
func (c *Client) EnrichNotificationsConcurrent(notifications []Notification, workers int, onProgress func(completed, total int)) (int, error) {
	// Initialize cache
	cache, cacheErr := NewCache()
	if cacheErr != nil {
		log.Debug("cache unavailable", "error", cacheErr)
	}

	total := len(notifications)
	var cacheHits int64

	// First pass: check cache and build list of items needing enrichment
	uncachedNotifications := make([]Notification, 0, len(notifications))
	uncachedIndices := make([]int, 0, len(notifications))

	for i := range notifications {
		if cache != nil {
			if details, ok := cache.Get(&notifications[i]); ok {
				notifications[i].Details = details
				cacheHits++
				continue
			}
		}
		uncachedNotifications = append(uncachedNotifications, notifications[i])
		uncachedIndices = append(uncachedIndices, i)
	}

	// Report cache hits as a single progress update (delta = cacheHits)
	if cacheHits > 0 && onProgress != nil {
		onProgress(int(cacheHits), total)
	}

	if cacheHits > 0 {
		log.Info("cache hit", "count", cacheHits, "total", total)
	}

	if len(uncachedNotifications) == 0 {
		return int(cacheHits), nil
	}

	// Use GraphQL for batch enrichment (uses GraphQL quota, not Core API)
	log.Debug("enriching via GraphQL", "count", len(uncachedNotifications))

	enriched, err := c.EnrichNotificationsGraphQL(uncachedNotifications, c.token, func(delta, batchTotal int) {
		if onProgress != nil {
			// Pass through the delta (number of items just processed)
			onProgress(delta, total)
		}
	})

	if err != nil {
		log.Debug("GraphQL enrichment error", "error", err)
	}

	// Copy enriched data back to original slice and cache results
	for i, origIdx := range uncachedIndices {
		notifications[origIdx] = uncachedNotifications[i]
		// Log what we're copying back
		n := &notifications[origIdx]
		if n.Details != nil {
			log.Debug("enriched notification",
				"id", n.ID,
				"repo", n.Repository.FullName,
				"isPR", n.Details.IsPR,
				"additions", n.Details.Additions,
				"deletions", n.Details.Deletions,
				"reviewState", n.Details.ReviewState)
		} else {
			log.Debug("notification not enriched - Details is nil", "id", n.ID, "repo", n.Repository.FullName)
		}
		// Cache successful enrichment
		if cache != nil && uncachedNotifications[i].Details != nil {
			if err := cache.Set(&notifications[origIdx], notifications[origIdx].Details); err != nil {
				log.Debug("failed to cache notification", "id", notifications[origIdx].ID, "error", err)
			}
		}
	}

	log.Debug("GraphQL enrichment complete", "enriched", enriched, "total", len(uncachedNotifications))

	return int(cacheHits), nil
}

func (c *Client) enrichPullRequest(n *Notification, owner, repo string, number int) error {
	var pr *github.PullRequest
	var prErr error
	var reviewState string
	var ciStatus string

	var wg sync.WaitGroup

	// Fetch review state concurrently (doesn't need PR details)
	wg.Add(1)
	go func() {
		defer wg.Done()
		reviewState = c.getPRReviewState(owner, repo, number)
	}()

	// Fetch PR details first (needed for CI status SHA)
	pr, _, prErr = c.client.PullRequests.Get(c.ctx, owner, repo, number)
	if prErr != nil {
		wg.Wait() // Wait for review state goroutine before returning
		return fmt.Errorf("failed to get PR #%d: %w", number, prErr)
	}

	// Fetch CI status concurrently (now that we have the SHA)
	wg.Add(1)
	go func() {
		defer wg.Done()
		ciStatus = c.getCIStatus(owner, repo, pr.GetHead().GetSHA())
	}()

	// Wait for review state and CI status to complete
	wg.Wait()

	details := &ItemDetails{
		Number:       number,
		State:        pr.GetState(),
		HTMLURL:      pr.GetHTMLURL(),
		CreatedAt:    pr.GetCreatedAt().Time,
		UpdatedAt:    pr.GetUpdatedAt().Time,
		Author:       pr.GetUser().GetLogin(),
		CommentCount: pr.GetComments() + pr.GetReviewComments(),
		IsPR:         true,
		Merged:       pr.GetMerged(),
		Additions:    pr.GetAdditions(),
		Deletions:    pr.GetDeletions(),
		ChangedFiles: pr.GetChangedFiles(),
		ReviewState:  reviewState,
		CIStatus:     ciStatus,
	}

	if pr.ClosedAt != nil {
		closedAt := pr.GetClosedAt().Time
		details.ClosedAt = &closedAt
	}

	if pr.MergedAt != nil {
		mergedAt := pr.GetMergedAt().Time
		details.MergedAt = &mergedAt
		details.State = "merged"
	}

	// Get assignees
	for _, assignee := range pr.Assignees {
		details.Assignees = append(details.Assignees, assignee.GetLogin())
	}

	// Get labels
	for _, label := range pr.Labels {
		details.Labels = append(details.Labels, label.GetName())
	}

	n.Details = details
	return nil
}

func (c *Client) enrichIssue(n *Notification, owner, repo string, number int) error {
	issue, _, err := c.client.Issues.Get(c.ctx, owner, repo, number)
	if err != nil {
		return fmt.Errorf("failed to get issue #%d: %w", number, err)
	}

	details := &ItemDetails{
		Number:       number,
		State:        issue.GetState(),
		HTMLURL:      issue.GetHTMLURL(),
		CreatedAt:    issue.GetCreatedAt().Time,
		UpdatedAt:    issue.GetUpdatedAt().Time,
		Author:       issue.GetUser().GetLogin(),
		CommentCount: issue.GetComments(),
		IsPR:         false,
	}

	if issue.ClosedAt != nil {
		closedAt := issue.GetClosedAt().Time
		details.ClosedAt = &closedAt
	}

	// Get assignees
	for _, assignee := range issue.Assignees {
		details.Assignees = append(details.Assignees, assignee.GetLogin())
	}

	// Get labels
	for _, label := range issue.Labels {
		details.Labels = append(details.Labels, label.GetName())
	}

	// Fetch last commenter if there are comments
	if issue.GetComments() > 0 {
		comments, _, err := c.client.Issues.ListComments(c.ctx, owner, repo, number, &github.IssueListCommentsOptions{
			Sort:      github.String("created"),
			Direction: github.String("desc"),
			ListOptions: github.ListOptions{
				PerPage: 1,
			},
		})
		if err == nil && len(comments) > 0 {
			details.LastCommenter = comments[0].GetUser().GetLogin()
		}
	}

	n.Details = details
	return nil
}

func (c *Client) getCIStatus(owner, repo, ref string) string {
	// Check rate limit before making API call
	if globalRateLimitState.IsLimited() {
		return ""
	}

	checkRuns, _, err := c.client.Checks.ListCheckRunsForRef(c.ctx, owner, repo, ref, nil)
	if err != nil {
		log.Debug("failed to get CI status", "owner", owner, "repo", repo, "ref", ref, "error", err)
		return ""
	}

	if checkRuns == nil || len(checkRuns.CheckRuns) == 0 {
		return ""
	}

	hasFailure := false
	hasPending := false
	allSuccessful := true

	for _, run := range checkRuns.CheckRuns {
		status := run.GetStatus()
		conclusion := run.GetConclusion()

		// Check for failures first
		if conclusion == "failure" || conclusion == "timed_out" || conclusion == "action_required" {
			hasFailure = true
			allSuccessful = false
		} else if status == "queued" || status == "in_progress" {
			hasPending = true
			allSuccessful = false
		} else if conclusion != "success" && conclusion != "skipped" && conclusion != "neutral" {
			// Unknown or cancelled - not a success
			allSuccessful = false
		}
	}

	if hasFailure {
		return "failure"
	}
	if hasPending {
		return "pending"
	}
	if allSuccessful {
		return "success"
	}
	return ""
}

func (c *Client) getPRReviewState(owner, repo string, number int) string {
	reviews, _, err := c.client.PullRequests.ListReviews(c.ctx, owner, repo, number, &github.ListOptions{
		PerPage: 100,
	})
	if err != nil {
		return "unknown"
	}

	// Track the latest review state per user
	latestReviews := make(map[string]string)
	for _, review := range reviews {
		user := review.GetUser().GetLogin()
		state := review.GetState()
		if state != "" && state != "COMMENTED" && state != "PENDING" {
			latestReviews[user] = state
		}
	}

	// Determine overall state
	hasApproval := false
	hasChangesRequested := false

	for _, state := range latestReviews {
		switch state {
		case "APPROVED":
			hasApproval = true
		case "CHANGES_REQUESTED":
			hasChangesRequested = true
		}
	}

	if hasChangesRequested {
		return "changes_requested"
	}
	if hasApproval {
		return "approved"
	}
	if len(reviews) > 0 {
		return "reviewed"
	}
	return "pending"
}
