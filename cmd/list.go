package cmd

import (
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"
	"github.com/spiffcs/triage/config"
	"github.com/spiffcs/triage/internal/constants"
	"github.com/spiffcs/triage/internal/duration"
	"github.com/spiffcs/triage/internal/ghclient"
	"github.com/spiffcs/triage/internal/log"
	"github.com/spiffcs/triage/internal/model"
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
	cmd.Flags().StringVarP(&opts.Reason, "reason", "r", "", "Filter by reason ("+model.ItemReasonsString()+")")
	cmd.Flags().StringVar(&opts.Repo, "repo", "", "Filter to specific repo (owner/repo)")
	cmd.Flags().CountVarP(&opts.Verbosity, "verbose", "v", "Increase verbosity (-v info, -vv debug, -vvv trace)")
	cmd.Flags().IntVarP(&opts.Workers, "workers", "w", 20, "Number of concurrent workers for fetching details")
	cmd.Flags().BoolVar(&opts.IncludeMerged, "include-merged", false, "Include notifications for merged PRs")
	cmd.Flags().BoolVar(&opts.IncludeClosed, "include-closed", false, "Include notifications for closed issues/PRs")
	cmd.Flags().BoolVar(&opts.GreenCI, "green-ci", false, "Only show PRs with passing CI status (excludes issues)")
	cmd.Flags().StringVarP(&opts.Type, "type", "t", "", "Filter by type (pr, issue)")

	// TUI flag with tri-state: nil = auto, true = force, false = disable
	cmd.Flags().Var(newTUIFlag(opts), "tui", "Enable/disable TUI progress (default: auto-detect)")

	// Orphaned contribution flags
	cmd.Flags().BoolVar(&opts.SkipOrphaned, "skip-orphaned", false, "Skip fetching orphaned contributions (included by default)")
	cmd.Flags().StringSliceVar(&opts.OrphanedRepos, "orphaned-repos", nil, "Repositories to check for orphaned contributions (owner/repo)")
	cmd.Flags().IntVar(&opts.StaleDays, "stale-days", 7, "Days without team response to be considered orphaned")
	cmd.Flags().IntVar(&opts.ConsecutiveComments, "consecutive", 2, "Consecutive author comments without response to be considered orphaned")
	// Profiling flags
	cmd.Flags().StringVar(&opts.CPUProfile, "cpuprofile", "", "Write CPU profile to file")
	cmd.Flags().StringVar(&opts.MemProfile, "memprofile", "", "Write memory profile to file")
	cmd.Flags().StringVar(&opts.Trace, "trace", "", "Write execution trace to file")
}

