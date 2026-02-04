package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"
	"github.com/spiffcs/triage/config"
	"github.com/spiffcs/triage/internal/cache"
	"github.com/spiffcs/triage/internal/constants"
	"github.com/spiffcs/triage/internal/duration"
	"github.com/spiffcs/triage/internal/ghclient"
	"github.com/spiffcs/triage/internal/log"
	"github.com/spiffcs/triage/internal/model"
	"github.com/spiffcs/triage/internal/output"
	"github.com/spiffcs/triage/internal/resolved"
	"github.com/spiffcs/triage/internal/service"
	"github.com/spiffcs/triage/internal/triage"
	"github.com/spiffcs/triage/internal/tui"
)

// listRuntime bundles TUI-related state that's threaded through the list command.
type listRuntime struct {
	useTUI  bool
	events  chan tui.Event
	tuiDone chan error
}

// startTUI initializes and starts the TUI goroutine if TUI mode is enabled.
func (rt *listRuntime) startTUI() {
	if !rt.useTUI {
		return
	}
	rt.events = make(chan tui.Event, 100)
	rt.tuiDone = make(chan error, 1)
	go func() {
		rt.tuiDone <- tui.Run(rt.events)
	}()
}

// close closes the event channel and waits for the TUI to finish.
func (rt *listRuntime) close() {
	closeTUI(rt.events, rt.tuiDone)
}

// sendEvent sends a task event to the TUI channel if it exists.
func (rt *listRuntime) sendEvent(task tui.TaskID, status tui.TaskStatus, opts ...tui.TaskEventOption) {
	sendTaskEvent(rt.events, task, status, opts...)
}

