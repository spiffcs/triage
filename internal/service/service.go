// Package service provides orchestration between GitHub API and caching layers.
package service

import (
	"context"
	"time"

	"github.com/spiffcs/triage/internal/cache"
	"github.com/spiffcs/triage/internal/ghclient"
	"github.com/spiffcs/triage/internal/log"
	"github.com/spiffcs/triage/internal/model"
)

// ItemService orchestrates data flow between GitHub API and cache.
// It combines the functionality of ItemStore and Enricher.
type ItemService struct {
	fetcher     ghclient.GitHubFetcher
	cache       *cache.Cache
	currentUser string
	since       time.Time
}

// New creates a new ItemService with the given fetcher and cache.
// If cache is nil, caching is disabled.
func New(fetcher ghclient.GitHubFetcher, c *cache.Cache, currentUser string, since time.Time) *ItemService {
	return &ItemService{
		fetcher:     fetcher,
		cache:       c,
		currentUser: currentUser,
		since:       since,
	}
}

// CurrentUser returns the authenticated user's username.
func (s *ItemService) CurrentUser() string {
	return s.currentUser
}

// ItemFetchResult contains the result of a cached item fetch.
type ItemFetchResult struct {
	Items     []model.Item
	FromCache bool
	NewCount  int // Number of new items fetched (for incremental updates)
}

// GetReviewRequestedPRs fetches PRs with caching support.
// Returns (items, fromCache, error).
func (s *ItemService) GetReviewRequestedPRs(ctx context.Context) ([]model.Item, bool, error) {
	// Check cache first
	if s.cache != nil {
		if entry, ok := s.cache.GetList(s.currentUser, cache.ListTypeReviewRequested, cache.ListOptions{}); ok {
			return entry.Items, true, nil
		}
	}

	// Check if rate limited
	if ghclient.IsRateLimited() {
		return nil, false, ghclient.ErrRateLimited
	}

	// Fetch from API
	prs, err := s.fetcher.ListReviewRequestedPRs(ctx, s.currentUser)
	if err != nil {
		return nil, false, err
	}

	// Cache the result
	if s.cache != nil {
		if err := s.cache.SetList(s.currentUser, cache.ListTypeReviewRequested, &cache.ListCacheEntry{
			Items:    prs,
			CachedAt: time.Now(),
			Version:  cache.Version,
		}); err != nil {
			log.Debug("failed to cache review-requested PRs", "error", err)
		}
	}

	return prs, false, nil
}

// GetAuthoredPRs fetches authored PRs with caching support.
// Returns (items, fromCache, error).
func (s *ItemService) GetAuthoredPRs(ctx context.Context) ([]model.Item, bool, error) {
	// Check cache first
	if s.cache != nil {
		if entry, ok := s.cache.GetList(s.currentUser, cache.ListTypeAuthored, cache.ListOptions{}); ok {
			return entry.Items, true, nil
		}
	}

	// Check if rate limited
	if ghclient.IsRateLimited() {
		return nil, false, ghclient.ErrRateLimited
	}

	// Fetch from API
	prs, err := s.fetcher.ListAuthoredPRs(ctx, s.currentUser)
	if err != nil {
		return nil, false, err
	}

	// Cache the result
	if s.cache != nil {
		if err := s.cache.SetList(s.currentUser, cache.ListTypeAuthored, &cache.ListCacheEntry{
			Items:    prs,
			CachedAt: time.Now(),
			Version:  cache.Version,
		}); err != nil {
			log.Debug("failed to cache authored PRs", "error", err)
		}
	}

	return prs, false, nil
}

// GetAssignedIssues fetches assigned issues with caching support.
// Returns (items, fromCache, error).
func (s *ItemService) GetAssignedIssues(ctx context.Context) ([]model.Item, bool, error) {
	// Check cache first
	if s.cache != nil {
		if entry, ok := s.cache.GetList(s.currentUser, cache.ListTypeAssignedIssues, cache.ListOptions{}); ok {
			return entry.Items, true, nil
		}
	}

	// Check if rate limited
	if ghclient.IsRateLimited() {
		return nil, false, ghclient.ErrRateLimited
	}

	// Fetch from API
	issues, err := s.fetcher.ListAssignedIssues(ctx, s.currentUser)
	if err != nil {
		return nil, false, err
	}

	// Cache the result
	if s.cache != nil {
		if err := s.cache.SetList(s.currentUser, cache.ListTypeAssignedIssues, &cache.ListCacheEntry{
			Items:    issues,
			CachedAt: time.Now(),
			Version:  cache.Version,
		}); err != nil {
			log.Debug("failed to cache assigned issues", "error", err)
		}
	}

	return issues, false, nil
}

