package ghclient

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
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
	// token is intentionally unexported. NEVER add String(), MarshalJSON(),
	// or any method that could expose this value in logs or serialized output.
	token string
}

// NewClient creates a new GitHub client using a personal access token.
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
		token:  token,
	}, nil
}

// GetAuthenticatedUser returns the authenticated user's login
func (c *Client) GetAuthenticatedUser(ctx context.Context) (string, error) {
	user, _, err := c.client.Users.Get(ctx, "")
	if err != nil {
		return "", fmt.Errorf("failed to get authenticated user: %w", err)
	}
	return user.GetLogin(), nil
}

// RawClient returns the underlying go-github client for advanced operations
func (c *Client) RawClient() *gh.Client {
	return c.client
}

// Token returns the GitHub token for GraphQL API calls
func (c *Client) Token() string {
	return c.token
}

// ListReviewRequestedPRs fetches open PRs where the user is a requested reviewer
func (c *Client) ListReviewRequestedPRs(ctx context.Context, username string) ([]model.Item, error) {
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
		result, resp, err := c.client.Search.Issues(ctx, query, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to search for review-requested PRs: %w", err)
		}

		for _, issue := range result.Issues {
			// Convert search result to item format
			item := c.prToItem(issue)
			notifications = append(notifications, item)
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return notifications, nil
}

// prToItem converts a PR from search results to a model.Item
func (c *Client) prToItem(issue *gh.Issue) model.Item {
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

	// Extract labels
	var labels []string
	for _, label := range issue.Labels {
		labels = append(labels, label.GetName())
	}

	// Extract assignees
	var assignees []string
	for _, assignee := range issue.Assignees {
		assignees = append(assignees, assignee.GetLogin())
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
		// Promoted common fields
		Type:         model.ItemTypePullRequest,
		Number:       issue.GetNumber(),
		State:        issue.GetState(),
		HTMLURL:      issue.GetHTMLURL(),
		CreatedAt:    issue.GetCreatedAt().Time,
		Author:       issue.GetUser().GetLogin(),
		CommentCount: issue.GetComments(),
		Labels:       labels,
		Assignees:    assignees,
		// PR-specific details
		Details: &model.PRDetails{
			ReviewState: "pending",
		},
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
func (c *Client) ListAssignedIssues(ctx context.Context, username string) ([]model.Item, error) {
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
		result, resp, err := c.client.Search.Issues(ctx, query, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to search for assigned issues: %w", err)
		}

		for _, issue := range result.Issues {
			item := c.assignedIssueToItem(issue)
			notifications = append(notifications, item)
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return notifications, nil
}

// assignedIssueToItem converts an assigned issue to a model.Item
func (c *Client) assignedIssueToItem(issue *gh.Issue) model.Item {
	repoURL := issue.GetRepositoryURL()
	parts := splitRepoURL(repoURL)
	fullName := ""
	repoName := ""
	if len(parts) >= 2 {
		fullName = parts[0] + "/" + parts[1]
		repoName = parts[1]
	}

	// Extract labels
	var labels []string
	for _, label := range issue.Labels {
		labels = append(labels, label.GetName())
	}

	// Extract assignees
	var assignees []string
	for _, assignee := range issue.Assignees {
		assignees = append(assignees, assignee.GetLogin())
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
		// Promoted common fields
		Type:         model.ItemTypeIssue,
		Number:       issue.GetNumber(),
		State:        issue.GetState(),
		HTMLURL:      issue.GetHTMLURL(),
		CreatedAt:    issue.GetCreatedAt().Time,
		Author:       issue.GetUser().GetLogin(),
		CommentCount: issue.GetComments(),
		Labels:       labels,
		Assignees:    assignees,
		// Issue-specific details
		Details: &model.IssueDetails{},
	}

	return notification
}

// ListAssignedPRs fetches open PRs assigned to the user
func (c *Client) ListAssignedPRs(ctx context.Context, username string) ([]model.Item, error) {
	query := fmt.Sprintf("is:pr is:open assignee:%s", username)

	opts := &gh.SearchOptions{
		Sort:  "updated",
		Order: "desc",
		ListOptions: gh.ListOptions{
			PerPage: 100,
		},
	}

	var items []model.Item

	for {
		result, resp, err := c.client.Search.Issues(ctx, query, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to search for assigned PRs: %w", err)
		}

		for _, issue := range result.Issues {
			item := c.assignedPRToItem(issue)
			items = append(items, item)
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return items, nil
}

// assignedPRToItem converts an assigned PR to a model.Item
func (c *Client) assignedPRToItem(issue *gh.Issue) model.Item {
	repoURL := issue.GetRepositoryURL()
	parts := splitRepoURL(repoURL)
	fullName := ""
	repoName := ""
	if len(parts) >= 2 {
		fullName = parts[0] + "/" + parts[1]
		repoName = parts[1]
	}

	// Extract labels
	var labels []string
	for _, label := range issue.Labels {
		labels = append(labels, label.GetName())
	}

	// Extract assignees
	var assignees []string
	for _, assignee := range issue.Assignees {
		assignees = append(assignees, assignee.GetLogin())
	}

	return model.Item{
		ID:        fmt.Sprintf("assigned-pr-%d", issue.GetID()),
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
			Type:  model.SubjectPullRequest,
		},
		URL: issue.GetURL(),
		// Promoted common fields
		Type:         model.ItemTypePullRequest,
		Number:       issue.GetNumber(),
		State:        issue.GetState(),
		HTMLURL:      issue.GetHTMLURL(),
		CreatedAt:    issue.GetCreatedAt().Time,
		Author:       issue.GetUser().GetLogin(),
		Labels:       labels,
		Assignees:    assignees,
		CommentCount: issue.GetComments(),
		// PR-specific details
		Details: &model.PRDetails{},
	}
}

// ListAuthoredPRs fetches open PRs authored by the user
func (c *Client) ListAuthoredPRs(ctx context.Context, username string) ([]model.Item, error) {
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
		result, resp, err := c.client.Search.Issues(ctx, query, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to search for authored PRs: %w", err)
		}

		for _, issue := range result.Issues {
			item := c.authoredPRToItem(issue)
			notifications = append(notifications, item)
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return notifications, nil
}

// authoredPRToItem converts an authored PR to a model.Item
func (c *Client) authoredPRToItem(issue *gh.Issue) model.Item {
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

	// Extract labels
	var labels []string
	for _, label := range issue.Labels {
		labels = append(labels, label.GetName())
	}

	// Extract assignees
	var assignees []string
	for _, assignee := range issue.Assignees {
		assignees = append(assignees, assignee.GetLogin())
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
		// Promoted common fields
		Type:         model.ItemTypePullRequest,
		Number:       issue.GetNumber(),
		State:        issue.GetState(),
		HTMLURL:      issue.GetHTMLURL(),
		CreatedAt:    issue.GetCreatedAt().Time,
		Author:       issue.GetUser().GetLogin(),
		CommentCount: issue.GetComments(),
		Labels:       labels,
		Assignees:    assignees,
		// PR-specific details
		Details: &model.PRDetails{
			Draft: issue.GetDraft(),
		},
	}

	return notification
}
