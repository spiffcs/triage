package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/spiffcs/triage/internal/ghclient"
	"github.com/spiffcs/triage/internal/model"
	"golang.org/x/sync/errgroup"
)

// ProgressFunc is called as fetch sources complete.
type ProgressFunc func(completed, total int)

// FetchOptions configures the fetch operation.
type FetchOptions struct {
	OrphanedRepos       []string
	StaleDays           int
	ConsecutiveComments int
}

// FetchResult contains all data fetched from GitHub.
type FetchResult struct {
	Notifications  []model.Item
	ReviewPRs      []model.Item
	AuthoredPRs    []model.Item
	AssignedIssues []model.Item
	AssignedPRs    []model.Item
	Orphaned       []model.Item
	RateLimited    bool
}

// TotalFetched returns the total number of items fetched across all sources.
func (r *FetchResult) TotalFetched() int {
	return len(r.Notifications) + len(r.ReviewPRs) + len(r.AuthoredPRs) +
		len(r.AssignedIssues) + len(r.AssignedPRs) + len(r.Orphaned)
}

// MergeStats contains the counts of items added during merge operations.
type MergeStats struct {
	ReviewPRsAdded      int
	AuthoredPRsAdded    int
	AssignedIssuesAdded int
	AssignedPRsAdded    int
	OrphanedAdded       int
}

// Merge combines all sources into a single deduplicated list.
// Returns a new slice (does not mutate r.Notifications).
func (r *FetchResult) Merge() ([]model.Item, MergeStats) {
	// Start with a copy of Notifications to avoid mutating the original.
	merged := make([]model.Item, len(r.Notifications))
	copy(merged, r.Notifications)

	var stats MergeStats

	if len(r.ReviewPRs) > 0 {
		merged, stats.ReviewPRsAdded = deduplicateItems(merged, r.ReviewPRs, model.SubjectPullRequest)
	}
	if len(r.AuthoredPRs) > 0 {
		merged, stats.AuthoredPRsAdded = deduplicateItems(merged, r.AuthoredPRs, model.SubjectPullRequest)
	}
	if len(r.AssignedIssues) > 0 {
		merged, stats.AssignedIssuesAdded = deduplicateItems(merged, r.AssignedIssues, model.SubjectIssue)
	}
	if len(r.AssignedPRs) > 0 {
		merged, stats.AssignedPRsAdded = deduplicateItems(merged, r.AssignedPRs, model.SubjectPullRequest)
	}
	if len(r.Orphaned) > 0 {
		merged, stats.OrphanedAdded = deduplicateOrphaned(merged, r.Orphaned)
	}

	return merged, stats
}

// Fetcher fetches all data sources from GitHub in parallel.
type Fetcher struct {
	svc        *ItemService
	onProgress ProgressFunc
}

// NewFetcher creates a Fetcher. onProgress may be nil (no-op).
func NewFetcher(svc *ItemService, onProgress ProgressFunc) *Fetcher {
	return &Fetcher{
		svc:        svc,
		onProgress: onProgress,
	}
}

func (f *Fetcher) reportProgress(completed, total int) {
	if f.onProgress != nil {
		f.onProgress(completed, total)
	}
}

