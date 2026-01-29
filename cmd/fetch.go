package cmd

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spiffcs/triage/internal/ghclient"
	"github.com/spiffcs/triage/internal/log"
	"github.com/spiffcs/triage/internal/model"
	"github.com/spiffcs/triage/internal/tui"
)

// fetchResult contains all data fetched from GitHub.
type fetchResult struct {
	Notifications  []model.Item
	ReviewPRs      []model.Item
	AuthoredPRs    []model.Item
	AssignedIssues []model.Item
	Orphaned       []model.Item
	RateLimited    bool

	// Cache statistics
	NotifCached    bool
	NotifNewCount  int
	ReviewCached   bool
	AuthoredCached bool
	AssignedCached bool
	OrphanedCached bool
}

// totalFetched returns the total number of items fetched.
func (r *fetchResult) totalFetched() int {
	return len(r.Notifications) + len(r.ReviewPRs) + len(r.AuthoredPRs) + len(r.AssignedIssues) + len(r.Orphaned)
}

// fetchOptions configures the fetch operation.
type fetchOptions struct {
	Since       time.Time
	SinceLabel  string // Human-readable label like "1w"
	CurrentUser string
	Events      chan tui.Event

	// Orphaned contribution options
	IncludeOrphaned     bool
	OrphanedRepos       []string
	StaleDays           int
	ConsecutiveComments int
}

// fetchAll fetches all data sources in parallel.
func fetchAll(ctx context.Context, client *ghclient.Client, cache *ghclient.Cache, opts fetchOptions) (*fetchResult, error) {
	totalFetches := 4
	if len(opts.OrphanedRepos) > 0 {
		totalFetches = 5
	}

	// Track progress of parallel fetches
	var completedFetches int32

	updateFetchProgress := func() {
		current := atomic.AddInt32(&completedFetches, 1)
		progress := float64(current) / float64(totalFetches)
		msg := fmt.Sprintf("for the past %s (%d/%d sources)", opts.SinceLabel, current, totalFetches)
		sendTaskEvent(opts.Events, tui.TaskFetch, tui.StatusRunning,
			tui.WithProgress(progress),
			tui.WithMessage(msg))
	}

	sendTaskEvent(opts.Events, tui.TaskFetch, tui.StatusRunning,
		tui.WithProgress(0.0),
		tui.WithMessage(fmt.Sprintf("for the past %s (0/%d sources)", opts.SinceLabel, totalFetches)))

	var wg sync.WaitGroup
	result := &fetchResult{}

	// Error tracking
	var notifErr, reviewErr, authoredErr, assignedErr error

	// Fetch notifications (with caching)
	wg.Add(1)
	go func() {
		defer wg.Done()
		notifResult, err := client.ListUnreadNotificationsCached(opts.CurrentUser, opts.Since, cache)
		if err != nil {
			notifErr = err
			updateFetchProgress()
			return
		}
		result.Notifications = notifResult.Notifications
		result.NotifCached = notifResult.FromCache
		result.NotifNewCount = notifResult.NewCount
		updateFetchProgress()
	}()

	// Fetch review-requested PRs
	wg.Add(1)
	go func() {
		defer wg.Done()
		result.ReviewPRs, result.ReviewCached, reviewErr = client.ListReviewRequestedPRsCached(opts.CurrentUser, cache)
		updateFetchProgress()
	}()

	// Fetch authored PRs
	wg.Add(1)
	go func() {
		defer wg.Done()
		result.AuthoredPRs, result.AuthoredCached, authoredErr = client.ListAuthoredPRsCached(opts.CurrentUser, cache)
		updateFetchProgress()
	}()

	// Fetch assigned issues
	wg.Add(1)
	go func() {
		defer wg.Done()
		result.AssignedIssues, result.AssignedCached, assignedErr = client.ListAssignedIssuesCached(opts.CurrentUser, cache)
		updateFetchProgress()
	}()

	// Fetch orphaned contributions (if enabled)
	var orphanedErr error
	if len(opts.OrphanedRepos) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			searchOpts := model.OrphanedSearchOptions{
				Repos:                     opts.OrphanedRepos,
				Since:                     opts.Since,
				StaleDays:                 opts.StaleDays,
				ConsecutiveAuthorComments: opts.ConsecutiveComments,
				MaxPerRepo:                50,
			}
			result.Orphaned, result.OrphanedCached, orphanedErr = client.ListOrphanedContributionsCached(searchOpts, opts.CurrentUser, cache)
			updateFetchProgress()
		}()
	}

	wg.Wait()

	// Handle fetch errors - check for rate limiting
	if notifErr != nil {
		if errors.Is(notifErr, ghclient.ErrRateLimited) {
			result.RateLimited = true
		} else {
			sendTaskEvent(opts.Events, tui.TaskFetch, tui.StatusError, tui.WithError(notifErr))
			return nil, fmt.Errorf("failed to fetch notifications: %w", notifErr)
		}
	}
	var fetchErrs []error
	for _, fe := range []struct {
		err error
		msg string
	}{
		{reviewErr, "review-requested PRs"},
		{authoredErr, "authored PRs"},
		{assignedErr, "assigned issues"},
		{orphanedErr, "orphaned contributions"},
	} {
		if fe.err == nil {
			continue
		}
		if errors.Is(fe.err, ghclient.ErrRateLimited) {
			result.RateLimited = true
			continue
		}
		fetchErrs = append(fetchErrs, fmt.Errorf("%s: %w", fe.msg, fe.err))
	}

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
	sendTaskEvent(opts.Events, tui.TaskFetch, tui.StatusComplete, tui.WithMessage(fetchMsg))

	log.Info("fetched data",
		"notifications", len(result.Notifications),
		"reviewPRs", len(result.ReviewPRs),
		"authoredPRs", len(result.AuthoredPRs),
		"assignedIssues", len(result.AssignedIssues),
		"orphaned", len(result.Orphaned),
		"notifCached", result.NotifCached,
		"notifNewCount", result.NotifNewCount,
		"reviewCached", result.ReviewCached,
		"authoredCached", result.AuthoredCached,
		"assignedCached", result.AssignedCached,
		"orphanedCached", result.OrphanedCached)

	return result, errors.Join(fetchErrs...)
}
