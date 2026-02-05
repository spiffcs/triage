package ghclient

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	gh "github.com/google/go-github/v57/github"
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
	if remaining <= RateLimitLowWatermark && remaining > 0 {
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

// AuthenticatedUser returns the authenticated user's login
func (c *Client) AuthenticatedUser(ctx context.Context) (string, error) {
	user, _, err := c.client.Users.Get(ctx, "")
	if err != nil {
		return "", fmt.Errorf("failed to get authenticated user: %w", err)
	}
	return user.GetLogin(), nil
}

// RateLimits fetches the current GitHub API rate limit status.
func (c *Client) RateLimits(ctx context.Context) (*gh.RateLimits, error) {
	limits, _, err := c.client.RateLimit.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get rate limits: %w", err)
	}
	return limits, nil
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

	var items []model.Item

	for {
		result, resp, err := c.client.Search.Issues(ctx, query, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to search for review-requested PRs: %w", err)
		}

		for _, issue := range result.Issues {
			items = append(items, issueToItem(issue,
				fmt.Sprintf("review-requested-%d", issue.GetID()),
				model.ReasonReviewRequested,
				model.SubjectPullRequest,
				&model.PRDetails{ReviewState: "pending"},
			))
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return items, nil
}

// repoFromURL extracts owner and repo name from a GitHub API repository URL.
// URL format: https://api.github.com/repos/owner/repo
func repoFromURL(url string) (owner, repo string) {
	const prefix = "https://api.github.com/repos/"
	trimmed := strings.TrimPrefix(url, prefix)
	if trimmed == url || trimmed == "" {
		return "", ""
	}
	parts := strings.SplitN(trimmed, "/", 3)
	if len(parts) < 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

// issueToItem converts a GitHub search result issue to a model.Item.
// This is the shared conversion logic used by all search-based item constructors.
func issueToItem(issue *gh.Issue, id string, reason model.ItemReason, subjectType model.SubjectType, details model.Details) model.Item {
	owner, repo := repoFromURL(issue.GetRepositoryURL())
	fullName := ""
	if owner != "" && repo != "" {
		fullName = owner + "/" + repo
	}

	var labels []string
	for _, label := range issue.Labels {
		labels = append(labels, label.GetName())
	}

	var assignees []string
	for _, assignee := range issue.Assignees {
		assignees = append(assignees, assignee.GetLogin())
	}

	itemType := model.ItemTypeIssue
	if subjectType == model.SubjectPullRequest {
		itemType = model.ItemTypePullRequest
	}

	return model.Item{
		ID:        id,
		Reason:    reason,
		Unread:    true,
		UpdatedAt: issue.GetUpdatedAt().Time,
		Repository: model.Repository{
			Name:     repo,
			FullName: fullName,
			HTMLURL:  fmt.Sprintf("https://github.com/%s", fullName),
		},
		Subject: model.Subject{
			Title: issue.GetTitle(),
			URL:   issue.GetURL(),
			Type:  subjectType,
		},
		URL:          issue.GetURL(),
		Type:         itemType,
		Number:       issue.GetNumber(),
		State:        issue.GetState(),
		HTMLURL:      issue.GetHTMLURL(),
		CreatedAt:    issue.GetCreatedAt().Time,
		Author:       issue.GetUser().GetLogin(),
		CommentCount: issue.GetComments(),
		Labels:       labels,
		Assignees:    assignees,
		Details:      details,
	}
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

	var items []model.Item

	for {
		result, resp, err := c.client.Search.Issues(ctx, query, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to search for assigned issues: %w", err)
		}

		for _, issue := range result.Issues {
			items = append(items, issueToItem(issue,
				fmt.Sprintf("assigned-%d", issue.GetID()),
				model.ReasonAssign,
				model.SubjectIssue,
				&model.IssueDetails{},
			))
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return items, nil
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
			items = append(items, issueToItem(issue,
				fmt.Sprintf("assigned-pr-%d", issue.GetID()),
				model.ReasonAssign,
				model.SubjectPullRequest,
				&model.PRDetails{},
			))
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return items, nil
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

	var items []model.Item

	for {
		result, resp, err := c.client.Search.Issues(ctx, query, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to search for authored PRs: %w", err)
		}

		for _, issue := range result.Issues {
			items = append(items, issueToItem(issue,
				fmt.Sprintf("authored-%d", issue.GetID()),
				model.ReasonAuthor,
				model.SubjectPullRequest,
				&model.PRDetails{Draft: issue.GetDraft()},
			))
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return items, nil
}
