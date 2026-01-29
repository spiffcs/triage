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
	Get(key CacheKey, updatedAt time.Time) (*model.ItemDetails, bool)
	Set(key CacheKey, updatedAt time.Time, details *model.ItemDetails) error
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

// Get retrieves cached details for an item.
// The caller provides the cache key and the item's updated time for invalidation.
func (c *Cache) Get(key CacheKey, updatedAt time.Time) (*model.ItemDetails, bool) {
	if key.Number == 0 {
		return nil, false
	}

	keyStr := c.cacheKeyString(key)
	path := filepath.Join(c.dir, keyStr)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}

	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, false
	}

	// Invalidate if cache version doesn't match (format/schema changed)
	if entry.Version != cacheVersion {
		log.Debug("cache version mismatch", "cached", entry.Version, "current", cacheVersion, "key", keyStr)
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

	return entry.Details, true
}

// Set caches details for an item.
// The caller provides the cache key and the item's updated time.
func (c *Cache) Set(key CacheKey, updatedAt time.Time, details *model.ItemDetails) error {
	if key.Number == 0 || details == nil {
		return nil
	}

	entry := CacheEntry{
		Details:   details,
		CachedAt:  time.Now(),
		UpdatedAt: updatedAt,
		Version:   cacheVersion,
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
	totalCount := stats.DetailTotal + stats.PRListTotal + stats.NotifListTotal + stats.OrphanedListTotal
	validCount = stats.DetailValid + stats.PRListValid + stats.NotifListValid + stats.OrphanedListValid
	return totalCount, validCount, nil
}

// DetailedStats returns detailed cache statistics broken down by type
func (c *Cache) DetailedStats() (*CacheStats, error) {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return nil, err
	}

	stats := &CacheStats{}
	now := time.Now()

	for _, entry := range entries {
		path := filepath.Join(c.dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		name := entry.Name()

		// Check if it's an item list cache entry (starts with "notif_list_")
		if len(name) > 11 && name[:11] == "notif_list_" {
			stats.NotifListTotal++
			var notifEntry NotificationsCacheEntry
			if err := json.Unmarshal(data, &notifEntry); err != nil {
				continue
			}
			if now.Sub(notifEntry.CachedAt) <= NotificationsCacheTTL {
				stats.NotifListValid++
			}
		} else if len(name) > 14 && name[:14] == "orphaned_list_" {
			// Check if it's an orphaned list cache entry (starts with "orphaned_list_")
			stats.OrphanedListTotal++
			var orphanedEntry OrphanedListCacheEntry
			if err := json.Unmarshal(data, &orphanedEntry); err != nil {
				continue
			}
			if now.Sub(orphanedEntry.CachedAt) <= OrphanedListCacheTTL {
				stats.OrphanedListValid++
			}
		} else if len(name) > 7 && name[:7] == "prlist_" {
			// Check if it's a PR list cache entry (starts with "prlist_")
			stats.PRListTotal++
			var prEntry PRListCacheEntry
			if err := json.Unmarshal(data, &prEntry); err != nil {
				continue
			}
			if now.Sub(prEntry.CachedAt) <= PRListCacheTTL {
				stats.PRListValid++
			}
		} else {
			stats.DetailTotal++
			var cacheEntry CacheEntry
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

// prListCacheKey generates a cache key for a PR list
func (c *Cache) prListCacheKey(username string, listType string) string {
	return fmt.Sprintf("prlist_%s_%s.json", listType, username)
}

// GetPRList retrieves cached PR list
func (c *Cache) GetPRList(username string, listType string) ([]model.Item, bool) {
	key := c.prListCacheKey(username, listType)
	path := filepath.Join(c.dir, key)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}

	var entry PRListCacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, false
	}

	// Check version (invalidate old cache format)
	if entry.Version != cacheVersion {
		return nil, false
	}

	// Check TTL
	if time.Since(entry.CachedAt) > PRListCacheTTL {
		return nil, false
	}

	return entry.PRs, true
}

// SetPRList caches a PR list
func (c *Cache) SetPRList(username string, listType string, prs []model.Item) error {
	entry := PRListCacheEntry{
		PRs:      prs,
		CachedAt: time.Now(),
		Version:  cacheVersion,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	key := c.prListCacheKey(username, listType)
	path := filepath.Join(c.dir, key)

	return os.WriteFile(path, data, 0600)
}

// itemListCacheKey generates a cache key for an item list
func (c *Cache) itemListCacheKey(username string) string {
	return fmt.Sprintf("notif_list_%s.json", username)
}

// GetItemList retrieves cached item list.
// Returns: cached items, last fetch time, ok
func (c *Cache) GetItemList(username string, sinceTime time.Time) ([]model.Item, time.Time, bool) {
	key := c.itemListCacheKey(username)
	path := filepath.Join(c.dir, key)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, time.Time{}, false
	}

	var entry NotificationsCacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, time.Time{}, false
	}

	// Check version (invalidate old cache format)
	if entry.Version != cacheVersion {
		return nil, time.Time{}, false
	}

	// Check TTL - if cache is too old, require full refresh
	if time.Since(entry.CachedAt) > NotificationsCacheTTL {
		return nil, time.Time{}, false
	}

	// If user's since time is earlier than what we cached, require full refresh
	// (they want more history than we have)
	if sinceTime.Before(entry.SinceTime) {
		return nil, time.Time{}, false
	}

	return entry.Items, entry.LastFetchTime, true
}

// SetItemList caches an item list
func (c *Cache) SetItemList(username string, items []model.Item, sinceTime time.Time) error {
	entry := NotificationsCacheEntry{
		Items:         items,
		LastFetchTime: time.Now(),
		CachedAt:      time.Now(),
		SinceTime:     sinceTime,
		Version:       cacheVersion,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	key := c.itemListCacheKey(username)
	path := filepath.Join(c.dir, key)

	return os.WriteFile(path, data, 0600)
}

// orphanedListCacheKey generates a cache key for orphaned contributions
func (c *Cache) orphanedListCacheKey(username string) string {
	return fmt.Sprintf("orphaned_list_%s.json", username)
}

// GetOrphanedList retrieves cached orphaned contributions.
// The cache is invalidated if repos don't match or sinceTime has changed significantly.
func (c *Cache) GetOrphanedList(username string, repos []string, sinceTime time.Time) ([]model.Item, bool) {
	key := c.orphanedListCacheKey(username)
	path := filepath.Join(c.dir, key)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}

	var entry OrphanedListCacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, false
	}

	// Check version
	if entry.Version != cacheVersion {
		return nil, false
	}

	// Check TTL
	if time.Since(entry.CachedAt) > OrphanedListCacheTTL {
		return nil, false
	}

	// Invalidate if repos don't match (order-insensitive comparison)
	if !stringSlicesEqual(entry.Repos, repos) {
		return nil, false
	}

	// Invalidate if user's since time is earlier than cached
	if sinceTime.Before(entry.SinceTime) {
		return nil, false
	}

	return entry.Orphaned, true
}

// SetOrphanedList caches orphaned contributions
func (c *Cache) SetOrphanedList(username string, orphaned []model.Item, repos []string, sinceTime time.Time) error {
	entry := OrphanedListCacheEntry{
		Orphaned:  orphaned,
		Repos:     repos,
		CachedAt:  time.Now(),
		SinceTime: sinceTime,
		Version:   cacheVersion,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	key := c.orphanedListCacheKey(username)
	path := filepath.Join(c.dir, key)

	return os.WriteFile(path, data, 0600)
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