// GetAssignedPRs fetches assigned PRs with caching support.
// Returns (items, fromCache, error).
func (s *ItemService) GetAssignedPRs(ctx context.Context) ([]model.Item, bool, error) {
	// Check cache first
	if s.cache != nil {
		if entry, ok := s.cache.GetList(s.currentUser, cache.ListTypeAssignedPRs, cache.ListOptions{}); ok {
			return entry.Items, true, nil
		}
	}

	// Check if rate limited
	if ghclient.IsRateLimited() {
		return nil, false, ghclient.ErrRateLimited
	}

	// Fetch from API
	prs, err := s.fetcher.ListAssignedPRs(ctx, s.currentUser)
	if err != nil {
		return nil, false, err
	}

	// Cache the result
	if s.cache != nil {
		if err := s.cache.SetList(s.currentUser, cache.ListTypeAssignedPRs, &cache.ListCacheEntry{
			Items:    prs,
			CachedAt: time.Now(),
			Version:  cache.Version,
		}); err != nil {
			log.Debug("failed to cache assigned PRs", "error", err)
		}
	}

	return prs, false, nil
}

// GetUnreadItems fetches items with incremental caching.
// It returns cached items merged with any new ones since the last fetch.
func (s *ItemService) GetUnreadItems(ctx context.Context) (*ItemFetchResult, error) {
	result := &ItemFetchResult{}
	opts := cache.ListOptions{SinceTime: s.since}

	// Check if rate limited - return cached data if available
	if ghclient.IsRateLimited() {
		if s.cache != nil {
			if entry, ok := s.cache.GetList(s.currentUser, cache.ListTypeNotifications, opts); ok {
				result.Items = entry.Items
				result.FromCache = true
				return result, nil
			}
		}
		return nil, ghclient.ErrRateLimited
	}

	// Check cache
	if s.cache != nil {
		if entry, ok := s.cache.GetList(s.currentUser, cache.ListTypeNotifications, opts); ok {
			// Fetch only NEW notifications since last fetch
			newItems, err := s.fetcher.ListUnreadNotifications(ctx, entry.LastFetchTime)
			if err != nil {
				// Return cached on error
				log.Debug("failed to fetch new items, using cache", "error", err)
				result.Items = entry.Items
				result.FromCache = true
				return result, nil
			}

			// Merge: new items replace old ones by ID
			merged := mergeItems(entry.Items, newItems, s.since)
			result.Items = merged
			result.FromCache = true
			result.NewCount = len(newItems)

			// Update cache with merged result
			if err := s.cache.SetList(s.currentUser, cache.ListTypeNotifications, &cache.ListCacheEntry{
				Items:         merged,
				CachedAt:      time.Now(),
				LastFetchTime: time.Now(),
				SinceTime:     s.since,
				Version:       cache.Version,
			}); err != nil {
				log.Debug("failed to update item cache", "error", err)
			}

			return result, nil
		}
	}

	// No cache - full fetch
	items, err := s.fetcher.ListUnreadNotifications(ctx, s.since)
	if err != nil {
		return nil, err
	}

	result.Items = items
	result.FromCache = false

	// Cache result
	if s.cache != nil {
		if err := s.cache.SetList(s.currentUser, cache.ListTypeNotifications, &cache.ListCacheEntry{
			Items:         items,
			CachedAt:      time.Now(),
			LastFetchTime: time.Now(),
			SinceTime:     s.since,
			Version:       cache.Version,
		}); err != nil {
			log.Debug("failed to cache items", "error", err)
		}
	}

	return result, nil
}

// GetOrphanedContributions fetches orphaned contributions with caching support.
// Returns (items, fromCache, error).
func (s *ItemService) GetOrphanedContributions(ctx context.Context, opts ghclient.OrphanedSearchOptions) ([]model.Item, bool, error) {
	if len(opts.Repos) == 0 {
		return nil, false, nil
	}

	// Use service's since if opts.Since is zero
	since := opts.Since
	if since.IsZero() {
		since = s.since
	}

	cacheOpts := cache.ListOptions{
		SinceTime: since,
		Repos:     opts.Repos,
	}

	// Try cache first
	if s.cache != nil {
		if entry, ok := s.cache.GetList(s.currentUser, cache.ListTypeOrphaned, cacheOpts); ok {
			return entry.Items, true, nil
		}
	}

	// Fetch fresh data
	orphaned, err := s.fetcher.ListOrphanedContributions(ctx, opts)
	if err != nil {
		return nil, false, err
	}

	// Cache the result
	if s.cache != nil {
		if cacheErr := s.cache.SetList(s.currentUser, cache.ListTypeOrphaned, &cache.ListCacheEntry{
			Items:     orphaned,
			CachedAt:  time.Now(),
			SinceTime: since,
			Repos:     opts.Repos,
			Version:   cache.Version,
		}); cacheErr != nil {
			log.Debug("failed to cache orphaned list", "error", cacheErr)
		}
	}

	return orphaned, false, nil
}

