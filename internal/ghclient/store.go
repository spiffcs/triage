package ghclient

import (
	"context"
	"time"

	"github.com/spiffcs/triage/internal/cache"
	"github.com/spiffcs/triage/internal/log"
	"github.com/spiffcs/triage/internal/model"
)

// ItemStore provides cache-aware item fetching.
// It wraps a GitHubFetcher and Cache to provide transparent caching
// of GitHub API responses.
type ItemStore struct {
	fetcher GitHubFetcher
	cache   *cache.Cache
}

// NewItemStore creates a new ItemStore with the given fetcher and cache.
// If cache is nil, caching is disabled.
func NewItemStore(fetcher GitHubFetcher, cache *cache.Cache) *ItemStore {
	return &ItemStore{
		fetcher: fetcher,
		cache:   cache,
	}
}

// ItemFetchResult contains the result of a cached item fetch.
type ItemFetchResult struct {
	Items     []model.Item
	FromCache bool
	NewCount  int // Number of new items fetched (for incremental updates)
}

// GetReviewRequestedPRs fetches PRs with caching support.
// Returns (items, fromCache, error).
func (s *ItemStore) GetReviewRequestedPRs(ctx context.Context, username string) ([]model.Item, bool, error) {
	// Check cache first
	if s.cache != nil {
		if prs, ok := s.cache.GetPRList(username, "review-requested"); ok {
			return prs, true, nil
		}
	}

	// Check if rate limited
	if globalRateLimitState.IsLimited() {
		return nil, false, ErrRateLimited
	}

	// Fetch from API
	prs, err := s.fetcher.ListReviewRequestedPRs(ctx, username)
	if err != nil {
		return nil, false, err
	}

	// Cache the result
	if s.cache != nil {
		if err := s.cache.SetPRList(username, "review-requested", prs); err != nil {
			log.Debug("failed to cache review-requested PRs", "error", err)
		}
	}

	return prs, false, nil
}

// GetAuthoredPRs fetches authored PRs with caching support.
// Returns (items, fromCache, error).
func (s *ItemStore) GetAuthoredPRs(ctx context.Context, username string) ([]model.Item, bool, error) {
	// Check cache first
	if s.cache != nil {
		if prs, ok := s.cache.GetPRList(username, "authored"); ok {
			return prs, true, nil
		}
	}

	// Check if rate limited
	if globalRateLimitState.IsLimited() {
		return nil, false, ErrRateLimited
	}

	// Fetch from API
	prs, err := s.fetcher.ListAuthoredPRs(ctx, username)
	if err != nil {
		return nil, false, err
	}

	// Cache the result
	if s.cache != nil {
		if err := s.cache.SetPRList(username, "authored", prs); err != nil {
			log.Debug("failed to cache authored PRs", "error", err)
		}
	}

	return prs, false, nil
}

// GetAssignedIssues fetches assigned issues with caching support.
// Returns (items, fromCache, error).
func (s *ItemStore) GetAssignedIssues(ctx context.Context, username string) ([]model.Item, bool, error) {
	// Check cache first
	if s.cache != nil {
		if issues, ok := s.cache.GetPRList(username, "assigned-issues"); ok {
			return issues, true, nil
		}
	}

	// Check if rate limited
	if globalRateLimitState.IsLimited() {
		return nil, false, ErrRateLimited
	}

	// Fetch from API
	issues, err := s.fetcher.ListAssignedIssues(ctx, username)
	if err != nil {
		return nil, false, err
	}

	// Cache the result
	if s.cache != nil {
		if err := s.cache.SetPRList(username, "assigned-issues", issues); err != nil {
			log.Debug("failed to cache assigned issues", "error", err)
		}
	}

	return issues, false, nil
}

// GetUnreadItems fetches items with incremental caching.
// It returns cached items merged with any new ones since the last fetch.
func (s *ItemStore) GetUnreadItems(ctx context.Context, username string, since time.Time) (*ItemFetchResult, error) {
	result := &ItemFetchResult{}

	// Check if rate limited - return cached data if available
	if globalRateLimitState.IsLimited() {
		if s.cache != nil {
			if cached, _, ok := s.cache.GetItemList(username, since); ok {
				result.Items = cached
				result.FromCache = true
				return result, nil
			}
		}
		return nil, ErrRateLimited
	}

	// Check cache
	if s.cache != nil {
		cached, lastFetch, ok := s.cache.GetItemList(username, since)
		if ok {
			// Fetch only NEW notifications since last fetch
			newItems, err := s.fetcher.ListUnreadNotifications(ctx, lastFetch)
			if err != nil {
				// Return cached on error
				log.Debug("failed to fetch new items, using cache", "error", err)
				result.Items = cached
				result.FromCache = true
				return result, nil
			}

			// Merge: new items replace old ones by ID
			merged := mergeItems(cached, newItems, since)
			result.Items = merged
			result.FromCache = true
			result.NewCount = len(newItems)

			// Update cache with merged result
			if err := s.cache.SetItemList(username, merged, since); err != nil {
				log.Debug("failed to update item cache", "error", err)
			}

			return result, nil
		}
	}

	// No cache - full fetch
	items, err := s.fetcher.ListUnreadNotifications(ctx, since)
	if err != nil {
		return nil, err
	}

	result.Items = items
	result.FromCache = false

	// Cache result
	if s.cache != nil {
		if err := s.cache.SetItemList(username, items, since); err != nil {
			log.Debug("failed to cache items", "error", err)
		}
	}

	return result, nil
}

// GetOrphanedContributions fetches orphaned contributions with caching support.
// Returns (items, fromCache, error).
func (s *ItemStore) GetOrphanedContributions(ctx context.Context, opts OrphanedSearchOptions, username string) ([]model.Item, bool, error) {
	if len(opts.Repos) == 0 {
		return nil, false, nil
	}

	// Try cache first
	if s.cache != nil {
		if cached, ok := s.cache.GetOrphanedList(username, opts.Repos, opts.Since); ok {
			return cached, true, nil
		}
	}

	// Fetch fresh data
	orphaned, err := s.fetcher.ListOrphanedContributions(ctx, opts)
	if err != nil {
		return nil, false, err
	}

	// Cache the result
	if s.cache != nil {
		if cacheErr := s.cache.SetOrphanedList(username, orphaned, opts.Repos, opts.Since); cacheErr != nil {
			log.Debug("failed to cache orphaned list", "error", cacheErr)
		}
	}

	return orphaned, false, nil
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
