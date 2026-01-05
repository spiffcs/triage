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
		return nil, fmt.Errorf("GitHub token not provided. Set GITHUB_TOKEN env var or use 'github-prio config set token <TOKEN>'")
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
