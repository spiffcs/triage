// Package github provides GitHub API client functionality.
package github

import "time"

// GitHubClient defines the interface for GitHub API operations.
// This interface enables mocking the GitHub client in unit tests.
type GitHubClient interface {
	// Authentication
	GetAuthenticatedUser() (string, error)

	// Notifications
	ListUnreadNotifications(since time.Time) ([]Notification, error)

	// Search operations
	ListReviewRequestedPRs(username string) ([]Notification, error)
	ListAuthoredPRs(username string) ([]Notification, error)
	ListAssignedIssues(username string) ([]Notification, error)

	// Cached operations
	ListReviewRequestedPRsCached(username string, cache *Cache) ([]Notification, bool, error)
	ListAuthoredPRsCached(username string, cache *Cache) ([]Notification, bool, error)
	ListAssignedIssuesCached(username string, cache *Cache) ([]Notification, bool, error)
	ListUnreadNotificationsCached(username string, since time.Time, cache *Cache) (*NotificationFetchResult, error)

	// Enrichment
	EnrichNotificationsConcurrent(notifications []Notification, workers int, onProgress func(completed, total int)) (int, error)
	EnrichPRsConcurrent(notifications []Notification, workers int, cache *Cache, onProgress func(completed, total int)) (int, error)
}

// Cacher defines the interface for caching operations.
// This interface enables mocking the cache in unit tests.
type Cacher interface {
	// Item details cache
	Get(n *Notification) (*ItemDetails, bool)
	Set(n *Notification, details *ItemDetails) error
	Clear() error

	// PR list cache
	GetPRList(username, listType string) ([]Notification, bool)
	SetPRList(username, listType string, prs []Notification) error

	// Notification list cache
	GetNotificationList(username string, sinceTime time.Time) ([]Notification, time.Time, bool)
	SetNotificationList(username string, notifications []Notification, sinceTime time.Time) error

	// Stats
	Stats() (total int, validCount int, err error)
	DetailedStats() (*CacheStats, error)
}

// Ensure Client implements GitHubClient interface.
var _ GitHubClient = (*Client)(nil)

// Ensure Cache implements Cacher interface.
var _ Cacher = (*Cache)(nil)
