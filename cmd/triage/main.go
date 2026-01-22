package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/hal/triage/config"
	"github.com/hal/triage/internal/github"
	"github.com/hal/triage/internal/output"
	"github.com/hal/triage/internal/triage"
)

var (
	// Global flags
	formatFlag    string
	limitFlag     int
	sinceFlag     string
	priorityFlag  string
	reasonFlag    string
	repoFlag      string
	typeFlag      string
	verboseFlag   bool
	workersFlag   int
	includeMerged bool
	includeClosed bool
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "triage",
	Short: "GitHub notification triage manager",
	Long: `A CLI tool that analyzes your GitHub notifications to help you
triage your work. It uses heuristics to score notifications.`,
	RunE: runList,
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List prioritized GitHub notifications",
	Long: `Fetches your unread GitHub notifications, enriches them with
issue/PR details, and displays them sorted by priority.`,
	RunE: runList,
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Long: `Set a configuration value. Available keys:
  format      - Default output format (table, json)`,
	Args: cobra.ExactArgs(2),
	RunE: runConfigSet,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	RunE:  runConfigShow,
}

var markReadCmd = &cobra.Command{
	Use:   "mark-read <notification-id>",
	Short: "Mark a notification as read",
	Args:  cobra.ExactArgs(1),
	RunE:  runMarkRead,
}

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Manage the notification details cache",
}

var cacheClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear the notification details cache",
	RunE:  runCacheClear,
}

var cacheStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show cache statistics",
	RunE:  runCacheStats,
}

func init() {
	// List command flags (on both root and list commands so `triage` and `triage list` work identically)
	for _, cmd := range []*cobra.Command{rootCmd, listCmd} {
		cmd.Flags().StringVarP(&formatFlag, "format", "f", "", "Output format (table, json)")
		cmd.Flags().IntVarP(&limitFlag, "limit", "l", 0, "Limit number of results")
		cmd.Flags().StringVarP(&sinceFlag, "since", "s", "1w", "Show notifications since (e.g., 1w, 30d, 6mo)")
		cmd.Flags().StringVarP(&priorityFlag, "priority", "p", "", "Filter by priority (urgent, important, quick-win, fyi)")
		cmd.Flags().StringVarP(&reasonFlag, "reason", "r", "", "Filter by reason (mention, review_requested, author, etc.)")
		cmd.Flags().StringVar(&repoFlag, "repo", "", "Filter to specific repo (owner/repo)")
		cmd.Flags().BoolVarP(&verboseFlag, "verbose", "v", false, "Show detailed output")
		cmd.Flags().IntVarP(&workersFlag, "workers", "w", 20, "Number of concurrent workers for fetching details")
		cmd.Flags().BoolVar(&includeMerged, "include-merged", false, "Include notifications for merged PRs")
		cmd.Flags().BoolVar(&includeClosed, "include-closed", false, "Include notifications for closed issues/PRs")
		cmd.Flags().StringVarP(&typeFlag, "type", "t", "", "Filter by type (pr, issue)")
	}

	// Config subcommands
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configShowCmd)

	// Cache subcommands
	cacheCmd.AddCommand(cacheClearCmd)
	cacheCmd.AddCommand(cacheStatsCmd)

	// Root command
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(markReadCmd)
	rootCmd.AddCommand(cacheCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
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

	// Get current user for heuristics
	currentUser, err := ghClient.GetAuthenticatedUser()
	if err != nil {
		return fmt.Errorf("failed to get authenticated user: %w", err)
	}

	// Parse since duration
	since, err := parseDuration(sinceFlag)
	if err != nil {
		return fmt.Errorf("invalid duration: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Fetching notifications from the past %s...\n", sinceFlag)

	// Fetch notifications
	notifications, err := ghClient.ListUnreadNotifications(since)
	if err != nil {
		return fmt.Errorf("failed to fetch notifications: %w", err)
	}

	// Enrich with details
	if len(notifications) > 0 {
		fmt.Fprintf(os.Stderr, "Found %d notifications. Fetching details...\n", len(notifications))

		// Progress callback
		lastPercent := -1
		onProgress := func(completed, total int) {
			percent := (completed * 100) / total
			if percent != lastPercent && percent%5 == 0 {
				fmt.Fprintf(os.Stderr, "\rFetching details: %d/%d (%d%%)...", completed, total, percent)
				lastPercent = percent
			}
		}

		if err := ghClient.EnrichNotificationsConcurrent(notifications, workersFlag, onProgress); err != nil {
			fmt.Fprintf(os.Stderr, "\nWarning: some notifications could not be enriched: %v\n", err)
		}
		fmt.Fprintf(os.Stderr, "\rFetching details: %d/%d (100%%)... done\n", len(notifications), len(notifications))
	}

	// Create cache for PR lists
	prCache, cacheErr := github.NewCache()
	if cacheErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to initialize cache: %v\n", cacheErr)
	}

	// Fetch PRs where user is a requested reviewer
	reviewPRs, reviewFromCache, err := ghClient.ListReviewRequestedPRsCached(currentUser, prCache)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to fetch review-requested PRs: %v\n", err)
	} else if len(reviewPRs) > 0 {
		// Enrich review-requested PRs with additions/deletions (search API doesn't return them)
		for i := range reviewPRs {
			if err := ghClient.EnrichAuthoredPR(&reviewPRs[i]); err != nil {
				continue
			}
		}
		var added int
		notifications, added = mergeReviewRequests(notifications, reviewPRs)
		if added > 0 {
			if reviewFromCache {
				fmt.Fprintf(os.Stderr, "PRs awaiting your review: %d (cached)\n", added)
			} else {
				fmt.Fprintf(os.Stderr, "PRs awaiting your review: %d\n", added)
			}
		}
	}

	// Fetch user's own open PRs
	authoredPRs, authoredFromCache, err := ghClient.ListAuthoredPRsCached(currentUser, prCache)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to fetch authored PRs: %v\n", err)
	} else if len(authoredPRs) > 0 {
		// Enrich authored PRs with additions/deletions (search API doesn't return them)
		for i := range authoredPRs {
			if err := ghClient.EnrichAuthoredPR(&authoredPRs[i]); err != nil {
				continue
			}
		}
		var added int
		notifications, added = mergeAuthoredPRs(notifications, authoredPRs)
		if added > 0 {
			if authoredFromCache {
				fmt.Fprintf(os.Stderr, "Your open PRs: %d (cached)\n", added)
			} else {
				fmt.Fprintf(os.Stderr, "Your open PRs: %d\n", added)
			}
		}
	}

	if len(notifications) == 0 {
		fmt.Println("No unread notifications, pending reviews, or open PRs found.")
		return nil
	}

	// Prioritize
	engine := triage.NewEngine(currentUser)
	items := engine.Prioritize(notifications)

	// Apply filters
	if !includeMerged {
		items = triage.FilterOutMerged(items)
	}
	if !includeClosed {
		items = triage.FilterOutClosed(items)
	}

	if priorityFlag != "" {
		items = triage.FilterByPriority(items, triage.PriorityLevel(priorityFlag))
	}

	if reasonFlag != "" {
		items = triage.FilterByReason(items, []github.NotificationReason{github.NotificationReason(reasonFlag)})
	}

	if typeFlag != "" {
		var subjectType github.SubjectType
		switch typeFlag {
		case "pr", "PR", "pullrequest", "PullRequest":
			subjectType = github.SubjectPullRequest
		case "issue", "Issue":
			subjectType = github.SubjectIssue
		default:
			return fmt.Errorf("invalid type: %s (must be 'pr' or 'issue')", typeFlag)
		}
		items = triage.FilterByType(items, subjectType)
	}

	// Apply limit
	if limitFlag > 0 && len(items) > limitFlag {
		items = items[:limitFlag]
	}

	// Determine format
	format := output.Format(formatFlag)
	if format == "" {
		format = output.Format(cfg.DefaultFormat)
	}

	// Output
	formatter := output.NewFormatter(format)

	if verboseFlag && format == output.FormatTable {
		tableFormatter := formatter.(*output.TableFormatter)
		return tableFormatter.FormatVerbose(items, os.Stdout)
	}

	return formatter.Format(items, os.Stdout)
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	key := args[0]
	value := args[1]

	switch key {
	case "token":
		return fmt.Errorf("tokens cannot be stored in config files for security reasons. Set the GITHUB_TOKEN environment variable instead")
	case "format":
		if value != "table" && value != "json" {
			return fmt.Errorf("invalid format: %s (must be table or json)", value)
		}
		if err := cfg.SetDefaultFormat(value); err != nil {
			return err
		}
		fmt.Printf("Default format set to %s.\n", value)
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}

	return nil
}