// Enrich enriches items using GraphQL batch queries with caching.
// Uses GraphQL API (separate quota from Core API) for efficient batch enrichment.
// Returns the number of cache hits and any error.
func (s *ItemService) Enrich(ctx context.Context, items []model.Item, onProgress func(completed, total int)) (int, error) {
	// Use provided cache or create one internally
	c := s.cache
	if c == nil {
		var cacheErr error
		c, cacheErr = cache.NewCache()
		if cacheErr != nil {
			log.Debug("cache unavailable", "error", cacheErr)
		}
	}

	total := len(items)
	var cacheHits int64

	// First pass: check cache and build list of items needing enrichment
	uncachedItems := make([]model.Item, 0, len(items))
	uncachedIndices := make([]int, 0, len(items))

	for i := range items {
		if c != nil {
			key, ok := buildCacheKey(&items[i])
			if ok {
				if cachedItem, cacheOk := c.Get(key, items[i].UpdatedAt); cacheOk {
					// Copy enriched data from cached item
					items[i].Type = cachedItem.Type
					items[i].Number = cachedItem.Number
					items[i].State = cachedItem.State
					items[i].HTMLURL = cachedItem.HTMLURL
					items[i].CreatedAt = cachedItem.CreatedAt
					items[i].ClosedAt = cachedItem.ClosedAt
					items[i].Author = cachedItem.Author
					items[i].Assignees = cachedItem.Assignees
					items[i].Labels = cachedItem.Labels
					items[i].CommentCount = cachedItem.CommentCount
					items[i].AuthorAssociation = cachedItem.AuthorAssociation
					items[i].LastTeamActivityAt = cachedItem.LastTeamActivityAt
					items[i].ConsecutiveAuthorComments = cachedItem.ConsecutiveAuthorComments
					items[i].Details = cachedItem.Details
					cacheHits++
					// Report each cache hit individually for smooth progress
					if onProgress != nil {
						onProgress(1, total)
					}
					continue
				}
			}
		}
		uncachedItems = append(uncachedItems, items[i])
		uncachedIndices = append(uncachedIndices, i)
	}

	if cacheHits > 0 {
		log.Info("cache hit", "count", cacheHits, "total", total)
	}

	if len(uncachedItems) == 0 {
		return int(cacheHits), nil
	}

	// Use GraphQL for batch enrichment (uses GraphQL quota, not Core API)
	log.Debug("enriching via GraphQL", "count", len(uncachedItems))

	enriched, err := s.fetcher.EnrichItemsGraphQL(ctx, uncachedItems, s.fetcher.Token(), func(delta, batchTotal int) {
		if onProgress != nil {
			// Pass through the delta (number of items just processed)
			onProgress(delta, total)
		}
	})

	if err != nil {
		log.Debug("GraphQL enrichment error", "error", err)
	}

	// Copy enriched data back to original slice and cache results
	for i, origIdx := range uncachedIndices {
		items[origIdx] = uncachedItems[i]
		// Log what we're copying back
		n := &items[origIdx]
		if n.Details != nil {
			if pr := n.GetPRDetails(); pr != nil {
				log.Debug("enriched item",
					"id", n.ID,
					"repo", n.Repository.FullName,
					"isPR", true,
					"additions", pr.Additions,
					"deletions", pr.Deletions,
					"reviewState", pr.ReviewState)
			} else {
				log.Debug("enriched item",
					"id", n.ID,
					"repo", n.Repository.FullName,
					"isPR", false)
			}
		} else {
			log.Debug("item not enriched - Details is nil", "id", n.ID, "repo", n.Repository.FullName)
		}
		// Cache successful enrichment
		if c != nil && uncachedItems[i].Details != nil {
			key, ok := buildCacheKey(&items[origIdx])
			if ok {
				if err := c.Set(key, items[origIdx].UpdatedAt, &items[origIdx]); err != nil {
					log.Debug("failed to cache item", "id", items[origIdx].ID, "error", err)
				}
			}
		}
	}

	log.Debug("GraphQL enrichment complete", "enriched", enriched, "total", len(uncachedItems))

	return int(cacheHits), nil
}

// buildCacheKey creates a cache key from an item.
// Returns false if the key cannot be built (e.g., no URL).
func buildCacheKey(item *model.Item) (cache.Key, bool) {
	if item.Subject.URL == "" {
		return cache.Key{}, false
	}

	number, err := ghclient.ExtractIssueNumber(item.Subject.URL)
	if err != nil {
		return cache.Key{}, false
	}

	return cache.Key{
		RepoFullName: item.Repository.FullName,
		SubjectType:  item.Subject.Type,
		Number:       number,
	}, true
}

// mergeItems merges cached and fresh items.
// Fresh items replace cached ones by ID. Only unread items
// within the since timeframe are kept.
func mergeItems(cached, fresh []model.Item, since time.Time) []model.Item {
	byID := make(map[string]model.Item)

	// Add cached items
	for _, n := range cached {
		byID[n.ID] = n
	}

	// New overwrites old
	for _, n := range fresh {
		byID[n.ID] = n
	}

	// Build result, filtering appropriately
	result := make([]model.Item, 0, len(byID))
	for _, n := range byID {
		// Only keep unread items
		if !n.Unread {
			continue
		}
		// Only keep items within the since timeframe
		if n.UpdatedAt.Before(since) {
			continue
		}
		result = append(result, n)
	}

	return result
}
