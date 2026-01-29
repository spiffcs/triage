// Package ghclient provides GitHub API client functionality.
package ghclient

import (
	"context"
	"time"

	"github.com/spiffcs/triage/internal/model"
)

// GitHubFetcher defines the interface for raw GitHub API operations.
// This interface provides direct access to GitHub's REST and GraphQL APIs
// without any caching logic. Use ItemStore for cache-aware fetching.
type GitHubFetcher interface {
	// Authentication
	GetAuthenticatedUser(ctx context.Context) (string, error)

	// Notifications
	ListUnreadNotifications(ctx context.Context, since time.Time) ([]model.Item, error)

	// Search operations
	ListReviewRequestedPRs(ctx context.Context, username string) ([]model.Item, error)
	ListAuthoredPRs(ctx context.Context, username string) ([]model.Item, error)
	ListAssignedIssues(ctx context.Context, username string) ([]model.Item, error)

	// Orphaned contributions
	ListOrphanedContributions(ctx context.Context, opts OrphanedSearchOptions) ([]model.Item, error)

	// GraphQL enrichment (used by Enricher)
	EnrichItemsGraphQL(ctx context.Context, items []model.Item, token string, onProgress func(completed, total int)) (int, error)

	// Token access (needed for GraphQL operations)
	Token() string
}

// Ensure Client implements GitHubFetcher interface.
var _ GitHubFetcher = (*Client)(nil)
