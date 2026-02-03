package cmd

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/spiffcs/triage/internal/ghclient"
	"github.com/spiffcs/triage/internal/log"
	"github.com/spiffcs/triage/internal/model"
	"github.com/spiffcs/triage/internal/service"
	"github.com/spiffcs/triage/internal/tui"
	"golang.org/x/sync/errgroup"
)

// fetchResult contains all data fetched from GitHub.
type fetchResult struct {
	Notifications  []model.Item
	ReviewPRs      []model.Item
	AuthoredPRs    []model.Item
	AssignedIssues []model.Item
	AssignedPRs    []model.Item
	Orphaned       []model.Item
	RateLimited    bool

	// Cache statistics
	NotifCached       bool
	NotifNewCount     int
	ReviewCached      bool
	AuthoredCached    bool
	AssignedCached    bool
	AssignedPRsCached bool
	OrphanedCached    bool
}

// totalFetched returns the total number of items fetched.
func (r *fetchResult) totalFetched() int {
	return len(r.Notifications) + len(r.ReviewPRs) + len(r.AuthoredPRs) + len(r.AssignedIssues) + len(r.AssignedPRs) + len(r.Orphaned)
}

// fetchOptions configures the fetch operation.
type fetchOptions struct {
	SinceLabel string // Human-readable label like "1w"
	Events     chan tui.Event

	// Orphaned contribution options
	IncludeOrphaned     bool
	OrphanedRepos       []string
	StaleDays           int
	ConsecutiveComments int
}

