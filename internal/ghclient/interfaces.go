// Package ghclient provides GitHub API client functionality.
package ghclient

import (
	"time"

	"github.com/spiffcs/triage/internal/model"
)

// GitHubClient defines the interface for GitHub API operations.
// This interface enables mocking the GitHub client in unit tests.
type GitHubClient interface {
	// Authentication
	GetAuthenticatedUser() (string, error)

	// Notifications
	ListUnreadNotifications(since time.Time) ([]model.Item, error)

	// Search operations
	ListReviewRequestedPRs(username string) ([]model.Item, error)
	ListAuthoredPRs(username string) ([]model.Item, error)
	ListAssignedIssues(username string) ([]model.Item, error)

	// Cached operations
	ListReviewRequestedPRsCached(username string, cache *Cache) ([]model.Item, bool, error)
	ListAuthoredPRsCached(username string, cache *Cache) ([]model.Item, bool, error)
	ListAssignedIssuesCached(username string, cache *Cache) ([]model.Item, bool, error)
	ListUnreadNotificationsCached(username string, since time.Time, cache *Cache) (*NotificationFetchResult, error)

	// Enrichment
	EnrichNotificationsConcurrent(notifications []model.Item, workers int, onProgress func(completed, total int)) (int, error)
	EnrichPRsConcurrent(notifications []model.Item, workers int, cache *Cache, onProgress func(completed, total int)) (int, error)
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

	// Notification list cache
	GetNotificationList(username string, sinceTime time.Time) ([]model.Item, time.Time, bool)
	SetNotificationList(username string, notifications []model.Item, sinceTime time.Time) error

	// Stats
	Stats() (total int, validCount int, err error)
	DetailedStats() (*CacheStats, error)
}

// Ensure Client implements GitHubClient interface.
var _ GitHubClient = (*Client)(nil)

// Ensure Cache implements Cacher interface.
var _ Cacher = (*Cache)(nil)
