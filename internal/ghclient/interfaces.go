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
	ListUnreadItems(since time.Time) ([]model.Item, error)

	// Search operations
	ListReviewRequestedPRs(username string) ([]model.Item, error)
	ListAuthoredPRs(username string) ([]model.Item, error)
	ListAssignedIssues(username string) ([]model.Item, error)

	// Orphaned contributions
	ListOrphanedContributions(opts model.OrphanedSearchOptions) ([]model.Item, error)

	// GraphQL enrichment (used by Enricher)
	EnrichItemsGraphQL(items []model.Item, token string, onProgress func(completed, total int)) (int, error)

	// Token access (needed for GraphQL operations)
	Token() string
}

// Cacher defines the interface for caching operations.
// This interface enables mocking the cache in unit tests.
type Cacher interface {
	// Item details cache
	Get(n *model.Item) (*model.ItemDetails, bool)
	Set(n *model.Item, details *model.ItemDetails) error
	Clear() error

	// PR list cache
	GetPRList(username, listType string) ([]model.Item, bool)
	SetPRList(username, listType string, prs []model.Item) error

	// Item list cache
	GetItemList(username string, sinceTime time.Time) ([]model.Item, time.Time, bool)
	SetItemList(username string, items []model.Item, sinceTime time.Time) error

	// Stats
	Stats() (total int, validCount int, err error)
	DetailedStats() (*CacheStats, error)
}

// Ensure Client implements GitHubFetcher interface.
var _ GitHubFetcher = (*Client)(nil)

// Ensure Cache implements Cacher interface.
var _ Cacher = (*Cache)(nil)