func runConfigShow(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	fmt.Println("Configuration:")
	fmt.Printf("  Config file: %s\n", config.ConfigPath())
	fmt.Printf("  Default format: %s\n", cfg.DefaultFormat)

	if os.Getenv("GITHUB_TOKEN") != "" {
		fmt.Println("  GitHub token: (set via GITHUB_TOKEN env)")
	} else {
		fmt.Println("  GitHub token: (not set - set GITHUB_TOKEN env var)")
	}

	if len(cfg.ExcludeRepos) > 0 {
		fmt.Println("  Excluded repos:")
		for _, repo := range cfg.ExcludeRepos {
			fmt.Printf("    - %s\n", repo)
		}
	}

	return nil
}

func runMarkRead(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	token := cfg.GetGitHubToken()
	if token == "" {
		return fmt.Errorf("GitHub token not configured. Set the GITHUB_TOKEN environment variable")
	}

	ghClient, err := github.NewClient(token)
	if err != nil {
		return err
	}

	if err := ghClient.MarkAsRead(args[0]); err != nil {
		return err
	}

	fmt.Println("Notification marked as read.")
	return nil
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

func runCacheClear(cmd *cobra.Command, args []string) error {
	cache, err := github.NewCache()
	if err != nil {
		return fmt.Errorf("failed to access cache: %w", err)
	}

	if err := cache.Clear(); err != nil {
		return fmt.Errorf("failed to clear cache: %w", err)
	}

	fmt.Println("Cache cleared.")
	return nil
}

func runCacheStats(cmd *cobra.Command, args []string) error {
	cache, err := github.NewCache()
	if err != nil {
		return fmt.Errorf("failed to access cache: %w", err)
	}

	stats, err := cache.DetailedStats()
	if err != nil {
		return fmt.Errorf("failed to get cache stats: %w", err)
	}

	fmt.Printf("Cache statistics:\n")
	fmt.Printf("  Item details (TTL: 24h):\n")
	fmt.Printf("    Total: %d\n", stats.DetailTotal)
	fmt.Printf("    Valid: %d\n", stats.DetailValid)
	fmt.Printf("    Expired: %d\n", stats.DetailTotal-stats.DetailValid)
	fmt.Printf("  PR lists (TTL: 5m):\n")
	fmt.Printf("    Total: %d\n", stats.PRListTotal)
	fmt.Printf("    Valid: %d\n", stats.PRListValid)
	fmt.Printf("    Expired: %d\n", stats.PRListTotal-stats.PRListValid)
	return nil
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
