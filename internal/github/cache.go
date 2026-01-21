package github

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Cache stores notification details to avoid repeated API calls
type Cache struct {
	dir string
}

// CacheEntry represents a cached notification with its details
type CacheEntry struct {
	Details   *ItemDetails `json:"details"`
	CachedAt  time.Time    `json:"cachedAt"`
	UpdatedAt time.Time    `json:"updatedAt"` // Notification's UpdatedAt for invalidation
}

// NewCache creates a new cache instance
func NewCache() (*Cache, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	cacheDir := filepath.Join(home, ".cache", "priority", "details")
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	return &Cache{dir: cacheDir}, nil
}

// cacheKey generates a cache key for a notification
func (c *Cache) cacheKey(repoFullName string, subjectType SubjectType, number int) string {
	return fmt.Sprintf("%s_%s_%d.json",
		filepath.Base(repoFullName), // Use just repo name to avoid path issues
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
	DetailTotal   int
	DetailValid   int
	PRListTotal   int
	PRListValid   int
}

// Stats returns cache statistics
func (c *Cache) Stats() (total int, validCount int, err error) {
	stats, err := c.DetailedStats()
	if err != nil {
		return 0, 0, err
	}
	return stats.DetailTotal + stats.PRListTotal, stats.DetailValid + stats.PRListValid, nil
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

		// Check if it's a PR list cache entry (starts with "prlist_")
		if len(entry.Name()) > 7 && entry.Name()[:7] == "prlist_" {
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
}

// PRListCacheTTL is shorter than details cache since PR lists change more frequently
const PRListCacheTTL = 5 * time.Minute

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
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	key := c.prListCacheKey(username, listType)
	path := filepath.Join(c.dir, key)

	return os.WriteFile(path, data, 0600)
}
