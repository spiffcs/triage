package cmd

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spiffcs/triage/config"
	"github.com/spiffcs/triage/internal/github"
	"github.com/spiffcs/triage/internal/log"
	"github.com/spiffcs/triage/internal/output"
	"github.com/spiffcs/triage/internal/resolved"
	"github.com/spiffcs/triage/internal/triage"
	"github.com/spiffcs/triage/internal/tui"
)

// OrphanedOptions holds the command options for the orphaned command.
type OrphanedOptions struct {
	Repos               []string
	Since               string
	StaleDays           int
	ConsecutiveComments int
	Limit               int
	Format              string
	Verbosity           int
	TUI                 *bool  // nil = auto-detect, true = force TUI, false = disable TUI
	NoDiscover          bool   // Disable automatic repository discovery
	Type                string // Filter by type (pr, issue)
	SortByAge           bool   // Sort by age instead of priority
	OldestFirst         bool   // When sorting by age, show oldest items first
}

// NewCmdOrphaned creates the orphaned command.
func NewCmdOrphaned() *cobra.Command {
	opts := &OrphanedOptions{}

	cmd := &cobra.Command{
		Use:   "orphaned",
		Short: "Find external contributions needing team attention",
		Long: `Searches monitored repositories for PRs and issues from external
contributors that haven't received team engagement.

External contributors are identified by their author association (not MEMBER,
OWNER, or COLLABORATOR). Items are flagged as orphaned when:
  - No team member has responded in the configured number of days, or
  - The author has posted multiple consecutive comments without a response

Examples:
  # Search specific repositories
  triage orphaned --repos anchore/vunnel,anchore/grype

  # Use config file settings
  triage orphaned

  # Customize detection thresholds
  triage orphaned --repos myorg/myrepo --stale-days 14 --consecutive 3`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runOrphaned(opts)
		},
	}

	cmd.Flags().StringSliceVar(&opts.Repos, "repos", nil, "Repositories to monitor (owner/repo)")
	cmd.Flags().StringVar(&opts.Since, "since", "30d", "Look back period for contributions")
	cmd.Flags().IntVar(&opts.StaleDays, "stale-days", 7, "Days without team activity to be considered orphaned")
	cmd.Flags().IntVar(&opts.ConsecutiveComments, "consecutive", 2, "Consecutive author comments without response to be considered orphaned")
	cmd.Flags().IntVar(&opts.Limit, "limit", 50, "Maximum results to display")
	cmd.Flags().StringVarP(&opts.Format, "format", "f", "", "Output format (table, json)")
	cmd.Flags().CountVarP(&opts.Verbosity, "verbose", "v", "Increase verbosity (-v info, -vv debug, -vvv trace)")

	// TUI flag with tri-state: nil = auto, true = force, false = disable
	cmd.Flags().Var(&orphanedTUIFlag{opts: opts}, "tui", "Enable/disable TUI progress (default: auto-detect)")

	// Discovery opt-out flag
	cmd.Flags().BoolVar(&opts.NoDiscover, "no-discover", false, "Disable automatic repository discovery")

	// Type filter
	cmd.Flags().StringVarP(&opts.Type, "type", "t", "", "Filter by type (pr, issue)")

	// Sorting flags
	cmd.Flags().BoolVar(&opts.SortByAge, "sort-by-age", false, "Sort by age (newest first) instead of by priority")
	cmd.Flags().BoolVar(&opts.OldestFirst, "oldest-first", false, "Sort with oldest items first (requires --sort-by-age)")

	return cmd
}

// orphanedTUIFlag implements pflag.Value for the tri-state TUI flag.
type orphanedTUIFlag struct {
	opts *OrphanedOptions
}

func (f *orphanedTUIFlag) String() string {
	if f.opts.TUI == nil {
		return "auto"
	}
	if *f.opts.TUI {
		return "true"
	}
	return "false"
}

