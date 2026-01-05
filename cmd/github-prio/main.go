package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/hal/github-prio/config"
	"github.com/hal/github-prio/internal/github"
	"github.com/hal/github-prio/internal/output"
	"github.com/hal/github-prio/internal/priority"
)

var (
	// Global flags
	formatFlag   string
	limitFlag    int
	sinceFlag    string
	categoryFlag string
	reasonFlag   string
	repoFlag     string
	analyzeFlag  bool
	verboseFlag  bool
	quickFlag    bool
	workersFlag  int
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "github-prio",
	Short: "GitHub notification priority manager",
	Long: `A CLI tool that analyzes your GitHub notifications to help you
prioritize your work. It uses heuristics to score notifications and
optionally uses Claude AI for deeper analysis.`,
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List prioritized GitHub notifications",
	Long: `Fetches your unread GitHub notifications, enriches them with
issue/PR details, and displays them sorted by priority.`,
	RunE: runList,
}

var analyzeCmd = &cobra.Command{
	Use:   "analyze [notification-id or url]",
	Short: "Get detailed AI analysis of a notification",
	Long: `Uses Claude AI to provide detailed analysis of a specific
notification, including suggested actions and effort estimates.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runAnalyze,
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Long: `Set a configuration value. Available keys:
  token       - GitHub personal access token
  claude-key  - Claude API key
  format      - Default output format (table, json, markdown)`,
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

var summaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "Show a summary of notifications",
	RunE:  runSummary,
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
	// List command flags
	listCmd.Flags().StringVarP(&formatFlag, "format", "f", "", "Output format (table, json, markdown)")
	listCmd.Flags().IntVarP(&limitFlag, "limit", "l", 0, "Limit number of results")
	listCmd.Flags().StringVarP(&sinceFlag, "since", "s", "6mo", "Show notifications since (e.g., 1w, 30d, 6mo)")
	listCmd.Flags().StringVarP(&categoryFlag, "category", "c", "", "Filter by category (urgent, important, low-hanging, fyi)")
	listCmd.Flags().StringVarP(&reasonFlag, "reason", "r", "", "Filter by reason (mention, review_requested, author, etc.)")
	listCmd.Flags().StringVar(&repoFlag, "repo", "", "Filter to specific repo (owner/repo)")
	listCmd.Flags().BoolVarP(&analyzeFlag, "analyze", "a", false, "Include AI analysis (requires Claude API key)")
	listCmd.Flags().BoolVarP(&verboseFlag, "verbose", "v", false, "Show detailed output")
	listCmd.Flags().BoolVarP(&quickFlag, "quick", "q", false, "Skip fetching details (faster but less accurate prioritization)")
	listCmd.Flags().IntVarP(&workersFlag, "workers", "w", 20, "Number of concurrent workers for fetching details")

	// Summary command flags
	summaryCmd.Flags().StringVarP(&sinceFlag, "since", "s", "6mo", "Show notifications since")
	summaryCmd.Flags().StringVarP(&formatFlag, "format", "f", "", "Output format")

	// Config subcommands
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configShowCmd)

	// Cache subcommands
	cacheCmd.AddCommand(cacheClearCmd)
	cacheCmd.AddCommand(cacheStatsCmd)

	// Root command
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(analyzeCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(markReadCmd)
	rootCmd.AddCommand(summaryCmd)
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
		return fmt.Errorf("GitHub token not configured. Set GITHUB_TOKEN env var or run: github-prio config set token <TOKEN>")
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

	if len(notifications) == 0 {
		fmt.Println("No unread notifications found.")
		return nil
	}

	// Enrich with details (unless quick mode)
	if !quickFlag {
		fmt.Fprintf(os.Stderr, "Found %d notifications. Fetching details (use -q for quick mode)...\n", len(notifications))

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
	} else {
		fmt.Fprintf(os.Stderr, "Found %d notifications. Skipping detail fetch (quick mode)...\n", len(notifications))
	}

	// Create LLM client if analyze flag is set
	var llmClient *priority.LLMClient
	if analyzeFlag {
		claudeKey := cfg.GetClaudeAPIKey()
		if claudeKey == "" {
			fmt.Fprintf(os.Stderr, "Warning: Claude API key not set. Skipping AI analysis.\n")
		} else {
			llmClient, err = priority.NewLLMClient(claudeKey)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to create LLM client: %v\n", err)
			}
		}
	}

	// Prioritize
	engine := priority.NewEngine(currentUser, llmClient)
	var items []priority.PrioritizedItem

	if analyzeFlag && llmClient != nil {
		fmt.Fprintf(os.Stderr, "Analyzing with AI...\n")
		items, err = engine.PrioritizeWithAnalysis(notifications)
		if err != nil {
			return fmt.Errorf("failed to prioritize: %w", err)
		}
	} else {
		items = engine.Prioritize(notifications)
	}

	// Apply filters
	if categoryFlag != "" {
		items = priority.FilterByCategory(items, priority.Category(categoryFlag))
	}

	if reasonFlag != "" {
		items = priority.FilterByReason(items, []github.NotificationReason{github.NotificationReason(reasonFlag)})
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
		return tableFormatter.FormatWithAnalysis(items, os.Stdout)
	}

	return formatter.Format(items, os.Stdout)
}

func runAnalyze(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	claudeKey := cfg.GetClaudeAPIKey()
	if claudeKey == "" {
		return fmt.Errorf("Claude API key not configured. Set ANTHROPIC_API_KEY env var or run: github-prio config set claude-key <KEY>")
	}

	// If no specific notification, analyze top items
	if len(args) == 0 {
		// List top items and analyze them
		analyzeFlag = true
		verboseFlag = true
		limitFlag = 5
		return runList(cmd, args)
	}

	// TODO: Implement single notification analysis by ID/URL
	return fmt.Errorf("single notification analysis not yet implemented")
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
		if err := cfg.SetToken(value); err != nil {
			return err
		}
		fmt.Println("GitHub token saved.")
	case "claude-key":
		if err := cfg.SetClaudeKey(value); err != nil {
			return err
		}
		fmt.Println("Claude API key saved.")
	case "format":
		if value != "table" && value != "json" && value != "markdown" {
			return fmt.Errorf("invalid format: %s (must be table, json, or markdown)", value)
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

	if cfg.GitHubToken != "" {
		fmt.Println("  GitHub token: (set)")
	} else if os.Getenv("GITHUB_TOKEN") != "" {
		fmt.Println("  GitHub token: (set via GITHUB_TOKEN env)")
	} else {
		fmt.Println("  GitHub token: (not set)")
	}

	if cfg.ClaudeAPIKey != "" {
		fmt.Println("  Claude API key: (set)")
	} else if os.Getenv("ANTHROPIC_API_KEY") != "" {
		fmt.Println("  Claude API key: (set via ANTHROPIC_API_KEY env)")
	} else {
		fmt.Println("  Claude API key: (not set)")
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
		return fmt.Errorf("GitHub token not configured")
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

func runSummary(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	token := cfg.GetGitHubToken()
	if token == "" {
		return fmt.Errorf("GitHub token not configured")
	}

	ghClient, err := github.NewClient(token)
	if err != nil {
		return err
	}

	currentUser, err := ghClient.GetAuthenticatedUser()
	if err != nil {
		return err
	}

	since, err := parseDuration(sinceFlag)
	if err != nil {
		return err
	}

	notifications, err := ghClient.ListUnreadNotifications(since)
	if err != nil {
		return err
	}

	if err := ghClient.EnrichNotifications(notifications); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}

	engine := priority.NewEngine(currentUser, nil)
	items := engine.Prioritize(notifications)
	summary := priority.Summarize(items)

	format := output.Format(formatFlag)
	if format == "" {
		format = output.Format(cfg.DefaultFormat)
	}

	formatter := output.NewFormatter(format)
	return formatter.FormatSummary(summary, os.Stdout)
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

	total, valid, err := cache.Stats()
	if err != nil {
		return fmt.Errorf("failed to get cache stats: %w", err)
	}

	fmt.Printf("Cache statistics:\n")
	fmt.Printf("  Total entries: %d\n", total)
	fmt.Printf("  Valid (< 24h): %d\n", valid)
	fmt.Printf("  Expired: %d\n", total-valid)
	return nil
}
