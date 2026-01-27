package github

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spiffcs/triage/internal/log"
)

// Cache stores notification details to avoid repeated API calls
type Cache struct {
	dir string
}

// cacheVersion should be incremented when the cache format changes
// or when enrichment data structure changes to invalidate old entries
const cacheVersion = 2

// CacheEntry represents a cached notification with its details
type CacheEntry struct {
	Details   *ItemDetails `json:"details"`
	CachedAt  time.Time    `json:"cachedAt"`
	UpdatedAt time.Time    `json:"updatedAt"` // Notification's UpdatedAt for invalidation
	Version   int          `json:"version"`   // Cache version for invalidation
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

// cacheKey generates a cache key for a notification
func (c *Cache) cacheKey(repoFullName string, subjectType SubjectType, number int) string {
	// Replace slashes with underscores to avoid path issues while preserving uniqueness
	safeName := strings.ReplaceAll(repoFullName, "/", "_")
	return fmt.Sprintf("%s_%s_%d.json",
		safeName,
		subjectType,
		number,
	)
}

// Get retrieves cached details for a notification
func (c *Cache) Get(n *Notification) (*ItemDetails, bool) {
	if n.Subject.URL == "" {
		return nil, false
	}

	number, err := ExtractIssueNumber(n.Subject.URL)
	if err != nil {
		return nil, false
	}

	key := c.cacheKey(n.Repository.FullName, n.Subject.Type, number)
	path := filepath.Join(c.dir, key)

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
		log.Debug("cache version mismatch", "cached", entry.Version, "current", cacheVersion, "key", key)
		return nil, false
	}

	// Invalidate if notification was updated after cache
	if n.UpdatedAt.After(entry.UpdatedAt) {
		return nil, false
	}

	// Also invalidate if cache is too old (24 hours)
	if time.Since(entry.CachedAt) > 24*time.Hour {
		return nil, false
	}

	return entry.Details, true
}

// Set caches details for a notification
func (c *Cache) Set(n *Notification, details *ItemDetails) error {
	if n.Subject.URL == "" || details == nil {
		return nil
	}

	number, err := ExtractIssueNumber(n.Subject.URL)
	if err != nil {
		return err
	}

	entry := CacheEntry{
		Details:   details,
		CachedAt:  time.Now(),
		UpdatedAt: n.UpdatedAt,
		Version:   cacheVersion,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	key := c.cacheKey(n.Repository.FullName, n.Subject.Type, number)
	path := filepath.Join(c.dir, key)

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

// CacheStats contains detailed cache statistics
type CacheStats struct {
	DetailTotal    int
	DetailValid    int
	PRListTotal    int
	PRListValid    int
	NotifListTotal int
	NotifListValid int
}

// Stats returns cache statistics
func (c *Cache) Stats() (total int, validCount int, err error) {
	stats, err := c.DetailedStats()
	if err != nil {
		return 0, 0, err
	}
	totalCount := stats.DetailTotal + stats.PRListTotal + stats.NotifListTotal
	validCount = stats.DetailValid + stats.PRListValid + stats.NotifListValid
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

		// Check if it's a notification list cache entry (starts with "notif_list_")
		if len(name) > 11 && name[:11] == "notif_list_" {
			stats.NotifListTotal++
			var notifEntry NotificationListCacheEntry
			if err := json.Unmarshal(data, &notifEntry); err != nil {
				continue
			}
			if now.Sub(notifEntry.CachedAt) <= NotificationListCacheTTL {
				stats.NotifListValid++
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
			if now.Sub(cacheEntry.CachedAt) <= 24*time.Hour {
				stats.DetailValid++
			}
		}
	}

	return stats, nil
}

// PRListCacheEntry represents a cached list of PRs
type PRListCacheEntry struct {
	PRs      []Notification `json:"prs"`
	CachedAt time.Time      `json:"cachedAt"`
	Version  int            `json:"version"`
}

// PRListCacheTTL is shorter than details cache since PR lists change more frequently
const PRListCacheTTL = 5 * time.Minute

// NotificationListCacheEntry stores cached notifications with fetch timestamp
type NotificationListCacheEntry struct {
	Notifications []Notification `json:"notifications"`
	LastFetchTime time.Time      `json:"lastFetchTime"` // When we last hit the API
	CachedAt      time.Time      `json:"cachedAt"`
	SinceTime     time.Time      `json:"sinceTime"` // The --since value used
	Version       int            `json:"version"`
}

// NotificationListCacheTTL is the max age before a full refresh is required
const NotificationListCacheTTL = 1 * time.Hour

// prListCacheKey generates a cache key for a PR list
func (c *Cache) prListCacheKey(username string, listType string) string {
	return fmt.Sprintf("prlist_%s_%s.json", listType, username)
}

// GetPRList retrieves cached PR list
func (c *Cache) GetPRList(username string, listType string) ([]Notification, bool) {
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
func (c *Cache) SetPRList(username string, listType string, prs []Notification) error {
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

// notificationListCacheKey generates a cache key for a notification list
func (c *Cache) notificationListCacheKey(username string) string {
	return fmt.Sprintf("notif_list_%s.json", username)
}

// GetNotificationList retrieves cached notification list.
// Returns: cached notifications, last fetch time, ok
func (c *Cache) GetNotificationList(username string, sinceTime time.Time) ([]Notification, time.Time, bool) {
	key := c.notificationListCacheKey(username)
	path := filepath.Join(c.dir, key)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, time.Time{}, false
	}

	var entry NotificationListCacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, time.Time{}, false
	}

	// Check version (invalidate old cache format)
	if entry.Version != cacheVersion {
		return nil, time.Time{}, false
	}

	// Check TTL - if cache is too old, require full refresh
	if time.Since(entry.CachedAt) > NotificationListCacheTTL {
		return nil, time.Time{}, false
	}

	// If user's since time is earlier than what we cached, require full refresh
	// (they want more history than we have)
	if sinceTime.Before(entry.SinceTime) {
		return nil, time.Time{}, false
	}

	return entry.Notifications, entry.LastFetchTime, true
}

// SetNotificationList caches a notification list
func (c *Cache) SetNotificationList(username string, notifications []Notification, sinceTime time.Time) error {
	entry := NotificationListCacheEntry{
		Notifications: notifications,
		LastFetchTime: time.Now(),
		CachedAt:      time.Now(),
		SinceTime:     sinceTime,
		Version:       cacheVersion,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	key := c.notificationListCacheKey(username)
	path := filepath.Join(c.dir, key)

	return os.WriteFile(path, data, 0600)
}