func (f *orphanedTUIFlag) Set(s string) error {
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

func (f *orphanedTUIFlag) Type() string {
	return "bool"
}

func (f *orphanedTUIFlag) IsBoolFlag() bool {
	return true
}

func runOrphaned(opts *OrphanedOptions) error {
	// Determine if TUI should be used
	useTUI := shouldUseOrphanedTUI(opts)

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

	repos := opts.Repos
	staleDays := opts.StaleDays
	consecutiveComments := opts.ConsecutiveComments
	limit := opts.Limit

	// Merge command-line repos with config repos
	if len(repos) == 0 && cfg.Orphaned != nil {
		repos = cfg.Orphaned.Repos
	}

	// Create GitHub client early (needed for discovery)
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
		// Use OrphanedTasks since we don't have an enrichment step
		go func() {
			tuiDone <- tui.Run(events, tui.WithTasks(tui.OrphanedTasks()))
		}()
	}

	// Get current user for scoring and discovery
	sendTaskEvent(events, tui.TaskAuth, tui.StatusRunning)
	currentUser, err := ghClient.GetAuthenticatedUser()
	if err != nil {
		sendTaskEvent(events, tui.TaskAuth, tui.StatusError, tui.WithError(err))
		closeTUI(events, tuiDone)
		return fmt.Errorf("failed to get authenticated user: %w", err)
	}
	sendTaskEvent(events, tui.TaskAuth, tui.StatusComplete, tui.WithMessage(currentUser))

	// Auto-discover repos if none specified and discovery is enabled
	if len(repos) == 0 && !opts.NoDiscover && cfg.GetOrphanedAutoDiscover() {
		sendTaskEvent(events, tui.TaskFetch, tui.StatusRunning,
			tui.WithMessage("discovering repositories..."))

		discovered, err := ghClient.DiscoverMaintainableRepos(github.DiscoveryOptions{
			MaxRepos: cfg.GetOrphanedMaxDiscover(),
		})
		if err != nil {
			sendTaskEvent(events, tui.TaskFetch, tui.StatusError, tui.WithError(err))
			closeTUI(events, tuiDone)
			return fmt.Errorf("failed to discover repositories: %w", err)
		}

		if len(discovered) == 0 {
			sendTaskEvent(events, tui.TaskFetch, tui.StatusError,
				tui.WithError(fmt.Errorf("no repositories found")))
			closeTUI(events, tuiDone)
			return fmt.Errorf("no repositories found where you have maintainer access")
		}

		repos = github.RepoNamesToStrings(discovered)
		log.Info("discovered maintainable repositories", "count", len(repos))
	}

	if len(repos) == 0 {
		closeTUI(events, tuiDone)
		return fmt.Errorf("no repositories specified. Use --repos flag, configure orphaned.repos in config, or allow auto-discovery")
	}

	// Apply config defaults if flags weren't explicitly set
	if cfg.Orphaned != nil {
		if staleDays == 7 && cfg.Orphaned.StaleDays > 0 {
			staleDays = cfg.Orphaned.StaleDays
		}
		if consecutiveComments == 2 && cfg.Orphaned.ConsecutiveAuthorComments > 0 {
			consecutiveComments = cfg.Orphaned.ConsecutiveAuthorComments
		}
		if limit == 50 && cfg.Orphaned.MaxItemsPerRepo > 0 {
			limit = cfg.Orphaned.MaxItemsPerRepo * len(repos)
		}
	}

	// Parse since duration
	sinceTime, err := parseDuration(opts.Since)
	if err != nil {
		closeTUI(events, tuiDone)
		return fmt.Errorf("invalid since duration: %w", err)
	}

	log.Info("searching for orphaned contributions",
		"repos", strings.Join(repos, ","),
		"staleDays", staleDays,
		"consecutiveComments", consecutiveComments)

	// Search for orphaned contributions
	sendTaskEvent(events, tui.TaskFetch, tui.StatusRunning,
		tui.WithMessage(fmt.Sprintf("searching %d repos", len(repos))))

	searchOpts := github.OrphanedSearchOptions{
		Repos:                     repos,
		Since:                     sinceTime,
		StaleDays:                 staleDays,
		ConsecutiveAuthorComments: consecutiveComments,
		MaxPerRepo:                limit,
	}

	notifications, err := ghClient.ListOrphanedContributions(searchOpts)
	if err != nil {
		sendTaskEvent(events, tui.TaskFetch, tui.StatusError, tui.WithError(err))
		closeTUI(events, tuiDone)
		return fmt.Errorf("failed to search for orphaned contributions: %w", err)
	}
	sendTaskEvent(events, tui.TaskFetch, tui.StatusComplete,
		tui.WithMessage(fmt.Sprintf("%d items from %d repos", len(notifications), len(repos))))

	if len(notifications) == 0 {
		closeTUI(events, tuiDone)
		fmt.Println("No orphaned contributions found.")
		return nil
	}

	// Score and prioritize
	sendTaskEvent(events, tui.TaskProcess, tui.StatusRunning)

	weights := cfg.GetScoreWeights()
	quickWinLabels := cfg.GetQuickWinLabels()
	engine := triage.NewEngine(currentUser, weights, quickWinLabels)
	sortOpts := triage.SortOptions{
		SortByAge:   opts.SortByAge,
		OldestFirst: opts.OldestFirst,
	}
	items := engine.PrioritizeWithOptions(notifications, sortOpts)

	// If not sorting by age, sort by score descending (orphaned items care more about score than priority)
	if !opts.SortByAge {
		sort.Slice(items, func(i, j int) bool {
			return items[i].Score > items[j].Score
		})
	}

	// Filter by type if specified
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

	// Filter out resolved items (that haven't had new activity)
	if resolvedStore != nil {
		items = triage.FilterResolved(items, resolvedStore)
	}

	// Apply limit
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	sendTaskEvent(events, tui.TaskProcess, tui.StatusComplete, tui.WithCount(len(items)))

	// Close TUI and wait for it to finish before showing output
	closeTUI(events, tuiDone)

	// Determine format
	format := output.Format(opts.Format)

	// If running in a TTY with table format, launch interactive UI
	if shouldUseOrphanedTUI(opts) && (format == "" || format == output.FormatTable) {
		return tui.RunListUI(items, resolvedStore, weights, currentUser, tui.WithHideAssignedCI(), tui.WithHidePriority())
	}

	// Output
	formatter := output.NewFormatterWithWeights(format, weights, currentUser)
	return formatter.Format(items, os.Stdout)
}

// shouldUseOrphanedTUI determines whether to use the TUI based on options and environment.
func shouldUseOrphanedTUI(opts *OrphanedOptions) bool {
	// Disable TUI when verbose logging is requested so logs are visible
	if opts.Verbosity > 0 {
		return false
	}
	if opts.TUI != nil {
		return *opts.TUI
	}
	return tui.ShouldUseTUI()
}

