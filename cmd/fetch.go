package cmd

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spiffcs/triage/internal/github"
	"github.com/spiffcs/triage/internal/log"
	"github.com/spiffcs/triage/internal/tui"
)

// handleFetchError processes a fetch error by either setting the rate limit flag
// or logging a warning. It follows the guard clause pattern for early returns.
func handleFetchError(err error, msg string, rateLimited *bool) {
	if err == nil {
		return
	}
	if errors.Is(err, github.ErrRateLimited) {
		*rateLimited = true
		return
	}
	log.Warn(msg, "error", err)
}

// FetchResult contains all data fetched from GitHub.
type FetchResult struct {
	Notifications  []github.Notification
	ReviewPRs      []github.Notification
	AuthoredPRs    []github.Notification
	AssignedIssues []github.Notification
	RateLimited    bool

	// Cache statistics
	NotifCached    bool
	NotifNewCount  int
	ReviewCached   bool
	AuthoredCached bool
	AssignedCached bool
}

// FetchCacheStats returns a summary of cache usage.
type FetchCacheStats struct {
	NotifCached    bool
	NotifNewCount  int
	ReviewCached   bool
	AuthoredCached bool
	AssignedCached bool
}

// CacheStats returns the cache statistics from the fetch result.
func (r *FetchResult) CacheStats() FetchCacheStats {
	return FetchCacheStats{
		NotifCached:    r.NotifCached,
		NotifNewCount:  r.NotifNewCount,
		ReviewCached:   r.ReviewCached,
		AuthoredCached: r.AuthoredCached,
		AssignedCached: r.AssignedCached,
	}
}

// TotalFetched returns the total number of items fetched.
func (r *FetchResult) TotalFetched() int {
	return len(r.Notifications) + len(r.ReviewPRs) + len(r.AuthoredPRs) + len(r.AssignedIssues)
}

// FetchOptions configures the fetch operation.
type FetchOptions struct {
	Since       time.Time
	SinceLabel  string // Human-readable label like "1w"
	CurrentUser string
	Events      chan tui.Event
}

// FetchAll fetches all data sources in parallel.
func FetchAll(ctx context.Context, client *github.Client, cache *github.Cache, opts FetchOptions) (*FetchResult, error) {
	const totalFetches = 4

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
	result := &FetchResult{}

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

	wg.Wait()

	// Handle fetch errors - check for rate limiting
	if notifErr != nil {
		if errors.Is(notifErr, github.ErrRateLimited) {
			result.RateLimited = true
		} else {
			sendTaskEvent(opts.Events, tui.TaskFetch, tui.StatusError, tui.WithError(notifErr))
			return nil, fmt.Errorf("failed to fetch notifications: %w", notifErr)
		}
	}
	handleFetchError(reviewErr, "failed to fetch review-requested PRs", &result.RateLimited)
	handleFetchError(authoredErr, "failed to fetch authored PRs", &result.RateLimited)
	handleFetchError(assignedErr, "failed to fetch assigned issues", &result.RateLimited)

	// Send rate limit event to TUI if rate limited
	if result.RateLimited && opts.Events != nil {
		_, _, resetAt, _ := github.GetRateLimitStatus()
		opts.Events <- tui.RateLimitEvent{
			Limited: true,
			ResetAt: resetAt,
		}
	}

	totalFetched := result.TotalFetched()
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
		"notifCached", result.NotifCached,
		"notifNewCount", result.NotifNewCount,
		"reviewCached", result.ReviewCached,
		"authoredCached", result.AuthoredCached,
		"assignedCached", result.AssignedCached)

	return result, nil
}
