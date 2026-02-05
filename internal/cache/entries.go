package cache

import (
	"time"

	"github.com/spiffcs/triage/internal/model"
)

// Version should be incremented when the cache format changes
// or when enrichment data structure changes to invalidate old entries
const Version = 3

// ListType identifies the source of a list of items
type ListType string

const (
	ListTypeNotifications   ListType = "notifications"
	ListTypeReviewRequested ListType = "review-requested"
	ListTypeAuthored        ListType = "authored"
	ListTypeAssignedIssues  ListType = "assigned-issues"
	ListTypeAssignedPRs     ListType = "assigned-prs"
	ListTypeOrphaned        ListType = "orphaned"
)

// AllListTypes returns all defined list types for iteration
func AllListTypes() []ListType {
	return []ListType{
		ListTypeNotifications,
		ListTypeReviewRequested,
		ListTypeAuthored,
		ListTypeAssignedIssues,
		ListTypeAssignedPRs,
		ListTypeOrphaned,
	}
}

// ListOptions contains optional parameters for list cache operations
type ListOptions struct {
	SinceTime time.Time // For notifications/orphaned
	Repos     []string  // For orphaned validation
}

// ListCacheEntry stores a cached list of items with context
type ListCacheEntry struct {
	Items         []model.Item `json:"items"`
	CachedAt      time.Time    `json:"cachedAt"`
	LastFetchTime time.Time    `json:"lastFetchTime"`   // For incremental updates
	SinceTime     time.Time    `json:"sinceTime"`       // Time constraint used
	Repos         []string     `json:"repos,omitempty"` // For orphaned validation
	Version       int          `json:"version"`
}

// DetailsCacheEntry caches enrichment data for an item (PR or Issue)
// Note: Details is stored as raw JSON to handle the polymorphic Details interface
type DetailsCacheEntry struct {
	Item      model.Item `json:"item"` // Full item with promoted fields and Details
	CachedAt  time.Time  `json:"cachedAt"`
	UpdatedAt time.Time  `json:"updatedAt"` // model.Item's UpdatedAt for invalidation
	Version   int        `json:"version"`   // Cache version for invalidation
}

// CacheStats contains detailed cache statistics
type CacheStats struct {
	// Details cache (enrichment data for individual items)
	DetailTotal int
	DetailValid int

	// List caches by type
	ListStats map[ListType]ListStats
}

// ListStats contains statistics for a single list type
type ListStats struct {
	Total int
	Valid int
	Items int // number of items within the list cache entry
}