// NewCmdList creates the list command.
func NewCmdList(opts *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List prioritized GitHub notifications(same as root triage)",
		Long: `Fetches your unread GitHub notifications, enriches them with
issue/PR details, and displays them sorted by priority.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runList(cmd, opts)
		},
	}

	addListFlags(cmd, opts)
	return cmd
}

// addListFlags adds the list-specific flags to a command.
func addListFlags(cmd *cobra.Command, opts *Options) {
	cmd.Flags().StringVarP(&opts.Format, "output", "o", "", "Output format (table, json)")
	cmd.Flags().StringVarP(&opts.Since, "since", "s", "1w", "Show notifications since (e.g., 1w, 30d, 6mo)")
	cmd.Flags().CountVarP(&opts.Verbosity, "verbose", "v", "Increase verbosity (-v info, -vv debug, -vvv trace)")

	// TUI flag with tri-state: nil = auto, true = force, false = disable
	cmd.Flags().Var(newTUIFlag(opts), "tui", "Enable/disable TUI progress (default: auto-detect)")

	// Profiling flags
	cmd.Flags().StringVar(&opts.CPUProfile, "cpuprofile", "", "Write CPU profile to file")
	cmd.Flags().StringVar(&opts.MemProfile, "memprofile", "", "Write memory profile to file")
	cmd.Flags().StringVar(&opts.Trace, "trace", "", "Write execution trace to file")
}

func runList(cmd *cobra.Command, opts *Options) error {
	ctx := cmd.Context()

	// Setup
	rt, cleanup, err := setupRuntime(opts)
	if err != nil {
		return err
	}
	defer cleanup()
	rt.startTUI()

	// Load config (separate from service)
	cfg, resolvedStore, err := loadConfig()
	if err != nil {
		rt.close()
		return err
	}

	// Create service (combines auth + data pipeline)
	svc, err := initializeService(ctx, cfg, opts.Since, rt)
	if err != nil {
		rt.close()
		return err
	}

	// Fetch
	fetchOpts := buildFetchOptions(cfg, opts.Since, rt.events)
	result, err := fetchAll(ctx, svc, fetchOpts)
	if err != nil {
		log.Warn("some fetches failed", "error", err)
	}

	// Enrich
	runEnrichment(ctx, svc, result, rt)

	// Process
	if len(result.Notifications) == 0 {
		rt.close()
		fmt.Println("No unread notifications, pending reviews, or open PRs found.")
		return nil
	}
	items := processResults(result, cfg, svc.CurrentUser(), resolvedStore, rt.events)

	// Output
	rt.close()
	return renderOutput(items, opts, cfg, svc.CurrentUser(), resolvedStore)
}

// setupRuntime creates the runtime struct and returns a cleanup function for profiling.
func setupRuntime(opts *Options) (*listRuntime, func(), error) {
	profiler := newProfiler(opts.CPUProfile, opts.MemProfile, opts.Trace)
	if err := profiler.Start(); err != nil {
		return nil, nil, err
	}

	useTUI := shouldUseTUI(opts)

	// Initialize logging - suppress logs during TUI to avoid interleaving with display
	if useTUI {
		log.Initialize(opts.Verbosity, io.Discard)
	} else {
		log.Initialize(opts.Verbosity, os.Stderr)
	}

	rt := &listRuntime{useTUI: useTUI}
	return rt, profiler.Stop, nil
}

// loadConfig loads configuration and resolved store.
func loadConfig() (*config.Config, *resolved.Store, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load config: %w", err)
	}

	resolvedStore, err := resolved.NewStore()
	if err != nil {
		log.Warn("could not load resolved store", "error", err)
	}

	return cfg, resolvedStore, nil
}

// initializeService creates the ItemService with user context.
func initializeService(ctx context.Context, cfg *config.Config, sinceStr string, rt *listRuntime) (*service.ItemService, error) {
	since, err := duration.Parse(sinceStr)
	if err != nil {
		return nil, fmt.Errorf("invalid duration: %w", err)
	}

	log.Info("fetching notifications", "since", sinceStr)

	token := cfg.GetGitHubToken()
	if token == "" {
		return nil, fmt.Errorf("GitHub token not configured. Set the GITHUB_TOKEN environment variable")
	}

	ghClient, err := ghclient.NewClient(ctx, token)
	if err != nil {
		return nil, err
	}

	rt.sendEvent(tui.TaskAuth, tui.StatusRunning)
	currentUser, err := ghClient.GetAuthenticatedUser(ctx)
	if err != nil {
		rt.sendEvent(tui.TaskAuth, tui.StatusError, tui.WithError(err))
		return nil, fmt.Errorf("failed to get authenticated user: %w", err)
	}
	rt.sendEvent(tui.TaskAuth, tui.StatusComplete, tui.WithMessage(currentUser))

	c, cacheErr := cache.NewCache()
	if cacheErr != nil {
		log.Warn("failed to initialize cache", "error", cacheErr)
	}

	return service.New(ghClient, c, currentUser, since), nil
}

// buildFetchOptions constructs fetchOptions from config and parameters.
func buildFetchOptions(cfg *config.Config, sinceLabel string, events chan tui.Event) fetchOptions {
	var orphanedRepos []string
	var staleDays, consecutiveComments int
	if cfg.Orphaned != nil {
		orphanedRepos = cfg.Orphaned.Repos
		staleDays = cfg.Orphaned.StaleDays
		consecutiveComments = cfg.Orphaned.ConsecutiveAuthorComments
	}

	return fetchOptions{
		SinceLabel:          sinceLabel,
		Events:              events,
		IncludeOrphaned:     len(orphanedRepos) > 0,
		OrphanedRepos:       orphanedRepos,
		StaleDays:           staleDays,
		ConsecutiveComments: consecutiveComments,
	}
}

// runEnrichment enriches all fetched items and sends TUI events.
func runEnrichment(ctx context.Context, svc *service.ItemService, result *fetchResult, rt *listRuntime) {
	rt.sendEvent(tui.TaskEnrich, tui.StatusRunning)

	totalToEnrich := len(result.Notifications) + len(result.ReviewPRs) + len(result.AuthoredPRs)
	var totalCacheHits int64
	var totalCompleted int64

	if totalToEnrich > 0 {
		enrichItems(ctx, svc, result.Notifications, result.ReviewPRs, result.AuthoredPRs, rt.useTUI, rt.events, totalToEnrich, &totalCompleted, &totalCacheHits)
	}

	enrichCompleteMsg := fmt.Sprintf("%d/%d", totalCompleted, totalToEnrich)
	if totalCacheHits > 0 {
		enrichCompleteMsg = fmt.Sprintf("%d/%d (%d cached)", totalCompleted, totalToEnrich, totalCacheHits)
	}
	rt.sendEvent(tui.TaskEnrich, tui.StatusComplete, tui.WithMessage(enrichCompleteMsg))
}

// processResults merges, prioritizes, and filters the fetched data.
func processResults(result *fetchResult, cfg *config.Config, currentUser string, resolvedStore *resolved.Store, events chan tui.Event) []triage.PrioritizedItem {
	// Merge all additional data sources into notifications
	mergeRes := mergeAll(result)
	if mergeRes.ReviewPRsAdded > 0 {
		log.Info("PRs awaiting your review", "count", mergeRes.ReviewPRsAdded)
	}
	if mergeRes.AuthoredPRsAdded > 0 {
		log.Info("your open PRs", "count", mergeRes.AuthoredPRsAdded)
	}
	if mergeRes.AssignedIssuesAdded > 0 {
		log.Info("issues assigned to you", "count", mergeRes.AssignedIssuesAdded)
	}
	if mergeRes.AssignedPRsAdded > 0 {
		log.Info("PRs assigned to you", "count", mergeRes.AssignedPRsAdded)
	}
	if mergeRes.OrphanedAdded > 0 {
		log.Info("orphaned contributions", "count", mergeRes.OrphanedAdded)
	}

	weights := cfg.GetScoreWeights()
	quickWinLabels := cfg.GetQuickWinLabels()

	sendTaskEvent(events, tui.TaskProcess, tui.StatusRunning)

	// Debug: log state of notifications before prioritization
	var withDetails, withoutDetails int
	for _, n := range result.Notifications {
		if n.Details != nil {
			withDetails++
		} else {
			withoutDetails++
		}
	}
	log.Debug("notifications before prioritization", "total", len(result.Notifications), "withDetails", withDetails, "withoutDetails", withoutDetails)

	engine := triage.NewEngine(currentUser, weights, quickWinLabels)
	items := engine.Prioritize(result.Notifications)
	items = applyFilters(items, cfg, resolvedStore)

	sendTaskEvent(events, tui.TaskProcess, tui.StatusComplete, tui.WithCount(len(items)))
	return items
}

// renderOutput determines the format and outputs the results.
func renderOutput(items []triage.PrioritizedItem, opts *Options, cfg *config.Config, currentUser string, resolvedStore *resolved.Store) error {
	format := output.Format(opts.Format)
	if format == "" {
		format = output.Format(cfg.DefaultFormat)
	}

	// If running in a TTY with table format, launch interactive UI
	if shouldUseTUI(opts) && (format == "" || format == output.FormatTable) {
		weights := cfg.GetScoreWeights()
		blockedLabels := cfg.GetBlockedLabels()
		return tui.RunListUI(items, resolvedStore, weights, currentUser, tui.WithConfig(cfg), tui.WithBlockedLabels(blockedLabels))
	}

	weights := cfg.GetScoreWeights()
	formatter := output.NewFormatterWithWeights(format, weights, currentUser)
	return formatter.Format(items, os.Stdout)
}

// enrichItems enriches notifications and PRs concurrently using the ItemService.
func enrichItems(
	ctx context.Context,
	svc *service.ItemService,
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
			cacheHits, err := svc.Enrich(ctx, notifications, onProgress)
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
			cacheHits, err := svc.Enrich(ctx, reviewPRs, onProgress)
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
			cacheHits, err := svc.Enrich(ctx, authoredPRs, onProgress)
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
func applyFilters(items []triage.PrioritizedItem, cfg *config.Config, resolvedStore *resolved.Store) []triage.PrioritizedItem {
	// Filter out merged and closed items by default
	items = triage.FilterOutMerged(items)
	items = triage.FilterOutClosed(items)

	// Filter out unenriched (inaccessible) items - always applied
	items = triage.FilterOutUnenriched(items)

	// Filter out excluded authors (bots like dependabot, renovate, etc.)
	if len(cfg.ExcludeAuthors) > 0 {
		items = triage.FilterByExcludedAuthors(items, cfg.ExcludeAuthors)
	}

	// Filter out resolved items (that haven't had new activity)
	if resolvedStore != nil {
		items = triage.FilterResolved(items, resolvedStore)
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
