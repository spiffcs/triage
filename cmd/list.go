package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/spiffcs/triage/config"
	"github.com/spiffcs/triage/internal/github"
	"github.com/spiffcs/triage/internal/log"
	"github.com/spiffcs/triage/internal/output"
	"github.com/spiffcs/triage/internal/triage"
)

// NewCmdList creates the list command.
func NewCmdList(opts *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List prioritized GitHub notifications",
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
	cmd.Flags().StringVarP(&opts.Priority, "priority", "p", "", "Filter by priority (urgent, important, quick-win, fyi)")
	cmd.Flags().StringVarP(&opts.Reason, "reason", "r", "", "Filter by reason (mention, review_requested, author, etc.)")
	cmd.Flags().StringVar(&opts.Repo, "repo", "", "Filter to specific repo (owner/repo)")
	cmd.Flags().CountVarP(&opts.Verbosity, "verbose", "v", "Increase verbosity (-v info, -vv debug, -vvv trace)")
	cmd.Flags().IntVarP(&opts.Workers, "workers", "w", 20, "Number of concurrent workers for fetching details")
	cmd.Flags().BoolVar(&opts.IncludeMerged, "include-merged", false, "Include notifications for merged PRs")
	cmd.Flags().BoolVar(&opts.IncludeClosed, "include-closed", false, "Include notifications for closed issues/PRs")
	cmd.Flags().StringVarP(&opts.Type, "type", "t", "", "Filter by type (pr, issue)")
}

func runList(cmd *cobra.Command, args []string, opts *Options) error {
	// Initialize logging
	log.Initialize(opts.Verbosity, os.Stderr)

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
	since, err := parseDuration(opts.Since)
	if err != nil {
		return fmt.Errorf("invalid duration: %w", err)
	}

	log.Info("fetching notifications", "since", opts.Since)

	// Fetch notifications
	notifications, err := ghClient.ListUnreadNotifications(since)
	if err != nil {
		return fmt.Errorf("failed to fetch notifications: %w", err)
	}

	// Enrich with details
	if len(notifications) > 0 {
		log.Info("found notifications", "count", len(notifications))

		// Progress callback
		lastPercent := -1
		onProgress := func(completed, total int) {
			percent := (completed * 100) / total
			if percent != lastPercent && percent%5 == 0 {
				log.Progress("Fetching details: %d/%d (%d%%)...", completed, total, percent)
				lastPercent = percent
			}
		}

		if err := ghClient.EnrichNotificationsConcurrent(notifications, opts.Workers, onProgress); err != nil {
			log.Warn("some notifications could not be enriched", "error", err)
		}
		log.ProgressDone()
	}

	// Create cache for PR lists
	prCache, cacheErr := github.NewCache()
	if cacheErr != nil {
		log.Warn("failed to initialize cache", "error", cacheErr)
	}

	// Fetch PRs where user is a requested reviewer
	reviewPRs, reviewFromCache, err := ghClient.ListReviewRequestedPRsCached(currentUser, prCache)
	if err != nil {
		log.Warn("failed to fetch review-requested PRs", "error", err)
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
			log.Info("PRs awaiting your review", "count", added, "cached", reviewFromCache)
		}
	}

	// Fetch user's own open PRs
	authoredPRs, authoredFromCache, err := ghClient.ListAuthoredPRsCached(currentUser, prCache)
	if err != nil {
		log.Warn("failed to fetch authored PRs", "error", err)
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
			log.Info("your open PRs", "count", added, "cached", authoredFromCache)
		}
	}

	if len(notifications) == 0 {
		fmt.Println("No unread notifications, pending reviews, or open PRs found.")
		return nil
	}

	// Get score weights and quick win labels (with any user overrides)
	weights := cfg.GetScoreWeights()
	quickWinLabels := cfg.GetQuickWinLabels()

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

	if opts.Type != "" {
		var subjectType github.SubjectType
		switch opts.Type {
		case "pr", "PR", "pullrequest", "PullRequest":
			subjectType = github.SubjectPullRequest
		case "issue", "Issue":
			subjectType = github.SubjectIssue
		default:
			return fmt.Errorf("invalid type: %s (must be 'pr' or 'issue')", opts.Type)
		}
		items = triage.FilterByType(items, subjectType)
	}

	// Apply limit
	if opts.Limit > 0 && len(items) > opts.Limit {
		items = items[:opts.Limit]
	}

	// Determine format
	format := output.Format(opts.Format)
	if format == "" {
		format = output.Format(cfg.DefaultFormat)
	}

	// Output
	formatter := output.NewFormatter(format)

	return formatter.Format(items, os.Stdout)
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
