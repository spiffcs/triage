package github

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/go-github/v57/github"
	"github.com/spiffcs/triage/internal/log"
	"golang.org/x/oauth2"
)

// rateLimitTransport wraps an http.RoundTripper to handle GitHub rate limits
type rateLimitTransport struct {
	base       http.RoundTripper
	maxRetries int
}

func (t *rateLimitTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error

	maxRetries := t.maxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Clone the request for retry (body already consumed)
			req = req.Clone(req.Context())
		}

		resp, err = t.base.RoundTrip(req)
		if err != nil {
			return resp, err
		}

		// Check rate limit headers and log warning if low
		if remaining := resp.Header.Get("X-RateLimit-Remaining"); remaining != "" {
			if rem, parseErr := strconv.Atoi(remaining); parseErr == nil && rem <= 100 && rem > 0 {
				resetStr := resp.Header.Get("X-RateLimit-Reset")
				if resetTime, parseErr := strconv.ParseInt(resetStr, 10, 64); parseErr == nil {
					resetAt := time.Unix(resetTime, 0)
					log.Debug("rate limit low", "remaining", rem, "resets_at", resetAt.Format(time.RFC3339))
				}
			}
		}

		// Handle rate limit responses (403 with rate limit exceeded or 429)
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
			// Check if this is actually a rate limit error
			if resp.Header.Get("X-RateLimit-Remaining") == "0" || resp.StatusCode == http.StatusTooManyRequests {
				resetStr := resp.Header.Get("X-RateLimit-Reset")
				var waitDuration time.Duration

				if resetTime, parseErr := strconv.ParseInt(resetStr, 10, 64); parseErr == nil {
					resetAt := time.Unix(resetTime, 0)
					waitDuration = time.Until(resetAt)
					// Cap wait time at 60 seconds per retry
					if waitDuration > 60*time.Second {
						waitDuration = 60 * time.Second
					}
					// Ensure minimum wait
					if waitDuration < time.Second {
						waitDuration = time.Duration(1<<attempt) * time.Second // Exponential backoff: 1s, 2s, 4s
					}
				} else {
					// No reset header, use exponential backoff
					waitDuration = time.Duration(1<<attempt) * time.Second
				}

				if attempt < maxRetries {
					log.Warn("rate limited, retrying",
						"attempt", attempt+1,
						"max_retries", maxRetries,
						"wait", waitDuration.Round(time.Second))
					_ = resp.Body.Close()

					select {
					case <-time.After(waitDuration):
						continue
					case <-req.Context().Done():
						return nil, req.Context().Err()
					}
				}
			}
		}

		// Success or non-retryable error
		break
	}

	return resp, err
}

// Client wraps the GitHub API client
type Client struct {
	client *github.Client
	ctx    context.Context
}

// NewClient creates a new GitHub client using a personal access token
func NewClient(token string) (*Client, error) {
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	if token == "" {
		return nil, fmt.Errorf("GitHub token not provided. Set the GITHUB_TOKEN environment variable")
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)

	// Wrap transport with rate limit handling
	tc.Transport = &rateLimitTransport{
		base:       tc.Transport,
		maxRetries: 3,
	}

	client := github.NewClient(tc)

	return &Client{
		client: client,
		ctx:    ctx,
	}, nil
}

// GetAuthenticatedUser returns the authenticated user's login
func (c *Client) GetAuthenticatedUser() (string, error) {
	user, _, err := c.client.Users.Get(c.ctx, "")
	if err != nil {
		return "", fmt.Errorf("failed to get authenticated user: %w", err)
	}
	return user.GetLogin(), nil
}

// RawClient returns the underlying go-github client for advanced operations
func (c *Client) RawClient() *github.Client {
	return c.client
}

// Context returns the client's context
func (c *Client) Context() context.Context {
	return c.ctx
}

