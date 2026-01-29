package ghclient

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	gh "github.com/google/go-github/v57/github"
	"github.com/spiffcs/triage/internal/constants"
	"github.com/spiffcs/triage/internal/log"
	"github.com/spiffcs/triage/internal/model"
	"golang.org/x/oauth2"
)

// rateLimitTransport wraps an http.RoundTripper to handle GitHub rate limits
type rateLimitTransport struct {
	base http.RoundTripper
}

func (t *rateLimitTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Check if we're already rate limited before making the request
	if globalRateLimitState.IsLimited() {
		return nil, ErrRateLimited
	}

	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return resp, err
	}

	// Parse and update rate limit state from response headers
	remaining, limit, resetAt := parseRateLimitHeaders(resp)
	if remaining >= 0 && limit > 0 {
		globalRateLimitState.Update(remaining, limit, resetAt)
	}

	// Log warning if rate limit is low
	if remaining <= constants.RateLimitLowWatermark && remaining > 0 {
		log.Debug("rate limit low", "remaining", remaining, "resets_at", resetAt.Format(time.RFC3339))
	}

	// Handle rate limit responses (403 with rate limit exceeded or 429)
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		if resp.Header.Get("X-RateLimit-Remaining") == "0" || resp.StatusCode == http.StatusTooManyRequests {
			// Set global rate limit state
			globalRateLimitState.SetLimited(true, resetAt)
			_ = resp.Body.Close()
			return nil, ErrRateLimited
		}
	}

	return resp, err
}

// parseRateLimitHeaders extracts rate limit info from response headers.
func parseRateLimitHeaders(resp *http.Response) (remaining, limit int, resetAt time.Time) {
	remaining = -1
	limit = -1

	if remainingStr := resp.Header.Get("X-RateLimit-Remaining"); remainingStr != "" {
		if rem, err := strconv.Atoi(remainingStr); err == nil {
			remaining = rem
		}
	}

	if limitStr := resp.Header.Get("X-RateLimit-Limit"); limitStr != "" {
		if lim, err := strconv.Atoi(limitStr); err == nil {
			limit = lim
		}
	}

	if resetStr := resp.Header.Get("X-RateLimit-Reset"); resetStr != "" {
		if resetTime, err := strconv.ParseInt(resetStr, 10, 64); err == nil {
			resetAt = time.Unix(resetTime, 0)
		}
	}

	return remaining, limit, resetAt
}

// Client wraps the GitHub API client
type Client struct {
	client *gh.Client
	ctx    context.Context
	token  string
}

