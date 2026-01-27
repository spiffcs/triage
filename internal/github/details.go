package github

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

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

// EnrichNotificationsConcurrent enriches notifications using a worker pool with caching.
// Returns the number of cache hits and any error.
func (c *Client) EnrichNotificationsConcurrent(notifications []Notification, workers int, onProgress func(completed, total int)) (int, error) {
	if workers <= 0 {
		workers = 10 // Default concurrency
	}

	// Initialize cache
	cache, cacheErr := NewCache()
	if cacheErr != nil {
		log.Warn("cache unavailable", "error", cacheErr)
	}

	total := len(notifications)
	var completed int64
	var errors int64
	var cacheHits int64

	// First pass: check cache
	uncachedIndices := make([]int, 0, len(notifications))
	for i := range notifications {
		if cache != nil {
			if details, ok := cache.Get(&notifications[i]); ok {
				notifications[i].Details = details
				atomic.AddInt64(&cacheHits, 1)
				atomic.AddInt64(&completed, 1)
				if onProgress != nil {
					onProgress(int(completed), total)
				}
				continue
			}
		}
		uncachedIndices = append(uncachedIndices, i)
	}

	if cacheHits > 0 {
		log.Info("cache hit", "count", cacheHits, "total", total)
	}

	if len(uncachedIndices) == 0 {
		return int(cacheHits), nil
	}

	// Create work channel for uncached items
	work := make(chan int, len(uncachedIndices))
	for _, i := range uncachedIndices {
		work <- i
	}
	close(work)

	// Create worker pool
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range work {
				if err := c.EnrichNotification(&notifications[i]); err != nil {
					atomic.AddInt64(&errors, 1)
				} else if cache != nil && notifications[i].Details != nil {
					// Cache successful enrichment
					if err := cache.Set(&notifications[i], notifications[i].Details); err != nil {
						log.Debug("failed to cache notification", "id", notifications[i].ID, "error", err)
					}
				}
				done := atomic.AddInt64(&completed, 1)
				if onProgress != nil {
					onProgress(int(done), total)
				}
			}
		}()
	}

	wg.Wait()

	if errors > 0 {
		log.Warn("some notifications failed to enrich", "count", errors, "note", "may be deleted or inaccessible")
	}

	return int(cacheHits), nil
}

func (c *Client) enrichPullRequest(n *Notification, owner, repo string, number int) error {
	// Fetch PR details and review state concurrently
	var pr *github.PullRequest
	var prErr error
	var reviewState string

	var wg sync.WaitGroup
	wg.Add(2)

	// Fetch PR details
	go func() {
		defer wg.Done()
		pr, _, prErr = c.client.PullRequests.Get(c.ctx, owner, repo, number)
	}()

	// Fetch review state
	go func() {
		defer wg.Done()
		reviewState = c.getPRReviewState(owner, repo, number)
	}()

	wg.Wait()

	if prErr != nil {
		return fmt.Errorf("failed to get PR #%d: %w", number, prErr)
	}

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

	// Set review state (fetched concurrently)
	details.ReviewState = reviewState

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

func (c *Client) getPRReviewState(owner, repo string, number int) string {
	reviews, _, err := c.client.PullRequests.ListReviews(c.ctx, owner, repo, number, &github.ListOptions{
		PerPage: 100,
	})
	if err != nil {
		return "unknown"
	}

	if len(reviews) == 0 {
		return "pending"
	}

	// Single pass: track latest review state per user and determine overall state
	latestReviews := make(map[string]string, len(reviews))
	for _, review := range reviews {
		user := review.GetUser().GetLogin()
		state := review.GetState()
		if state != "" && state != "COMMENTED" && state != "PENDING" {
			latestReviews[user] = state
		}
	}

	// Determine overall state from the map
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
	return "reviewed"
}
