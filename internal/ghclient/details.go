package ghclient

import (
	"github.com/spiffcs/triage/internal/log"
	"github.com/spiffcs/triage/internal/model"
)

// EnrichmentProgress tracks enrichment progress
type EnrichmentProgress struct {
	Total     int
	Completed int64
	Errors    int64
}

// EnrichItemsConcurrent enriches items using GraphQL batch queries with caching.
// Uses GraphQL API (separate quota from Core API) for efficient batch enrichment.
// If cache is nil, creates one internally. Returns the number of cache hits and any error.
func (c *Client) EnrichItemsConcurrent(items []model.Item, workers int, cache *Cache, onProgress func(completed, total int)) (int, error) {
	// Use provided cache or create one internally
	if cache == nil {
		var cacheErr error
		cache, cacheErr = NewCache()
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
		if cache != nil {
			if details, ok := cache.Get(&items[i]); ok {
				items[i].Details = details
				cacheHits++
				// Report each cache hit individually for smooth progress
				if onProgress != nil {
					onProgress(1, total)
				}
				continue
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

	enriched, err := c.EnrichItemsGraphQL(uncachedItems, c.token, func(delta, batchTotal int) {
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
			log.Debug("enriched item",
				"id", n.ID,
				"repo", n.Repository.FullName,
				"isPR", n.Details.IsPR,
				"additions", n.Details.Additions,
				"deletions", n.Details.Deletions,
				"reviewState", n.Details.ReviewState)
		} else {
			log.Debug("item not enriched - Details is nil", "id", n.ID, "repo", n.Repository.FullName)
		}
		// Cache successful enrichment
		if cache != nil && uncachedItems[i].Details != nil {
			if err := cache.Set(&items[origIdx], items[origIdx].Details); err != nil {
				log.Debug("failed to cache item", "id", items[origIdx].ID, "error", err)
			}
		}
	}

	log.Debug("GraphQL enrichment complete", "enriched", enriched, "total", len(uncachedItems))

	return int(cacheHits), nil
}
