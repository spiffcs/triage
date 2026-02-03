// Package cache provides caching functionality for GitHub API responses.
package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spiffcs/triage/internal/constants"
	"github.com/spiffcs/triage/internal/log"
	"github.com/spiffcs/triage/internal/model"
)

// CacheKey uniquely identifies an item in the cache.
// The caller is responsible for extracting the number from the API URL.
type CacheKey struct {
	RepoFullName string
	SubjectType  model.SubjectType
	Number       int
}

// Cacher defines the interface for caching operations.
// This interface enables mocking the cache in unit tests.
type Cacher interface {
	// Item details cache
	Get(key CacheKey, updatedAt time.Time) (*model.Item, bool)
	Set(key CacheKey, updatedAt time.Time, item *model.Item) error
	Clear() error

	// Unified list cache
	GetList(username string, listType ListType, opts ListOptions) (*ListCacheEntry, bool)
	SetList(username string, listType ListType, entry *ListCacheEntry) error

	// Stats
	Stats() (total int, validCount int, err error)
	DetailedStats() (*CacheStats, error)
}

// Ensure Cache implements Cacher interface.
var _ Cacher = (*Cache)(nil)

// Cache stores notification details to avoid repeated API calls
type Cache struct {
	dir string
}

// NewCache creates a new cache instance
func NewCache() (*Cache, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}

	cacheDir = filepath.Join(cacheDir, "triage", "details")
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	return &Cache{dir: cacheDir}, nil
}

// cacheKeyString generates a file name for a cache key
func (c *Cache) cacheKeyString(key CacheKey) string {
	// Replace slashes with underscores to avoid path issues while preserving uniqueness
	safeName := strings.ReplaceAll(key.RepoFullName, "/", "_")
	return fmt.Sprintf("%s_%s_%d.json",
		safeName,
		key.SubjectType,
		key.Number,
	)
}

// Get retrieves cached item data for an item.
// The caller provides the cache key and the item's updated time for invalidation.
func (c *Cache) Get(key CacheKey, updatedAt time.Time) (*model.Item, bool) {
	if key.Number == 0 {
		return nil, false
	}

	keyStr := c.cacheKeyString(key)
	path := filepath.Join(c.dir, keyStr)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}

	var entry DetailsCacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, false
	}

	// Invalidate if cache version doesn't match (format/schema changed)
	if entry.Version != Version {
		log.Debug("cache version mismatch", "cached", entry.Version, "current", Version, "key", keyStr)
		return nil, false
	}

	// Invalidate if item was updated after cache
	if updatedAt.After(entry.UpdatedAt) {
		return nil, false
	}

	// Also invalidate if cache is too old
	if time.Since(entry.CachedAt) > constants.DetailCacheTTL {
		return nil, false
	}

	return &entry.Item, true
}

// Set caches item data for an item.
// The caller provides the cache key and the item's updated time.
func (c *Cache) Set(key CacheKey, updatedAt time.Time, item *model.Item) error {
	if key.Number == 0 || item == nil {
		return nil
	}

	entry := DetailsCacheEntry{
		Item:      *item,
		CachedAt:  time.Now(),
		UpdatedAt: updatedAt,
		Version:   Version,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	keyStr := c.cacheKeyString(key)
	path := filepath.Join(c.dir, keyStr)

	return os.WriteFile(path, data, 0600)
}

// Clear removes all cached entries
func (c *Cache) Clear() error {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if err := os.Remove(filepath.Join(c.dir, entry.Name())); err != nil {
			return err
		}
	}

	return nil
}

// Stats returns cache statistics
func (c *Cache) Stats() (total int, validCount int, err error) {
	stats, err := c.DetailedStats()
	if err != nil {
		return 0, 0, err
	}
	totalCount := stats.DetailTotal
	validCount = stats.DetailValid
	for _, ls := range stats.ListStats {
		totalCount += ls.Total
		validCount += ls.Valid
	}
	return totalCount, validCount, nil
}

// ttlForListType returns the TTL for a given list type
func ttlForListType(listType ListType) time.Duration {
	switch listType {
	case ListTypeNotifications:
		return constants.NotificationsCacheTTL
	case ListTypeOrphaned:
		return 15 * time.Minute
	default:
		// review-requested, authored, assigned-issues
		return constants.ItemListCacheTTL
	}
}

