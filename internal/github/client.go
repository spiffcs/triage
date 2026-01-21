package github

import (
	"context"
	"fmt"
	"os"

	"github.com/google/go-github/v57/github"
	"golang.org/x/oauth2"
)

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
		return nil, fmt.Errorf("GitHub token not provided. Set GITHUB_TOKEN env var or use 'priority config set token <TOKEN>'")
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)
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
			HTMLURL:  issue.GetHTMLURL(),
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
		_ = cache.SetPRList(username, "review-requested", prs)
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
		_ = cache.SetPRList(username, "authored", prs)
	}

	return prs, false, nil
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

	// Fetch full PR details
	pr, _, err := c.client.PullRequests.Get(c.ctx, owner, repo, number)
	if err != nil {
		return fmt.Errorf("failed to get PR details: %w", err)
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

	// Get review state
	n.Details.ReviewState = c.getPRReviewState(owner, repo, number)

	// Get review comments count
	comments, _, err := c.client.PullRequests.ListComments(c.ctx, owner, repo, number, &github.PullRequestListCommentsOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	})
	if err == nil {
		n.Details.ReviewComments = len(comments)
	}

	return nil
}