// fetchAll fetches all data sources in parallel using errgroup for context propagation.
func fetchAll(ctx context.Context, svc *service.ItemService, opts fetchOptions) (*fetchResult, error) {
	totalFetches := 6
	if !opts.IncludeOrphaned || len(opts.OrphanedRepos) == 0 {
		totalFetches = 5
	}

	// Track progress of parallel fetches
	var completedFetches int32

	updateFetchProgress := func() {
		current := atomic.AddInt32(&completedFetches, 1)
		progress := float64(current) / float64(totalFetches)
		msg := fmt.Sprintf("%s (%d/%d sources)", opts.SinceLabel, current, totalFetches)
		sendTaskEvent(opts.Events, tui.TaskFetch, tui.StatusRunning,
			tui.WithProgress(progress),
			tui.WithMessage(msg))
	}

	sendTaskEvent(opts.Events, tui.TaskFetch, tui.StatusRunning,
		tui.WithProgress(0.0),
		tui.WithMessage(fmt.Sprintf("for the past %s (0/%d sources)", opts.SinceLabel, totalFetches)))

	result := &fetchResult{}
	var mu sync.Mutex

	g, gctx := errgroup.WithContext(ctx)

	// Fetch notifications (with caching)
	g.Go(func() error {
		notifResult, err := svc.GetUnreadItems(gctx)
		if err != nil {
			if errors.Is(err, ghclient.ErrRateLimited) {
				mu.Lock()
				result.RateLimited = true
				mu.Unlock()
				updateFetchProgress()
				return nil
			}
			updateFetchProgress()
			return fmt.Errorf("notifications: %w", err)
		}
		mu.Lock()
		result.Notifications = notifResult.Items
		result.NotifCached = notifResult.FromCache
		result.NotifNewCount = notifResult.NewCount
		mu.Unlock()
		updateFetchProgress()
		return nil
	})

	// Fetch review-requested PRs
	g.Go(func() error {
		prs, cached, err := svc.GetReviewRequestedPRs(gctx)
		if err != nil {
			if errors.Is(err, ghclient.ErrRateLimited) {
				mu.Lock()
				result.RateLimited = true
				mu.Unlock()
				updateFetchProgress()
				return nil
			}
			updateFetchProgress()
			return fmt.Errorf("review-requested PRs: %w", err)
		}
		mu.Lock()
		result.ReviewPRs = prs
		result.ReviewCached = cached
		mu.Unlock()
		updateFetchProgress()
		return nil
	})

	// Fetch authored PRs
	g.Go(func() error {
		prs, cached, err := svc.GetAuthoredPRs(gctx)
		if err != nil {
			if errors.Is(err, ghclient.ErrRateLimited) {
				mu.Lock()
				result.RateLimited = true
				mu.Unlock()
				updateFetchProgress()
				return nil
			}
			updateFetchProgress()
			return fmt.Errorf("authored PRs: %w", err)
		}
		mu.Lock()
		result.AuthoredPRs = prs
		result.AuthoredCached = cached
		mu.Unlock()
		updateFetchProgress()
		return nil
	})

	// Fetch assigned issues
	g.Go(func() error {
		issues, cached, err := svc.GetAssignedIssues(gctx)
		if err != nil {
			if errors.Is(err, ghclient.ErrRateLimited) {
				mu.Lock()
				result.RateLimited = true
				mu.Unlock()
				updateFetchProgress()
				return nil
			}
			updateFetchProgress()
			return fmt.Errorf("assigned issues: %w", err)
		}
		mu.Lock()
		result.AssignedIssues = issues
		result.AssignedCached = cached
		mu.Unlock()
		updateFetchProgress()
		return nil
	})

	// Fetch assigned PRs
	g.Go(func() error {
		prs, cached, err := svc.GetAssignedPRs(gctx)
		if err != nil {
			if errors.Is(err, ghclient.ErrRateLimited) {
				mu.Lock()
				result.RateLimited = true
				mu.Unlock()
				updateFetchProgress()
				return nil
			}
			updateFetchProgress()
			return fmt.Errorf("assigned PRs: %w", err)
		}
		mu.Lock()
		result.AssignedPRs = prs
		result.AssignedPRsCached = cached
		mu.Unlock()
		updateFetchProgress()
		return nil
	})

	// Fetch orphaned contributions (if enabled)
	if len(opts.OrphanedRepos) > 0 {
		g.Go(func() error {
			searchOpts := ghclient.OrphanedSearchOptions{
				Repos:                     opts.OrphanedRepos,
				StaleDays:                 opts.StaleDays,
				ConsecutiveAuthorComments: opts.ConsecutiveComments,
				MaxPerRepo:                50,
			}
			orphaned, cached, err := svc.GetOrphanedContributions(gctx, searchOpts)
			if err != nil {
				if errors.Is(err, ghclient.ErrRateLimited) {
					mu.Lock()
					result.RateLimited = true
					mu.Unlock()
					updateFetchProgress()
					return nil
				}
				updateFetchProgress()
				return fmt.Errorf("orphaned contributions: %w", err)
			}
			mu.Lock()
			result.Orphaned = orphaned
			result.OrphanedCached = cached
			mu.Unlock()
			updateFetchProgress()
			return nil
		})
	}

	// Wait for all goroutines and collect errors
	err := g.Wait()

	// Send rate limit event to TUI if rate limited
	if result.RateLimited && opts.Events != nil {
		_, _, resetAt, _ := ghclient.GetRateLimitStatus()
		opts.Events <- tui.RateLimitEvent{
			Limited: true,
			ResetAt: resetAt,
		}
	}

	totalFetched := result.totalFetched()
	fetchMsg := fmt.Sprintf("for the past %s (%d items)", opts.SinceLabel, totalFetched)
	if result.NotifCached && result.NotifNewCount > 0 {
		fetchMsg = fmt.Sprintf("for the past %s (%d items, %d new)", opts.SinceLabel, totalFetched, result.NotifNewCount)
	} else if result.NotifCached {
		fetchMsg = fmt.Sprintf("for the past %s (%d items, cached)", opts.SinceLabel, totalFetched)
	}

	if err != nil {
		sendTaskEvent(opts.Events, tui.TaskFetch, tui.StatusError, tui.WithError(err))
	} else {
		sendTaskEvent(opts.Events, tui.TaskFetch, tui.StatusComplete, tui.WithMessage(fetchMsg))
	}

	log.Info("fetched data",
		"notifications", len(result.Notifications),
		"reviewPRs", len(result.ReviewPRs),
		"authoredPRs", len(result.AuthoredPRs),
		"assignedIssues", len(result.AssignedIssues),
		"assignedPRs", len(result.AssignedPRs),
		"orphaned", len(result.Orphaned),
		"notifCached", result.NotifCached,
		"notifNewCount", result.NotifNewCount,
		"reviewCached", result.ReviewCached,
		"authoredCached", result.AuthoredCached,
		"assignedCached", result.AssignedCached,
		"assignedPRsCached", result.AssignedPRsCached,
		"orphanedCached", result.OrphanedCached)

	return result, err
}