func runList(cmd *cobra.Command, _ []string, opts *Options) error {
	ctx := cmd.Context()

	// Set up profiling
	profiler := newProfiler(opts.CPUProfile, opts.MemProfile, opts.Trace)
	if err := profiler.Start(); err != nil {
		return err
	}
	defer profiler.Stop()

	// Determine if TUI should be used
	useTUI := shouldUseTUI(opts)

	// Initialize logging - suppress logs during TUI to avoid interleaving with display
	if useTUI {
		log.Initialize(opts.Verbosity, io.Discard)
	} else {
		log.Initialize(opts.Verbosity, os.Stderr)
	}

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

	ghClient, err := ghclient.NewClient(ctx, token)
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
	since, err := duration.Parse(opts.Since)
	if err != nil {
		closeTUI(events, tuiDone)
		return fmt.Errorf("invalid duration: %w", err)
	}

	log.Info("fetching notifications", "since", opts.Since)

	// Create cache for item storage
	cache, cacheErr := ghclient.NewCache()
	if cacheErr != nil {
		log.Warn("failed to initialize cache", "error", cacheErr)
	}

	// Create ItemStore for cache-aware fetching
	store := ghclient.NewItemStore(ghClient, cache)

	// Create Enricher for GraphQL-based enrichment
	enricher := ghclient.NewEnricher(ghClient, cache)

	// Determine orphaned settings - enabled by default unless explicitly skipped
	orphanedEnabled := !opts.SkipOrphaned
	orphanedRepos := opts.OrphanedRepos
	staleDays := opts.StaleDays
	consecutiveComments := opts.ConsecutiveComments

	// Apply config defaults for orphaned if not set via flags
	if cfg.Orphaned != nil {
		if len(orphanedRepos) == 0 {
			orphanedRepos = cfg.Orphaned.Repos
		}
		if staleDays == 7 && cfg.Orphaned.StaleDays > 0 {
			staleDays = cfg.Orphaned.StaleDays
		}
		if consecutiveComments == 2 && cfg.Orphaned.ConsecutiveAuthorComments > 0 {
			consecutiveComments = cfg.Orphaned.ConsecutiveAuthorComments
		}
	}

	// Fetch all data sources in parallel
	fetchResult, err := fetchAll(ctx, store, fetchOptions{
		Since:               since,
		SinceLabel:          opts.Since,
		CurrentUser:         currentUser,
		Events:              events,
		IncludeOrphaned:     orphanedEnabled && len(orphanedRepos) > 0,
		OrphanedRepos:       orphanedRepos,
		StaleDays:           staleDays,
		ConsecutiveComments: consecutiveComments,
	})
	if err != nil {
		// Log partial fetch failures but continue with results we have
		log.Warn("some fetches failed", "error", err)
	}

	// Enrich all items concurrently
	sendTaskEvent(events, tui.TaskEnrich, tui.StatusRunning)

	totalToEnrich := len(fetchResult.Notifications) + len(fetchResult.ReviewPRs) + len(fetchResult.AuthoredPRs)
	var totalCacheHits int64
	var totalCompleted int64

	if totalToEnrich > 0 {
		enrichItems(enricher, fetchResult.Notifications, fetchResult.ReviewPRs, fetchResult.AuthoredPRs, useTUI, events, totalToEnrich, &totalCompleted, &totalCacheHits)
	}

	enrichCompleteMsg := fmt.Sprintf("%d/%d", totalCompleted, totalToEnrich)
	if totalCacheHits > 0 {
		enrichCompleteMsg = fmt.Sprintf("%d/%d (%d cached)", totalCompleted, totalToEnrich, totalCacheHits)
	}
	sendTaskEvent(events, tui.TaskEnrich, tui.StatusComplete, tui.WithMessage(enrichCompleteMsg))

	// Merge all additional data sources into notifications
	mergeRes := mergeAll(fetchResult)
	if mergeRes.ReviewPRsAdded > 0 {
		log.Info("PRs awaiting your review", "count", mergeRes.ReviewPRsAdded)
	}
	if mergeRes.AuthoredPRsAdded > 0 {
		log.Info("your open PRs", "count", mergeRes.AuthoredPRsAdded)
	}
	if mergeRes.AssignedIssuesAdded > 0 {
		log.Info("issues assigned to you", "count", mergeRes.AssignedIssuesAdded)
	}
	if mergeRes.OrphanedAdded > 0 {
		log.Info("orphaned contributions", "count", mergeRes.OrphanedAdded)
	}

	if len(fetchResult.Notifications) == 0 {
		closeTUI(events, tuiDone)
		fmt.Println("No unread notifications, pending reviews, or open PRs found.")
		return nil
	}

	// Get score weights and quick win labels (with any user overrides)
	weights := cfg.GetScoreWeights()
	quickWinLabels := cfg.GetQuickWinLabels()

	// Process results: score and filter
	sendTaskEvent(events, tui.TaskProcess, tui.StatusRunning)

	// Debug: log state of notifications before prioritization
	var withDetails, withoutDetails int
	for _, n := range fetchResult.Notifications {
		if n.Details != nil {
			withDetails++
		} else {
			withoutDetails++
		}
	}
	log.Debug("notifications before prioritization", "total", len(fetchResult.Notifications), "withDetails", withDetails, "withoutDetails", withoutDetails)

	// Prioritize
	engine := triage.NewEngine(currentUser, weights, quickWinLabels)
	items := engine.Prioritize(fetchResult.Notifications)

	// Apply filters
	items = applyFilters(items, opts, cfg, resolvedStore)

	sendTaskEvent(events, tui.TaskProcess, tui.StatusComplete, tui.WithCount(len(items)))

	// Close TUI and wait for it to finish before showing output
	closeTUI(events, tuiDone)

	// Determine format
	format := output.Format(opts.Format)
	if format == "" {
		format = output.Format(cfg.DefaultFormat)
	}

	// If running in a TTY with table format, launch interactive UI
	// Use shouldUseTUI to respect verbose mode and --tui flag
	if shouldUseTUI(opts) && (format == "" || format == output.FormatTable) {
		return tui.RunListUI(items, resolvedStore, weights, currentUser, tui.WithConfig(cfg))
	}

	// Output
	formatter := output.NewFormatterWithWeights(format, weights, currentUser)

	return formatter.Format(items, os.Stdout)
}

