// Package ghclient provides GitHub API client functionality.
package ghclient

import (
	"time"

	"github.com/spiffcs/triage/internal/model"
)

// GitHubFetcher defines the interface for raw GitHub API operations.
// This interface provides direct access to GitHub's REST and GraphQL APIs
// without any caching logic. Use ItemStore for cache-aware fetching.
type GitHubFetcher interface {
	// Authentication
	GetAuthenticatedUser() (string, error)

	// Notifications
	ListUnreadNotifications(since time.Time) ([]model.Item, error)

	// Search operations
	ListReviewRequestedPRs(username string) ([]model.Item, error)
	ListAuthoredPRs(username string) ([]model.Item, error)
	ListAssignedIssues(username string) ([]model.Item, error)

	// Orphaned contributions
	ListOrphanedContributions(opts OrphanedSearchOptions) ([]model.Item, error)

	// GraphQL enrichment (used by Enricher)
	EnrichItemsGraphQL(items []model.Item, token string, onProgress func(completed, total int)) (int, error)

	// Token access (needed for GraphQL operations)
	Token() string
}

// Ensure Client implements GitHubFetcher interface.
var _ GitHubFetcher = (*Client)(nil)