// listCacheKey generates a cache key for a list
func (c *Cache) listCacheKey(username string, listType ListType) string {
	return fmt.Sprintf("list_%s_%s.json", listType, username)
}

// GetList retrieves a cached list.
// Returns the entry and true if found and valid, nil and false otherwise.
func (c *Cache) GetList(username string, listType ListType, opts ListOptions) (*ListCacheEntry, bool) {
	key := c.listCacheKey(username, listType)
	path := filepath.Join(c.dir, key)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}

	var entry ListCacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, false
	}

	// Check version
	if entry.Version != Version {
		return nil, false
	}

	// Check TTL
	ttl := ttlForListType(listType)
	if time.Since(entry.CachedAt) > ttl {
		return nil, false
	}

	// Type-specific validation
	switch listType {
	case ListTypeNotifications, ListTypeOrphaned:
		// Truncate both times to hour boundary for comparison
		// This avoids cache misses due to small time differences between runs
		cachedHour := entry.SinceTime.Truncate(time.Hour)
		requestedHour := opts.SinceTime.Truncate(time.Hour)
		// If user's since time is earlier than cached (by hour), require full refresh
		if requestedHour.Before(cachedHour) {
			return nil, false
		}
	}

	if listType == ListTypeOrphaned {
		// Invalidate if repos don't match
		if !stringSlicesEqual(opts.Repos, entry.Repos) {
			return nil, false
		}
	}

	return &entry, true
}

// SetList caches a list
func (c *Cache) SetList(username string, listType ListType, entry *ListCacheEntry) error {
	if entry == nil {
		return nil
	}

	// Ensure required fields are set
	if entry.CachedAt.IsZero() {
		entry.CachedAt = time.Now()
	}
	if entry.Version == 0 {
		entry.Version = Version
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	key := c.listCacheKey(username, listType)
	path := filepath.Join(c.dir, key)

	return os.WriteFile(path, data, 0600)
}

// DetailedStats returns detailed cache statistics broken down by type
func (c *Cache) DetailedStats() (*CacheStats, error) {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return nil, err
	}

	stats := &CacheStats{
		ListStats: make(map[ListType]ListStat),
	}

	// Initialize all list types
	for _, lt := range AllListTypes() {
		stats.ListStats[lt] = ListStat{}
	}

	now := time.Now()

	for _, entry := range entries {
		path := filepath.Join(c.dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		name := entry.Name()

		// Check if it's a list cache entry (starts with "list_")
		if strings.HasPrefix(name, "list_") {
			// Parse the list type from the filename: list_{type}_{username}.json
			rest := strings.TrimPrefix(name, "list_")
			// Find the list type by checking prefixes
			var matchedType ListType
			for _, lt := range AllListTypes() {
				prefix := string(lt) + "_"
				if strings.HasPrefix(rest, prefix) {
					matchedType = lt
					break
				}
			}

			if matchedType == "" {
				continue // Unknown list type
			}

			var listEntry ListCacheEntry
			if err := json.Unmarshal(data, &listEntry); err != nil {
				continue
			}

			ls := stats.ListStats[matchedType]
			ls.Total++
			ttl := ttlForListType(matchedType)
			if now.Sub(listEntry.CachedAt) <= ttl {
				ls.Valid++
			}
			stats.ListStats[matchedType] = ls
		} else {
			// Everything else is detail cache entries
			stats.DetailTotal++
			var cacheEntry DetailsCacheEntry
			if err := json.Unmarshal(data, &cacheEntry); err != nil {
				continue
			}
			if now.Sub(cacheEntry.CachedAt) <= constants.DetailCacheTTL {
				stats.DetailValid++
			}
		}
	}

	return stats, nil
}

// stringSlicesEqual checks if two string slices contain the same elements (order-insensitive)
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aMap := make(map[string]int, len(a))
	for _, s := range a {
		aMap[s]++
	}
	for _, s := range b {
		if aMap[s] == 0 {
			return false
		}
		aMap[s]--
	}
	return true
}