// ListReviewRequestedPRs fetches open PRs where the user is a requested reviewer
func (c *Client) ListReviewRequestedPRs(username string) ([]Notification, error) {
	query := fmt.Sprintf("is:pr is:open review-requested:%s", username)

	opts := &github.SearchOptions{
		Sort:  "updated",
		Order: "desc",
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	var notifications []Notification

	for {
		result, resp, err := c.client.Search.Issues(c.ctx, query, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to search for review-requested PRs: %w", err)
		}

		for _, issue := range result.Issues {
			// Convert search result to notification format
			notification := c.prToNotification(issue)
			notifications = append(notifications, notification)
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return notifications, nil
}

// prToNotification converts a PR from search results to a Notification
func (c *Client) prToNotification(issue *github.Issue) Notification {
	// Extract owner/repo from the repository URL
	// Issue.RepositoryURL format: https://api.github.com/repos/owner/repo
	repoURL := issue.GetRepositoryURL()
	parts := splitRepoURL(repoURL)
	fullName := ""
	repoName := ""
	if len(parts) >= 2 {
		fullName = parts[0] + "/" + parts[1]
		repoName = parts[1]
	}

	notification := Notification{
		ID:        fmt.Sprintf("review-requested-%d", issue.GetID()),
		Reason:    ReasonReviewRequested,
		Unread:    true, // Treat as unread since review is pending
		UpdatedAt: issue.GetUpdatedAt().Time,
		Repository: Repository{
			Name:     repoName,
			FullName: fullName,
			HTMLURL:  fmt.Sprintf("https://github.com/%s", fullName),
		},
		Subject: Subject{
			Title: issue.GetTitle(),
			URL:   issue.GetURL(),
			Type:  SubjectPullRequest,
		},
		URL: issue.GetURL(),
		Details: &ItemDetails{
			Number:       issue.GetNumber(),
			State:        issue.GetState(),
			HTMLURL:      issue.GetHTMLURL(),
			CreatedAt:    issue.GetCreatedAt().Time,
			UpdatedAt:    issue.GetUpdatedAt().Time,
			Author:       issue.GetUser().GetLogin(),
			CommentCount: issue.GetComments(),
			IsPR:         true,
			ReviewState:  "pending",
		},
	}

	// Extract labels
	for _, label := range issue.Labels {
		notification.Details.Labels = append(notification.Details.Labels, label.GetName())
	}

	// Extract assignees
	for _, assignee := range issue.Assignees {
		notification.Details.Assignees = append(notification.Details.Assignees, assignee.GetLogin())
	}

	return notification
}

// splitRepoURL extracts owner and repo from a GitHub API repo URL
func splitRepoURL(url string) []string {
	// URL format: https://api.github.com/repos/owner/repo
	parts := []string{}
	if url == "" {
		return parts
	}
	// Find the /repos/ part and extract what follows
	idx := len("https://api.github.com/repos/")
	if len(url) <= idx {
		return parts
	}
	remainder := url[idx:]
	// Split by /
	for _, p := range splitBySlash(remainder) {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

func splitBySlash(s string) []string {
	var result []string
	current := ""
	for _, c := range s {
		if c == '/' {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

// ListAssignedIssues fetches open issues assigned to the user
func (c *Client) ListAssignedIssues(username string) ([]Notification, error) {
	query := fmt.Sprintf("is:issue is:open assignee:%s", username)

	opts := &github.SearchOptions{
		Sort:  "updated",
		Order: "desc",
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	var notifications []Notification

	for {
		result, resp, err := c.client.Search.Issues(c.ctx, query, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to search for assigned issues: %w", err)
		}

		for _, issue := range result.Issues {
			notification := c.assignedIssueToNotification(issue)
			notifications = append(notifications, notification)
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return notifications, nil
}

// assignedIssueToNotification converts an assigned issue to a Notification
func (c *Client) assignedIssueToNotification(issue *github.Issue) Notification {
	repoURL := issue.GetRepositoryURL()
	parts := splitRepoURL(repoURL)
	fullName := ""
	repoName := ""
	if len(parts) >= 2 {
		fullName = parts[0] + "/" + parts[1]
		repoName = parts[1]
	}

	notification := Notification{
		ID:        fmt.Sprintf("assigned-%d", issue.GetID()),
		Reason:    ReasonAssign,
		Unread:    true,
		UpdatedAt: issue.GetUpdatedAt().Time,
		Repository: Repository{
			Name:     repoName,
			FullName: fullName,
			HTMLURL:  fmt.Sprintf("https://github.com/%s", fullName),
		},
		Subject: Subject{
			Title: issue.GetTitle(),
			URL:   issue.GetURL(),
			Type:  SubjectIssue,
		},
		URL: issue.GetURL(),
		Details: &ItemDetails{
			Number:       issue.GetNumber(),
			State:        issue.GetState(),
			HTMLURL:      issue.GetHTMLURL(),
			CreatedAt:    issue.GetCreatedAt().Time,
			UpdatedAt:    issue.GetUpdatedAt().Time,
			Author:       issue.GetUser().GetLogin(),
			CommentCount: issue.GetComments(),
			IsPR:         false,
		},
	}

	// Extract labels
	for _, label := range issue.Labels {
		notification.Details.Labels = append(notification.Details.Labels, label.GetName())
	}

	// Extract assignees
	for _, assignee := range issue.Assignees {
		notification.Details.Assignees = append(notification.Details.Assignees, assignee.GetLogin())
	}

	return notification
}

// ListAuthoredPRs fetches open PRs authored by the user
func (c *Client) ListAuthoredPRs(username string) ([]Notification, error) {
	query := fmt.Sprintf("is:pr is:open author:%s", username)

	opts := &github.SearchOptions{
		Sort:  "updated",
		Order: "desc",
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	var notifications []Notification

	for {
		result, resp, err := c.client.Search.Issues(c.ctx, query, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to search for authored PRs: %w", err)
		}

		for _, issue := range result.Issues {
			notification := c.authoredPRToNotification(issue)
			notifications = append(notifications, notification)
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return notifications, nil
}

// authoredPRToNotification converts an authored PR to a Notification
func (c *Client) authoredPRToNotification(issue *github.Issue) Notification {
	repoURL := issue.GetRepositoryURL()
	parts := splitRepoURL(repoURL)
	fullName := ""
	repoName := ""
	owner := ""
	if len(parts) >= 2 {
		owner = parts[0]
		repoName = parts[1]
		fullName = owner + "/" + repoName
	}

	notification := Notification{
		ID:        fmt.Sprintf("authored-%d", issue.GetID()),
		Reason:    ReasonAuthor,
		Unread:    true,
		UpdatedAt: issue.GetUpdatedAt().Time,
		Repository: Repository{
			Name:     repoName,
			FullName: fullName,
			HTMLURL:  fmt.Sprintf("https://github.com/%s", fullName),
		},
		Subject: Subject{
			Title: issue.GetTitle(),
			URL:   issue.GetURL(),
			Type:  SubjectPullRequest,
		},
		URL: issue.GetURL(),
		Details: &ItemDetails{
			Number:       issue.GetNumber(),
			State:        issue.GetState(),
			HTMLURL:      issue.GetHTMLURL(),
			CreatedAt:    issue.GetCreatedAt().Time,
			UpdatedAt:    issue.GetUpdatedAt().Time,
			Author:       issue.GetUser().GetLogin(),
			CommentCount: issue.GetComments(),
			IsPR:         true,
			Draft:        issue.GetDraft(),
		},
	}

	// Extract labels
	for _, label := range issue.Labels {
		notification.Details.Labels = append(notification.Details.Labels, label.GetName())
	}

	// Extract assignees
	for _, assignee := range issue.Assignees {
		notification.Details.Assignees = append(notification.Details.Assignees, assignee.GetLogin())
	}

	return notification
}

// ListReviewRequestedPRsCached fetches PRs with caching support
func (c *Client) ListReviewRequestedPRsCached(username string, cache *Cache) ([]Notification, bool, error) {
	// Check cache first
	if cache != nil {
		if prs, ok := cache.GetPRList(username, "review-requested"); ok {
			return prs, true, nil
		}
	}

	// Fetch from API
	prs, err := c.ListReviewRequestedPRs(username)
	if err != nil {
		return nil, false, err
	}

	// Cache the result
	if cache != nil {
		if err := cache.SetPRList(username, "review-requested", prs); err != nil {
			log.Debug("failed to cache review-requested PRs", "error", err)
		}
	}

	return prs, false, nil
}

// ListAuthoredPRsCached fetches authored PRs with caching support
func (c *Client) ListAuthoredPRsCached(username string, cache *Cache) ([]Notification, bool, error) {
	// Check cache first
	if cache != nil {
		if prs, ok := cache.GetPRList(username, "authored"); ok {
			return prs, true, nil
		}
	}

	// Fetch from API
	prs, err := c.ListAuthoredPRs(username)
	if err != nil {
		return nil, false, err
	}

	// Cache the result
	if cache != nil {
		if err := cache.SetPRList(username, "authored", prs); err != nil {
			log.Debug("failed to cache authored PRs", "error", err)
		}
	}

	return prs, false, nil
}

// ListAssignedIssuesCached fetches assigned issues with caching support
func (c *Client) ListAssignedIssuesCached(username string, cache *Cache) ([]Notification, bool, error) {
	// Check cache first
	if cache != nil {
		if issues, ok := cache.GetPRList(username, "assigned-issues"); ok {
			return issues, true, nil
		}
	}

	// Fetch from API
	issues, err := c.ListAssignedIssues(username)
	if err != nil {
		return nil, false, err
	}

	// Cache the result
	if cache != nil {
		if err := cache.SetPRList(username, "assigned-issues", issues); err != nil {
			log.Debug("failed to cache assigned issues", "error", err)
		}
	}

	return issues, false, nil
}

// NotificationFetchResult contains the result of a cached notification fetch
type NotificationFetchResult struct {
	Notifications []Notification
	FromCache     bool
	NewCount      int // Number of new notifications fetched (for incremental updates)
}

// ListUnreadNotificationsCached fetches notifications with incremental caching.
// It returns cached notifications merged with any new ones since the last fetch.
func (c *Client) ListUnreadNotificationsCached(username string, since time.Time, cache *Cache) (*NotificationFetchResult, error) {
	result := &NotificationFetchResult{}

	// Check cache
	if cache != nil {
		cached, lastFetch, ok := cache.GetNotificationList(username, since)
		if ok {
			// Fetch only NEW notifications since last fetch
			newNotifs, err := c.ListUnreadNotifications(lastFetch)
			if err != nil {
				// Return cached on error
				log.Warn("failed to fetch new notifications, using cache", "error", err)
				result.Notifications = cached
				result.FromCache = true
				return result, nil
			}

			// Merge: new notifications replace old ones by ID
			merged := mergeNotifications(cached, newNotifs, since)
			result.Notifications = merged
			result.FromCache = true
			result.NewCount = len(newNotifs)

			// Update cache with merged result
			if err := cache.SetNotificationList(username, merged, since); err != nil {
				log.Debug("failed to update notification cache", "error", err)
			}

			return result, nil
		}
	}

	// No cache - full fetch
	notifications, err := c.ListUnreadNotifications(since)
	if err != nil {
		return nil, err
	}

	result.Notifications = notifications
	result.FromCache = false

	// Cache result
	if cache != nil {
		if err := cache.SetNotificationList(username, notifications, since); err != nil {
			log.Debug("failed to cache notifications", "error", err)
		}
	}

	return result, nil
}

// mergeNotifications merges cached and fresh notifications.
// Fresh notifications replace cached ones by ID. Only unread notifications
// within the since timeframe are kept.
func mergeNotifications(cached, fresh []Notification, since time.Time) []Notification {
	byID := make(map[string]Notification)

	// Add cached notifications
	for _, n := range cached {
		byID[n.ID] = n
	}

	// New overwrites old
	for _, n := range fresh {
		byID[n.ID] = n
	}

	// Build result, filtering appropriately
	result := make([]Notification, 0, len(byID))
	for _, n := range byID {
		// Only keep unread notifications
		if !n.Unread {
			continue
		}
		// Only keep notifications within the since timeframe
		if n.UpdatedAt.Before(since) {
			continue
		}
		result = append(result, n)
	}

	return result
}

// EnrichPRsConcurrent enriches PRs (review-requested or authored) using a worker pool with caching.
// Returns the number of cache hits.
func (c *Client) EnrichPRsConcurrent(notifications []Notification, workers int, cache *Cache, onProgress func(completed, total int)) (int, error) {
	if workers <= 0 {
		workers = 10
	}

	total := len(notifications)
	if total == 0 {
		return 0, nil
	}

	var completed int64
	var errors int64
	var cacheHits int64

	// First pass: check cache
	uncachedIndices := make([]int, 0, total)
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
				if err := c.EnrichAuthoredPR(&notifications[i]); err != nil {
					atomic.AddInt64(&errors, 1)
				} else if cache != nil && notifications[i].Details != nil {
					// Cache successful enrichment
					if err := cache.Set(&notifications[i], notifications[i].Details); err != nil {
						log.Debug("failed to cache PR", "id", notifications[i].ID, "error", err)
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
		log.Warn("some PRs failed to enrich", "count", errors)
	}

	return int(cacheHits), nil
}

// EnrichAuthoredPR fetches additional PR details like review state, mergeable status
func (c *Client) EnrichAuthoredPR(n *Notification) error {
	if n.Details == nil || !n.Details.IsPR {
		return nil
	}

	parts := splitRepoURL(n.URL)
	if len(parts) < 2 {
		// Try parsing from Repository.FullName
		if n.Repository.FullName != "" {
			parts = splitBySlash(n.Repository.FullName)
		}
	}
	if len(parts) < 2 {
		return fmt.Errorf("could not determine owner/repo")
	}

	owner := parts[0]
	repo := parts[1]
	number := n.Details.Number

	// Fetch PR details, review state, and comments concurrently
	var pr *github.PullRequest
	var prErr error
	var reviewState string
	var comments []*github.PullRequestComment
	var commentsErr error

	var wg sync.WaitGroup
	wg.Add(3)

	// Fetch full PR details
	go func() {
		defer wg.Done()
		pr, _, prErr = c.client.PullRequests.Get(c.ctx, owner, repo, number)
	}()

	// Get review state
	go func() {
		defer wg.Done()
		reviewState = c.getPRReviewState(owner, repo, number)
	}()

	// Get review comments count
	go func() {
		defer wg.Done()
		comments, _, commentsErr = c.client.PullRequests.ListComments(c.ctx, owner, repo, number, &github.PullRequestListCommentsOptions{
			ListOptions: github.ListOptions{PerPage: 100},
		})
	}()

	wg.Wait()

	if prErr != nil {
		return fmt.Errorf("failed to get PR details: %w", prErr)
	}

	// Update details
	n.Details.Additions = pr.GetAdditions()
	n.Details.Deletions = pr.GetDeletions()
	n.Details.ChangedFiles = pr.GetChangedFiles()
	n.Details.Draft = pr.GetDraft()

	// Mergeable state
	if pr.Mergeable != nil {
		n.Details.Mergeable = pr.GetMergeable()
	}

	// Set review state (fetched concurrently)
	n.Details.ReviewState = reviewState

	// Set review comments count (fetched concurrently)
	if commentsErr == nil {
		n.Details.ReviewComments = len(comments)
	}

	return nil
}
