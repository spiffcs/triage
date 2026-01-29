package cache

import (
	"time"

	"github.com/spiffcs/triage/internal/constants"
	"github.com/spiffcs/triage/internal/model"
)

// cacheVersion should be incremented when the cache format changes
// or when enrichment data structure changes to invalidate old entries
const cacheVersion = 2

// CacheEntry represents a cached notification with its details
type CacheEntry struct {
	Details   *model.ItemDetails `json:"details"`
	CachedAt  time.Time          `json:"cachedAt"`
	UpdatedAt time.Time          `json:"updatedAt"` // model.Item's UpdatedAt for invalidation
	Version   int                `json:"version"`   // Cache version for invalidation
}

// PRListCacheEntry represents a cached list of PRs
type PRListCacheEntry struct {
	PRs      []model.Item `json:"prs"`
	CachedAt time.Time    `json:"cachedAt"`
	Version  int          `json:"version"`
}

// PRListCacheTTL is shorter than details cache since PR lists change more frequently.
// Note: This is kept as a package-level constant for backward compatibility,
// but the canonical value is in the constants package.
const PRListCacheTTL = constants.PRListCacheTTL

// NotificationsCacheEntry stores cached notifications with fetch timestamp
type NotificationsCacheEntry struct {
	Items         []model.Item `json:"items"`
	LastFetchTime time.Time    `json:"lastFetchTime"` // When we last hit the API
	CachedAt      time.Time    `json:"cachedAt"`
	SinceTime     time.Time    `json:"sinceTime"` // The --since value used
	Version       int          `json:"version"`
}

// NotificationsCacheTTL is the max age before a full refresh is required.
// Note: This is kept as a package-level constant for backward compatibility,
// but the canonical value is in the constants package.
const NotificationsCacheTTL = constants.NotificationsCacheTTL

// OrphanedListCacheEntry stores cached orphaned contributions
type OrphanedListCacheEntry struct {
	Orphaned  []model.Item `json:"orphaned"`
	Repos     []string     `json:"repos"`
	CachedAt  time.Time    `json:"cachedAt"`
	SinceTime time.Time    `json:"sinceTime"`
	Version   int          `json:"version"`
}

// OrphanedListCacheTTL is shorter than notifications since orphaned data changes less frequently
// but we want relatively fresh results for proactive outreach
const OrphanedListCacheTTL = 15 * time.Minute

// CacheStats contains detailed cache statistics
type CacheStats struct {
	DetailTotal       int
	DetailValid       int
	PRListTotal       int
	PRListValid       int
	NotifListTotal    int
	NotifListValid    int
	OrphanedListTotal int
	OrphanedListValid int
}
