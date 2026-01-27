package cmd

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"
	"github.com/spiffcs/triage/config"
	"github.com/spiffcs/triage/internal/github"
	"github.com/spiffcs/triage/internal/log"
	"github.com/spiffcs/triage/internal/output"
	"github.com/spiffcs/triage/internal/resolved"
	"github.com/spiffcs/triage/internal/triage"
	"github.com/spiffcs/triage/internal/tui"
)

// NewCmdList creates the list command.
func NewCmdList(opts *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List prioritized GitHub notifications(same as root triage)",
		Long: `Fetches your unread GitHub notifications, enriches them with
issue/PR details, and displays them sorted by priority.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, args, opts)
		},
	}

	addListFlags(cmd, opts)
	return cmd
}

// addListFlags adds the list-specific flags to a command.
func addListFlags(cmd *cobra.Command, opts *Options) {
	cmd.Flags().StringVarP(&opts.Format, "format", "f", "", "Output format (table, json)")
	cmd.Flags().IntVarP(&opts.Limit, "limit", "l", 0, "Limit number of results")
	cmd.Flags().StringVarP(&opts.Since, "since", "s", "1w", "Show notifications since (e.g., 1w, 30d, 6mo)")
	cmd.Flags().StringVarP(&opts.Priority, "priority", "p", "", "Filter by priority (urgent, important, quick-win, notable, fyi)")
	cmd.Flags().StringVarP(&opts.Reason, "reason", "r", "", "Filter by reason (mention, review_requested, author, etc.)")
	cmd.Flags().StringVar(&opts.Repo, "repo", "", "Filter to specific repo (owner/repo)")
	cmd.Flags().CountVarP(&opts.Verbosity, "verbose", "v", "Increase verbosity (-v info, -vv debug, -vvv trace)")
	cmd.Flags().IntVarP(&opts.Workers, "workers", "w", 20, "Number of concurrent workers for fetching details")
	cmd.Flags().BoolVar(&opts.IncludeMerged, "include-merged", false, "Include notifications for merged PRs")
	cmd.Flags().BoolVar(&opts.IncludeClosed, "include-closed", false, "Include notifications for closed issues/PRs")
	cmd.Flags().StringVarP(&opts.Type, "type", "t", "", "Filter by type (pr, issue)")

	// TUI flag with tri-state: nil = auto, true = force, false = disable
	cmd.Flags().Var(&tuiFlag{opts: opts}, "tui", "Enable/disable TUI progress (default: auto-detect)")
}

// tuiFlag implements pflag.Value for the tri-state TUI flag.
type tuiFlag struct {
	opts *Options
}

func (f *tuiFlag) String() string {
	if f.opts.TUI == nil {
		return "auto"
	}
	if *f.opts.TUI {
		return "true"
	}
	return "false"
}

func (f *tuiFlag) Set(s string) error {
	switch s {
	case "true", "1", "yes":
		v := true
		f.opts.TUI = &v
	case "false", "0", "no":
		v := false
		f.opts.TUI = &v
	case "auto":
		f.opts.TUI = nil
	default:
		return fmt.Errorf("invalid value %q: use true, false, or auto", s)
	}
	return nil
}

func (f *tuiFlag) Type() string {
	return "bool"
}

func (f *tuiFlag) IsBoolFlag() bool {
	return true
}

func runList(_ *cobra.Command, _ []string, opts *Options) error {
	// Determine if TUI should be used
	useTUI := shouldUseTUI(opts)

	// Initialize logging (suppress progress output if TUI is active)
	log.Initialize(opts.Verbosity, os.Stderr)

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Load resolved items store
	resolvedStore, err := resolved.NewStore()
	if err != nil {
		log.Warn("could not load resolved store", "error", err)
	}

	// Create GitHub client
	token := cfg.GetGitHubToken()
	if token == "" {
		return fmt.Errorf("GitHub token not configured. Set the GITHUB_TOKEN environment variable")
	}

	ghClient, err := github.NewClient(token)
	if err != nil {
		return err
	}

	// Set up TUI event channel if using TUI
	var events chan tui.Event
	var tuiDone chan error
	if useTUI {
		events = make(chan tui.Event, 100)
		tuiDone = make(chan error, 1)
		// Start TUI immediately so it can show auth progress
		go func() {
			tuiDone <- tui.Run(events)
		}()
	}

	// Get current user for heuristics
	sendTaskEvent(events, tui.TaskAuth, tui.StatusRunning)
	currentUser, err := ghClient.GetAuthenticatedUser()
	if err != nil {
		sendTaskEvent(events, tui.TaskAuth, tui.StatusError, tui.WithError(err))
		closeTUI(events, tuiDone)
		return fmt.Errorf("failed to get authenticated user: %w", err)
	}
	sendTaskEvent(events, tui.TaskAuth, tui.StatusComplete, tui.WithMessage(currentUser))

	// Parse since duration
	since, err := parseDuration(opts.Since)
	if err != nil {
		closeTUI(events, tuiDone)
		return fmt.Errorf("invalid duration: %w", err)
	}

	log.Info("fetching notifications", "since", opts.Since)

	// Create cache for PR lists
	prCache, cacheErr := github.NewCache()
	if cacheErr != nil {
		log.Warn("failed to initialize cache", "error", cacheErr)
	}

	// Parallel fetch all data sources
	type fetchResult struct {
		notifications  []github.Notification
		reviewPRs      []github.Notification
		authoredPRs    []github.Notification
		assignedIssues []github.Notification
		notifErr       error
		reviewErr      error
		authoredErr    error
		assignedErr    error
		notifCached    bool
		notifNewCount  int
		reviewCached   bool
		authoredCached bool
		assignedCached bool
	}

	// Track progress of parallel fetches
	var completedFetches int32
	const totalFetches = 4

	updateFetchProgress := func() {
		current := atomic.AddInt32(&completedFetches, 1)
		progress := float64(current) / float64(totalFetches)
		msg := fmt.Sprintf("for the past %s (%d/%d sources)", opts.Since, current, totalFetches)
		sendTaskEvent(events, tui.TaskFetch, tui.StatusRunning,
			tui.WithProgress(progress),
			tui.WithMessage(msg))
	}

	sendTaskEvent(events, tui.TaskFetch, tui.StatusRunning,
		tui.WithProgress(0.0),
		tui.WithMessage(fmt.Sprintf("for the past %s (0/%d sources)", opts.Since, totalFetches)))

	var wg sync.WaitGroup
	result := &fetchResult{}

	// Fetch notifications (with caching)
	wg.Add(1)
	go func() {
		defer wg.Done()
		notifResult, err := ghClient.ListUnreadNotificationsCached(currentUser, since, prCache)
		if err != nil {
			result.notifErr = err
			updateFetchProgress()
			return
		}
		result.notifications = notifResult.Notifications
		result.notifCached = notifResult.FromCache
		result.notifNewCount = notifResult.NewCount
		updateFetchProgress()
	}()

	// Fetch review-requested PRs
	wg.Add(1)
	go func() {
		defer wg.Done()
		result.reviewPRs, result.reviewCached, result.reviewErr = ghClient.ListReviewRequestedPRsCached(currentUser, prCache)
		updateFetchProgress()
	}()

	// Fetch authored PRs
	wg.Add(1)
	go func() {
		defer wg.Done()
		result.authoredPRs, result.authoredCached, result.authoredErr = ghClient.ListAuthoredPRsCached(currentUser, prCache)
		updateFetchProgress()
	}()

	// Fetch assigned issues
	wg.Add(1)
	go func() {
		defer wg.Done()
		result.assignedIssues, result.assignedCached, result.assignedErr = ghClient.ListAssignedIssuesCached(currentUser, prCache)
		updateFetchProgress()
	}()

	wg.Wait()

	// Handle fetch errors
	if result.notifErr != nil {
		sendTaskEvent(events, tui.TaskFetch, tui.StatusError, tui.WithError(result.notifErr))
		closeTUI(events, tuiDone)
		return fmt.Errorf("failed to fetch notifications: %w", result.notifErr)
	}
	if result.reviewErr != nil {
		log.Warn("failed to fetch review-requested PRs", "error", result.reviewErr)
	}
	if result.authoredErr != nil {
		log.Warn("failed to fetch authored PRs", "error", result.authoredErr)
	}
	if result.assignedErr != nil {
		log.Warn("failed to fetch assigned issues", "error", result.assignedErr)
	}

	notifications := result.notifications
	reviewPRs := result.reviewPRs
	authoredPRs := result.authoredPRs
	assignedIssues := result.assignedIssues

	totalFetched := len(notifications) + len(reviewPRs) + len(authoredPRs) + len(assignedIssues)
	fetchMsg := fmt.Sprintf("for the past %s (%d items)", opts.Since, totalFetched)
	if result.notifCached && result.notifNewCount > 0 {
		fetchMsg = fmt.Sprintf("for the past %s (%d items, %d new)", opts.Since, totalFetched, result.notifNewCount)
	} else if result.notifCached {
		fetchMsg = fmt.Sprintf("for the past %s (%d items, cached)", opts.Since, totalFetched)
	}
	sendTaskEvent(events, tui.TaskFetch, tui.StatusComplete, tui.WithMessage(fetchMsg))

	log.Info("fetched data",
		"notifications", len(notifications),
		"reviewPRs", len(reviewPRs),
		"authoredPRs", len(authoredPRs),
		"assignedIssues", len(assignedIssues),
		"notifCached", result.notifCached,
		"notifNewCount", result.notifNewCount,
		"reviewCached", result.reviewCached,
		"authoredCached", result.authoredCached,
		"assignedCached", result.assignedCached)

	// Enrich all items concurrently
	sendTaskEvent(events, tui.TaskEnrich, tui.StatusRunning)

	// Track total items to enrich
	totalToEnrich := len(notifications) + len(reviewPRs) + len(authoredPRs)
	var totalCacheHits int64
	var totalCompleted int64

	if totalToEnrich > 0 {
		// Progress callback using atomic counter for concurrent updates
		var lastPercent int64 = -1
		onProgress := func(_ int, _ int) {
			completed := atomic.AddInt64(&totalCompleted, 1)
			percent := (completed * 100) / int64(totalToEnrich)
			// Only update at 5% intervals to reduce UI updates
			if percent != atomic.LoadInt64(&lastPercent) && percent%5 == 0 {
				atomic.StoreInt64(&lastPercent, percent)
				cacheHits := atomic.LoadInt64(&totalCacheHits)
				if useTUI {
					progress := float64(completed) / float64(totalToEnrich)
					msg := fmt.Sprintf("%d/%d", completed, totalToEnrich)
					if cacheHits > 0 {
						msg = fmt.Sprintf("%d/%d (%d cached)", completed, totalToEnrich, cacheHits)
					}
					sendTaskEvent(events, tui.TaskEnrich, tui.StatusRunning,
						tui.WithProgress(progress),
						tui.WithMessage(msg))
				} else {
					log.Progress("Enriching items: %d/%d (%d%%)...", completed, totalToEnrich, percent)
				}
			}
		}

		// Enrich all three sources concurrently
		var enrichWg sync.WaitGroup

		// Enrich notifications
		if len(notifications) > 0 {
			enrichWg.Add(1)
			go func() {
				defer enrichWg.Done()
				cacheHits, err := ghClient.EnrichNotificationsConcurrent(notifications, opts.Workers, onProgress)
				if err != nil {
					log.Warn("some notifications could not be enriched", "error", err)
				}
				atomic.AddInt64(&totalCacheHits, int64(cacheHits))
			}()
		}

		// Enrich review-requested PRs
		if len(reviewPRs) > 0 {
			enrichWg.Add(1)
			go func() {
				defer enrichWg.Done()
				cacheHits, err := ghClient.EnrichPRsConcurrent(reviewPRs, opts.Workers, prCache, onProgress)
				if err != nil {
					log.Warn("some review PRs could not be enriched", "error", err)
				}
				atomic.AddInt64(&totalCacheHits, int64(cacheHits))
			}()
		}

		// Enrich authored PRs
		if len(authoredPRs) > 0 {
			enrichWg.Add(1)
			go func() {
				defer enrichWg.Done()
				cacheHits, err := ghClient.EnrichPRsConcurrent(authoredPRs, opts.Workers, prCache, onProgress)
				if err != nil {
					log.Warn("some authored PRs could not be enriched", "error", err)
				}
				atomic.AddInt64(&totalCacheHits, int64(cacheHits))
			}()
		}

		enrichWg.Wait()

		if !useTUI {
			log.ProgressDone()
		}
	}

	enrichCompleteMsg := fmt.Sprintf("%d/%d", totalCompleted, totalToEnrich)
	if totalCacheHits > 0 {
		enrichCompleteMsg = fmt.Sprintf("%d/%d (%d cached)", totalCompleted, totalToEnrich, totalCacheHits)
	}
	sendTaskEvent(events, tui.TaskEnrich, tui.StatusComplete,
		tui.WithMessage(enrichCompleteMsg))

	// Merge review-requested and authored PRs into notifications
	if len(reviewPRs) > 0 {
		var added int
		notifications, added = mergeReviewRequests(notifications, reviewPRs)
		if added > 0 {
			log.Info("PRs awaiting your review", "count", added)
		}
	}
	if len(authoredPRs) > 0 {
		var added int
		notifications, added = mergeAuthoredPRs(notifications, authoredPRs)
		if added > 0 {
			log.Info("your open PRs", "count", added)
		}
	}
	if len(assignedIssues) > 0 {
		var added int
		notifications, added = mergeAssignedIssues(notifications, assignedIssues)
		if added > 0 {
			log.Info("issues assigned to you", "count", added)
		}
	}

	if len(notifications) == 0 {
		closeTUI(events, tuiDone)
		fmt.Println("No unread notifications, pending reviews, or open PRs found.")
		return nil
	}

	// Get score weights and quick win labels (with any user overrides)
	weights := cfg.GetScoreWeights()
	quickWinLabels := cfg.GetQuickWinLabels()

	// Process results: score and filter
	sendTaskEvent(events, tui.TaskProcess, tui.StatusRunning)

	// Prioritize
	engine := triage.NewEngine(currentUser, weights, quickWinLabels)
	items := engine.Prioritize(notifications)

	// Apply filters
	if !opts.IncludeMerged {
		items = triage.FilterOutMerged(items)
	}
	if !opts.IncludeClosed {
		items = triage.FilterOutClosed(items)
	}

	if opts.Priority != "" {
		items = triage.FilterByPriority(items, triage.PriorityLevel(opts.Priority))
	}

	if opts.Reason != "" {
		items = triage.FilterByReason(items, []github.NotificationReason{github.NotificationReason(opts.Reason)})
	}

	if opts.Repo != "" {
		items = triage.FilterByRepo(items, opts.Repo)
	}

	if opts.Type != "" {
		var subjectType github.SubjectType
		switch opts.Type {
		case "pr", "PR", "pullrequest", "PullRequest":
			subjectType = github.SubjectPullRequest
		case "issue", "Issue":
			subjectType = github.SubjectIssue
		default:
			closeTUI(events, tuiDone)
			return fmt.Errorf("invalid type: %s (must be 'pr' or 'issue')", opts.Type)
		}
		items = triage.FilterByType(items, subjectType)
	}

	// Filter out excluded authors (bots like dependabot, renovate, etc.)
	if len(cfg.ExcludeAuthors) > 0 {
		items = triage.FilterByExcludedAuthors(items, cfg.ExcludeAuthors)
	}

	// Filter out resolved items (that haven't had new activity)
	if resolvedStore != nil {
		items = triage.FilterResolved(items, resolvedStore)
	}

	// Apply limit
	if opts.Limit > 0 && len(items) > opts.Limit {
		items = items[:opts.Limit]
	}
	sendTaskEvent(events, tui.TaskProcess, tui.StatusComplete, tui.WithCount(len(items)))

	// Close TUI and wait for it to finish before showing output
	closeTUI(events, tuiDone)

	// Determine format
	format := output.Format(opts.Format)
	if format == "" {
		format = output.Format(cfg.DefaultFormat)
	}

	// If running in a TTY with table format, launch interactive UI
	if tui.ShouldUseTUI() && (format == "" || format == output.FormatTable) {
		return tui.RunListUI(items, resolvedStore, weights, currentUser)
	}

	// Output
	formatter := output.NewFormatterWithWeights(format, weights, currentUser)

	return formatter.Format(items, os.Stdout)
}

// shouldUseTUI determines whether to use the TUI based on options and environment.
func shouldUseTUI(opts *Options) bool {
	if opts.TUI != nil {
		return *opts.TUI
	}
	return tui.ShouldUseTUI()
}

// sendTaskEvent sends a task event to the TUI channel if it exists.
func sendTaskEvent(events chan tui.Event, task tui.TaskID, status tui.TaskStatus, opts ...tui.TaskEventOption) {
	if events == nil {
		return
	}
	tui.SendTaskEvent(events, task, status, opts...)
}

// closeTUI closes the event channel and waits for the TUI to finish.
func closeTUI(events chan tui.Event, tuiDone chan error) {
	if events == nil {
		return
	}
	close(events)
	if tuiDone != nil {
		<-tuiDone
	}
}

func parseDuration(s string) (time.Time, error) {
	now := time.Now()

	// Handle common patterns
	var duration time.Duration
	var n int
	var unit string

	if _, err := fmt.Sscanf(s, "%d%s", &n, &unit); err != nil {
		return time.Time{}, fmt.Errorf("invalid duration format: %s (use e.g., 1w, 30d, 6mo)", s)
	}

	switch unit {
	case "m", "min", "mins":
		duration = time.Duration(n) * time.Minute
	case "h", "hr", "hrs", "hour", "hours":
		duration = time.Duration(n) * time.Hour
	case "d", "day", "days":
		duration = time.Duration(n) * 24 * time.Hour
	case "w", "wk", "wks", "week", "weeks":
		duration = time.Duration(n) * 7 * 24 * time.Hour
	case "mo", "month", "months":
		duration = time.Duration(n) * 30 * 24 * time.Hour
	case "y", "yr", "yrs", "year", "years":
		duration = time.Duration(n) * 365 * 24 * time.Hour
	default:
		return time.Time{}, fmt.Errorf("unknown duration unit: %s", unit)
	}

	return now.Add(-duration), nil
}

// mergeReviewRequests adds review-requested PRs that aren't already in notifications
// Returns the merged list and the count of newly added items
func mergeReviewRequests(notifications []github.Notification, reviewPRs []github.Notification) ([]github.Notification, int) {
	// Build a set of existing PR identifiers (repo/number and also Subject.URL for fallback)
	existing := make(map[string]bool)
	existingURLs := make(map[string]bool)

	for _, n := range notifications {
		if n.Subject.Type == github.SubjectPullRequest {
			// Track by URL (always available)
			if n.Subject.URL != "" {
				existingURLs[n.Subject.URL] = true
			}
			// Track by repo#number if Details available
			if n.Details != nil {
				key := fmt.Sprintf("%s#%d", n.Repository.FullName, n.Details.Number)
				existing[key] = true
			}
		}
	}

	// Add review PRs that aren't already in the list
	added := 0
	for _, pr := range reviewPRs {
		if pr.Details == nil {
			continue
		}

		// Check both key formats to avoid duplicates
		key := fmt.Sprintf("%s#%d", pr.Repository.FullName, pr.Details.Number)
		if existing[key] || existingURLs[pr.Subject.URL] {
			continue
		}

		notifications = append(notifications, pr)
		existing[key] = true
		added++
	}

	return notifications, added
}

// mergeAuthoredPRs adds user's open PRs that aren't already in notifications
// Returns the merged list and the count of newly added items
func mergeAuthoredPRs(notifications []github.Notification, authoredPRs []github.Notification) ([]github.Notification, int) {
	// Build a set of existing PR identifiers
	existing := make(map[string]bool)
	existingURLs := make(map[string]bool)

	for _, n := range notifications {
		if n.Subject.Type == github.SubjectPullRequest {
			if n.Subject.URL != "" {
				existingURLs[n.Subject.URL] = true
			}
			if n.Details != nil {
				key := fmt.Sprintf("%s#%d", n.Repository.FullName, n.Details.Number)
				existing[key] = true
			}
		}
	}

	// Add authored PRs that aren't already in the list
	added := 0
	for _, pr := range authoredPRs {
		if pr.Details == nil {
			continue
		}

		key := fmt.Sprintf("%s#%d", pr.Repository.FullName, pr.Details.Number)
		if existing[key] || existingURLs[pr.Subject.URL] {
			continue
		}

		notifications = append(notifications, pr)
		existing[key] = true
		added++
	}

	return notifications, added
}

// mergeAssignedIssues adds user's assigned issues that aren't already in notifications
// Returns the merged list and the count of newly added items
func mergeAssignedIssues(notifications []github.Notification, assignedIssues []github.Notification) ([]github.Notification, int) {
	// Build a set of existing issue identifiers
	existing := make(map[string]bool)
	existingURLs := make(map[string]bool)

	for _, n := range notifications {
		if n.Subject.Type == github.SubjectIssue {
			if n.Subject.URL != "" {
				existingURLs[n.Subject.URL] = true
			}
			if n.Details != nil {
				key := fmt.Sprintf("%s#%d", n.Repository.FullName, n.Details.Number)
				existing[key] = true
			}
		}
	}

	// Add assigned issues that aren't already in the list
	added := 0
	for _, issue := range assignedIssues {
		if issue.Details == nil {
			continue
		}

		key := fmt.Sprintf("%s#%d", issue.Repository.FullName, issue.Details.Number)
		if existing[key] || existingURLs[issue.Subject.URL] {
			continue
		}

		notifications = append(notifications, issue)
		existing[key] = true
		added++
	}

	return notifications, added
}