// enrichItems enriches notifications and PRs concurrently using the Enricher.
func enrichItems(
	enricher *ghclient.Enricher,
	notifications, reviewPRs, authoredPRs []model.Item,
	useTUI bool,
	events chan tui.Event,
	totalToEnrich int,
	totalCompleted, totalCacheHits *int64,
) {
	// Progress callback using atomic counter for concurrent updates
	var lastLogPercent int64 = -1
	var lastTUIUpdate int64 = 0 // Unix nanoseconds
	tuiUpdateInterval := int64(constants.TUIUpdateInterval)

	onProgress := func(delta int, _ int) {
		completed := atomic.AddInt64(totalCompleted, int64(delta))
		cacheHits := atomic.LoadInt64(totalCacheHits)

		if useTUI {
			// Throttle TUI updates to every 50ms for smooth progress without overhead
			now := time.Now().UnixNano()
			lastUpdate := atomic.LoadInt64(&lastTUIUpdate)
			if now-lastUpdate >= tuiUpdateInterval || completed == int64(totalToEnrich) {
				if atomic.CompareAndSwapInt64(&lastTUIUpdate, lastUpdate, now) {
					progress := float64(completed) / float64(totalToEnrich)
					msg := fmt.Sprintf("%d/%d", completed, totalToEnrich)
					if cacheHits > 0 {
						msg = fmt.Sprintf("%d/%d (%d cached)", completed, totalToEnrich, cacheHits)
					}
					sendTaskEvent(events, tui.TaskEnrich, tui.StatusRunning,
						tui.WithProgress(progress),
						tui.WithMessage(msg))
				}
			}
		} else {
			// Throttle log output to configured percent intervals
			percent := (completed * 100) / int64(totalToEnrich)
			if percent != atomic.LoadInt64(&lastLogPercent) && percent%constants.LogThrottlePercent == 0 {
				atomic.StoreInt64(&lastLogPercent, percent)
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
			cacheHits, err := enricher.Enrich(notifications, onProgress)
			if err != nil {
				log.Warn("some notifications could not be enriched", "error", err)
			}
			atomic.AddInt64(totalCacheHits, int64(cacheHits))
		}()
	}

	// Enrich review-requested PRs
	if len(reviewPRs) > 0 {
		enrichWg.Add(1)
		go func() {
			defer enrichWg.Done()
			cacheHits, err := enricher.Enrich(reviewPRs, onProgress)
			if err != nil {
				log.Warn("some review PRs could not be enriched", "error", err)
			}
			atomic.AddInt64(totalCacheHits, int64(cacheHits))
		}()
	}

	// Enrich authored PRs
	if len(authoredPRs) > 0 {
		enrichWg.Add(1)
		go func() {
			defer enrichWg.Done()
			cacheHits, err := enricher.Enrich(authoredPRs, onProgress)
			if err != nil {
				log.Warn("some authored PRs could not be enriched", "error", err)
			}
			atomic.AddInt64(totalCacheHits, int64(cacheHits))
		}()
	}

	enrichWg.Wait()

	if !useTUI {
		log.ProgressDone()
	}
}

// applyFilters applies all configured filters to the items.
func applyFilters(items []triage.PrioritizedItem, opts *Options, cfg *config.Config, resolvedStore *resolved.Store) []triage.PrioritizedItem {
	if !opts.IncludeMerged {
		items = triage.FilterOutMerged(items)
	}
	if !opts.IncludeClosed {
		items = triage.FilterOutClosed(items)
	}

	// Filter out unenriched (inaccessible) items - always applied
	items = triage.FilterOutUnenriched(items)

	if opts.Priority != "" {
		items = triage.FilterByPriority(items, triage.PriorityLevel(opts.Priority))
	}

	if opts.Reason != "" {
		items = triage.FilterByReason(items, []model.ItemReason{model.ItemReason(opts.Reason)})
	}

	if opts.Repo != "" {
		items = triage.FilterByRepo(items, opts.Repo)
	}

	if opts.Type != "" {
		var subjectType model.SubjectType
		switch opts.Type {
		case "pr", "PR", "pullrequest", "PullRequest":
			subjectType = model.SubjectPullRequest
		case "issue", "Issue":
			subjectType = model.SubjectIssue
		}
		if subjectType != "" {
			items = triage.FilterByType(items, subjectType)
		}
	}

	// Filter out excluded authors (bots like dependabot, renovate, etc.)
	if len(cfg.ExcludeAuthors) > 0 {
		items = triage.FilterByExcludedAuthors(items, cfg.ExcludeAuthors)
	}

	// Filter to only show PRs with green CI
	if opts.GreenCI {
		items = triage.FilterByGreenCI(items)
	}

	// Filter out resolved items (that haven't had new activity)
	if resolvedStore != nil {
		items = triage.FilterResolved(items, resolvedStore)
	}

	// Apply limit
	if opts.Limit > 0 && len(items) > opts.Limit {
		items = items[:opts.Limit]
	}

	return items
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