// NewClient creates a new GitHub client using a personal access token.
// The context is used for all API requests and enables graceful cancellation.
func NewClient(ctx context.Context, token string) (*Client, error) {
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	if token == "" {
		return nil, fmt.Errorf("GitHub token not provided. Set the GITHUB_TOKEN environment variable")
	}

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)

	// Wrap transport with rate limit handling
	tc.Transport = &rateLimitTransport{
		base: tc.Transport,
	}

	client := gh.NewClient(tc)

	return &Client{
		client: client,
		ctx:    ctx,
		token:  token,
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
func (c *Client) RawClient() *gh.Client {
	return c.client
}

// Context returns the client's context
func (c *Client) Context() context.Context {
	return c.ctx
}

// Token returns the GitHub token for GraphQL API calls
func (c *Client) Token() string {
	return c.token
}

// ListReviewRequestedPRs fetches open PRs where the user is a requested reviewer
func (c *Client) ListReviewRequestedPRs(username string) ([]model.Item, error) {
	query := fmt.Sprintf("is:pr is:open review-requested:%s", username)

	opts := &gh.SearchOptions{
		Sort:  "updated",
		Order: "desc",
		ListOptions: gh.ListOptions{
			PerPage: 100,
		},
	}

	var notifications []model.Item

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

// prToNotification converts a PR from search results to a model.Item
func (c *Client) prToNotification(issue *gh.Issue) model.Item {
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

	notification := model.Item{
		ID:        fmt.Sprintf("review-requested-%d", issue.GetID()),
		Reason:    model.ReasonReviewRequested,
		Unread:    true, // Treat as unread since review is pending
		UpdatedAt: issue.GetUpdatedAt().Time,
		Repository: model.Repository{
			Name:     repoName,
			FullName: fullName,
			HTMLURL:  fmt.Sprintf("https://github.com/%s", fullName),
		},
		Subject: model.Subject{
			Title: issue.GetTitle(),
			URL:   issue.GetURL(),
			Type:  model.SubjectPullRequest,
		},
		URL: issue.GetURL(),
		Details: &model.ItemDetails{
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
func (c *Client) ListAssignedIssues(username string) ([]model.Item, error) {
	query := fmt.Sprintf("is:issue is:open assignee:%s", username)

	opts := &gh.SearchOptions{
		Sort:  "updated",
		Order: "desc",
		ListOptions: gh.ListOptions{
			PerPage: 100,
		},
	}

	var notifications []model.Item

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

// assignedIssueToNotification converts an assigned issue to a model.Item
func (c *Client) assignedIssueToNotification(issue *gh.Issue) model.Item {
	repoURL := issue.GetRepositoryURL()
	parts := splitRepoURL(repoURL)
	fullName := ""
	repoName := ""
	if len(parts) >= 2 {
		fullName = parts[0] + "/" + parts[1]
		repoName = parts[1]
	}

	notification := model.Item{
		ID:        fmt.Sprintf("assigned-%d", issue.GetID()),
		Reason:    model.ReasonAssign,
		Unread:    true,
		UpdatedAt: issue.GetUpdatedAt().Time,
		Repository: model.Repository{
			Name:     repoName,
			FullName: fullName,
			HTMLURL:  fmt.Sprintf("https://github.com/%s", fullName),
		},
		Subject: model.Subject{
			Title: issue.GetTitle(),
			URL:   issue.GetURL(),
			Type:  model.SubjectIssue,
		},
		URL: issue.GetURL(),
		Details: &model.ItemDetails{
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
func (c *Client) ListAuthoredPRs(username string) ([]model.Item, error) {
	query := fmt.Sprintf("is:pr is:open author:%s", username)

	opts := &gh.SearchOptions{
		Sort:  "updated",
		Order: "desc",
		ListOptions: gh.ListOptions{
			PerPage: 100,
		},
	}

	var notifications []model.Item

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

// authoredPRToNotification converts an authored PR to a model.Item
func (c *Client) authoredPRToNotification(issue *gh.Issue) model.Item {
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

	notification := model.Item{
		ID:        fmt.Sprintf("authored-%d", issue.GetID()),
		Reason:    model.ReasonAuthor,
		Unread:    true,
		UpdatedAt: issue.GetUpdatedAt().Time,
		Repository: model.Repository{
			Name:     repoName,
			FullName: fullName,
			HTMLURL:  fmt.Sprintf("https://github.com/%s", fullName),
		},
		Subject: model.Subject{
			Title: issue.GetTitle(),
			URL:   issue.GetURL(),
			Type:  model.SubjectPullRequest,
		},
		URL: issue.GetURL(),
		Details: &model.ItemDetails{
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
func (c *Client) ListReviewRequestedPRsCached(username string, cache *Cache) ([]model.Item, bool, error) {
	// Check cache first
	if cache != nil {
		if prs, ok := cache.GetPRList(username, "review-requested"); ok {
			return prs, true, nil
		}
	}

	// Check if rate limited
	if globalRateLimitState.IsLimited() {
		return nil, false, ErrRateLimited
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
func (c *Client) ListAuthoredPRsCached(username string, cache *Cache) ([]model.Item, bool, error) {
	// Check cache first
	if cache != nil {
		if prs, ok := cache.GetPRList(username, "authored"); ok {
			return prs, true, nil
		}
	}

	// Check if rate limited
	if globalRateLimitState.IsLimited() {
		return nil, false, ErrRateLimited
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
func (c *Client) ListAssignedIssuesCached(username string, cache *Cache) ([]model.Item, bool, error) {
	// Check cache first
	if cache != nil {
		if issues, ok := cache.GetPRList(username, "assigned-issues"); ok {
			return issues, true, nil
		}
	}

	// Check if rate limited
	if globalRateLimitState.IsLimited() {
		return nil, false, ErrRateLimited
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
	Notifications []model.Item
	FromCache     bool
	NewCount      int // Number of new notifications fetched (for incremental updates)
}

// ListUnreadNotificationsCached fetches notifications with incremental caching.
// It returns cached notifications merged with any new ones since the last fetch.
func (c *Client) ListUnreadNotificationsCached(username string, since time.Time, cache *Cache) (*NotificationFetchResult, error) {
	result := &NotificationFetchResult{}

	// Check if rate limited - return cached data if available
	if globalRateLimitState.IsLimited() {
		if cache != nil {
			if cached, _, ok := cache.GetNotificationList(username, since); ok {
				result.Notifications = cached
				result.FromCache = true
				return result, nil
			}
		}
		return nil, ErrRateLimited
	}

	// Check cache
	if cache != nil {
		cached, lastFetch, ok := cache.GetNotificationList(username, since)
		if ok {
			// Fetch only NEW notifications since last fetch
			newNotifs, err := c.ListUnreadNotifications(lastFetch)
			if err != nil {
				// Return cached on error
				log.Debug("failed to fetch new notifications, using cache", "error", err)
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
func mergeNotifications(cached, fresh []model.Item, since time.Time) []model.Item {
	byID := make(map[string]model.Item)

	// Add cached notifications
	for _, n := range cached {
		byID[n.ID] = n
	}

	// New overwrites old
	for _, n := range fresh {
		byID[n.ID] = n
	}

	// Build result, filtering appropriately
	result := make([]model.Item, 0, len(byID))
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

// EnrichPRsConcurrent enriches PRs (review-requested or authored) using GraphQL batch queries with caching.
// Uses GraphQL API (separate quota from Core API) for efficient batch enrichment.
// Returns the number of cache hits.
func (c *Client) EnrichPRsConcurrent(notifications []model.Item, workers int, cache *Cache, onProgress func(completed, total int)) (int, error) {
	total := len(notifications)
	if total == 0 {
		return 0, nil
	}

	var cacheHits int64

	// First pass: check cache and build list of items needing enrichment
	uncachedNotifications := make([]model.Item, 0, total)
	uncachedIndices := make([]int, 0, total)

	for i := range notifications {
		if cache != nil {
			if details, ok := cache.Get(&notifications[i]); ok {
				notifications[i].Details = details
				cacheHits++
				if onProgress != nil {
					onProgress(1, total) // Report 1 item completed from cache
				}
				continue
			}
		}
		uncachedNotifications = append(uncachedNotifications, notifications[i])
		uncachedIndices = append(uncachedIndices, i)
	}

	if len(uncachedNotifications) == 0 {
		return int(cacheHits), nil
	}

	// Use GraphQL for batch enrichment (uses GraphQL quota, not Core API)
	log.Debug("enriching PRs via GraphQL", "count", len(uncachedNotifications))

	progressOffset := int(cacheHits)
	enriched, err := c.EnrichNotificationsGraphQL(uncachedNotifications, c.token, func(completed, batchTotal int) {
		if onProgress != nil {
			onProgress(progressOffset+completed, total)
		}
	})

	if err != nil {
		log.Debug("GraphQL PR enrichment error", "error", err)
	}

	// Copy enriched data back to original slice and cache results
	for i, origIdx := range uncachedIndices {
		notifications[origIdx] = uncachedNotifications[i]
		// Log what we're copying back
		n := &notifications[origIdx]
		if n.Details != nil {
			log.Debug("enriched PR",
				"id", n.ID,
				"repo", n.Repository.FullName,
				"additions", n.Details.Additions,
				"deletions", n.Details.Deletions,
				"reviewState", n.Details.ReviewState,
				"ciStatus", n.Details.CIStatus)
		} else {
			log.Debug("PR not enriched - Details is nil", "id", n.ID, "repo", n.Repository.FullName)
		}
		// Cache successful enrichment
		if cache != nil && uncachedNotifications[i].Details != nil {
			if err := cache.Set(&notifications[origIdx], notifications[origIdx].Details); err != nil {
				log.Debug("failed to cache PR", "id", notifications[origIdx].ID, "error", err)
			}
		}
	}

	log.Debug("GraphQL PR enrichment complete", "enriched", enriched, "total", len(uncachedNotifications))

	return int(cacheHits), nil
}

// EnrichAuthoredPR fetches additional PR details like review state, mergeable status
func (c *Client) EnrichAuthoredPR(n *model.Item) error {
	if n.Details == nil || !n.Details.IsPR {
		return nil
	}

	parts := splitRepoURL(n.URL)
	if len(parts) < 2 {
		// Try parsing from model.Repository.FullName
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
	var pr *gh.PullRequest
	var prErr error
	var reviewState string
	var comments []*gh.PullRequestComment
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
		comments, _, commentsErr = c.client.PullRequests.ListComments(c.ctx, owner, repo, number, &gh.PullRequestListCommentsOptions{
			ListOptions: gh.ListOptions{PerPage: 100},
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