// FetchAll fetches all data sources in parallel using errgroup for context propagation.
func (f *Fetcher) FetchAll(ctx context.Context, opts FetchOptions) (*FetchResult, error) {
	totalFetches := 5
	if len(opts.OrphanedRepos) > 0 {
		totalFetches = 6
	}

	var completedFetches int32
	f.reportProgress(0, totalFetches)

	updateProgress := func() {
		current := int(atomic.AddInt32(&completedFetches, 1))
		f.reportProgress(current, totalFetches)
	}

	result := &FetchResult{}
	var mu sync.Mutex

	g, gctx := errgroup.WithContext(ctx)

	// Fetch notifications (with caching)
	g.Go(func() error {
		notifResult, err := f.svc.UnreadItems(gctx)
		if err != nil {
			if errors.Is(err, ghclient.ErrRateLimited) {
				mu.Lock()
				result.RateLimited = true
				mu.Unlock()
				updateProgress()
				return nil
			}
			updateProgress()
			return fmt.Errorf("notifications: %w", err)
		}
		mu.Lock()
		result.Notifications = notifResult.Items
		mu.Unlock()
		updateProgress()
		return nil
	})

	// Fetch review-requested PRs
	g.Go(func() error {
		prs, _, err := f.svc.ReviewRequestedPRs(gctx)
		if err != nil {
			if errors.Is(err, ghclient.ErrRateLimited) {
				mu.Lock()
				result.RateLimited = true
				mu.Unlock()
				updateProgress()
				return nil
			}
			updateProgress()
			return fmt.Errorf("review-requested PRs: %w", err)
		}
		mu.Lock()
		result.ReviewPRs = prs
		mu.Unlock()
		updateProgress()
		return nil
	})

	// Fetch authored PRs
	g.Go(func() error {
		prs, _, err := f.svc.AuthoredPRs(gctx)
		if err != nil {
			if errors.Is(err, ghclient.ErrRateLimited) {
				mu.Lock()
				result.RateLimited = true
				mu.Unlock()
				updateProgress()
				return nil
			}
			updateProgress()
			return fmt.Errorf("authored PRs: %w", err)
		}
		mu.Lock()
		result.AuthoredPRs = prs
		mu.Unlock()
		updateProgress()
		return nil
	})

	// Fetch assigned issues
	g.Go(func() error {
		issues, _, err := f.svc.AssignedIssues(gctx)
		if err != nil {
			if errors.Is(err, ghclient.ErrRateLimited) {
				mu.Lock()
				result.RateLimited = true
				mu.Unlock()
				updateProgress()
				return nil
			}
			updateProgress()
			return fmt.Errorf("assigned issues: %w", err)
		}
		mu.Lock()
		result.AssignedIssues = issues
		mu.Unlock()
		updateProgress()
		return nil
	})

	// Fetch assigned PRs
	g.Go(func() error {
		prs, _, err := f.svc.AssignedPRs(gctx)
		if err != nil {
			if errors.Is(err, ghclient.ErrRateLimited) {
				mu.Lock()
				result.RateLimited = true
				mu.Unlock()
				updateProgress()
				return nil
			}
			updateProgress()
			return fmt.Errorf("assigned PRs: %w", err)
		}
		mu.Lock()
		result.AssignedPRs = prs
		mu.Unlock()
		updateProgress()
		return nil
	})

	// Fetch orphaned contributions (if configured)
	if len(opts.OrphanedRepos) > 0 {
		g.Go(func() error {
			searchOpts := ghclient.OrphanedSearchOptions{
				Repos:                     opts.OrphanedRepos,
				StaleDays:                 opts.StaleDays,
				ConsecutiveAuthorComments: opts.ConsecutiveComments,
				MaxPerRepo:                50,
			}
			orphaned, _, err := f.svc.OrphanedContributions(gctx, searchOpts)
			if err != nil {
				if errors.Is(err, ghclient.ErrRateLimited) {
					mu.Lock()
					result.RateLimited = true
					mu.Unlock()
					updateProgress()
					return nil
				}
				updateProgress()
				return fmt.Errorf("orphaned contributions: %w", err)
			}
			mu.Lock()
			result.Orphaned = orphaned
			mu.Unlock()
			updateProgress()
			return nil
		})
	}

	err := g.Wait()
	return result, err
}

// deduplicateItems adds items that aren't already in the existing list.
// It filters existing items by subjectType and checks for duplicates
// by repo#number and Subject.URL. Returns the merged list and count of added items.
func deduplicateItems(
	existing []model.Item,
	newItems []model.Item,
	subjectType model.SubjectType,
) ([]model.Item, int) {
	// Build sets of existing identifiers for items matching the subject type
	existingKeys := make(map[string]bool)
	existingURLs := make(map[string]bool)

	for _, n := range existing {
		if n.Subject.Type == subjectType {
			if n.Subject.URL != "" {
				existingURLs[n.Subject.URL] = true
			}
			if n.Details != nil {
				key := fmt.Sprintf("%s#%d", n.Repository.FullName, n.Number)
				existingKeys[key] = true
			}
		}
	}

	// Add items that aren't already in the list
	added := 0
	for _, item := range newItems {
		if item.Details == nil {
			continue
		}

		key := fmt.Sprintf("%s#%d", item.Repository.FullName, item.Number)
		if existingKeys[key] || existingURLs[item.Subject.URL] {
			continue
		}

		existing = append(existing, item)
		existingKeys[key] = true
		added++
	}

	return existing, added
}

// deduplicateOrphaned adds orphaned contributions that aren't already in the list.
// Returns the merged list and the count of newly added items.
func deduplicateOrphaned(existing []model.Item, orphaned []model.Item) ([]model.Item, int) {
	// Orphaned items can be either PRs or issues, so we need to check both types
	existingKeys := make(map[string]bool)
	existingURLs := make(map[string]bool)

	for _, n := range existing {
		if n.Subject.URL != "" {
			existingURLs[n.Subject.URL] = true
		}
		if n.Details != nil {
			key := fmt.Sprintf("%s#%d", n.Repository.FullName, n.Number)
			existingKeys[key] = true
		}
	}

	// Add items that aren't already in the list
	added := 0
	for _, item := range orphaned {
		if item.Details == nil {
			continue
		}

		key := fmt.Sprintf("%s#%d", item.Repository.FullName, item.Number)
		if existingKeys[key] || existingURLs[item.Subject.URL] {
			continue
		}

		existing = append(existing, item)
		existingKeys[key] = true
		added++
	}

	return existing, added
}
