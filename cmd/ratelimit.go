package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/spiffcs/triage/config"
	"github.com/spiffcs/triage/internal/github"
)

// NewCmdRateLimit creates the ratelimit command.
func NewCmdRateLimit() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ratelimit",
		Short: "Check GitHub API rate limit status",
		Long:  `Display current GitHub API rate limit status including remaining quota and reset time.`,
	}
	cmd.AddCommand(NewCmdRateLimitStatus())
	return cmd
}

// NewCmdRateLimitStatus creates the ratelimit status subcommand.
func NewCmdRateLimitStatus() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current rate limit status",
		Long:  `Display the current GitHub API rate limit status for core and search APIs.`,
		RunE:  runRateLimitStatus,
	}
}

func runRateLimitStatus(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	token := cfg.GetGitHubToken()
	if token == "" {
		return fmt.Errorf("GitHub token not configured. Set the GITHUB_TOKEN environment variable")
	}

	client, err := github.NewClient(token)
	if err != nil {
		return err
	}

	limits, _, err := client.RawClient().RateLimit.Get(client.Context())
	if err != nil {
		return fmt.Errorf("failed to get rate limits: %w", err)
	}

	fmt.Println("GitHub API Rate Limits:")
	fmt.Println()

	if limits.Core != nil {
		resetIn := time.Until(limits.Core.Reset.Time).Round(time.Second)
		if resetIn < 0 {
			resetIn = 0
		}
		fmt.Printf("Core API:   %d/%d remaining (resets in %s)\n",
			limits.Core.Remaining, limits.Core.Limit, resetIn)
	}

	if limits.Search != nil {
		resetIn := time.Until(limits.Search.Reset.Time).Round(time.Second)
		if resetIn < 0 {
			resetIn = 0
		}
		fmt.Printf("Search API: %d/%d remaining (resets in %s)\n",
			limits.Search.Remaining, limits.Search.Limit, resetIn)
	}

	if limits.GraphQL != nil {
		resetIn := time.Until(limits.GraphQL.Reset.Time).Round(time.Second)
		if resetIn < 0 {
			resetIn = 0
		}
		fmt.Printf("GraphQL:    %d/%d remaining (resets in %s)\n",
			limits.GraphQL.Remaining, limits.GraphQL.Limit, resetIn)
	}

	return nil
}
