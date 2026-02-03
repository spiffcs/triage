package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spiffcs/triage/internal/cache"
)

// NewCmdCache creates the cache command with subcommands.
func NewCmdCache() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cache",
		Short: "Manage the notification details cache",
	}

	cmd.AddCommand(newCmdCacheClear())
	cmd.AddCommand(newCmdCacheStats())

	return cmd
}

// newCmdCacheClear creates the cache clear subcommand.
func newCmdCacheClear() *cobra.Command {
	return &cobra.Command{
		Use:   "clear",
		Short: "Clear the notification details cache",
		RunE:  runCacheClear,
	}
}

// newCmdCacheStats creates the cache stats subcommand.
func newCmdCacheStats() *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Show cache statistics",
		RunE:  runCacheStats,
	}
}

func runCacheClear(cmd *cobra.Command, args []string) error {
	c, err := cache.NewCache()
	if err != nil {
		return fmt.Errorf("failed to access cache: %w", err)
	}

	if err := c.Clear(); err != nil {
		return fmt.Errorf("failed to clear cache: %w", err)
	}

	fmt.Println("Cache cleared.")
	return nil
}

func runCacheStats(cmd *cobra.Command, args []string) error {
	c, err := cache.NewCache()
	if err != nil {
		return fmt.Errorf("failed to access cache: %w", err)
	}

	stats, err := c.DetailedStats()
	if err != nil {
		return fmt.Errorf("failed to get cache stats: %w", err)
	}

	fmt.Printf("Cache statistics:\n")
	fmt.Printf("  Item details (TTL: 24h):\n")
	fmt.Printf("    Total: %d\n", stats.DetailTotal)
	fmt.Printf("    Valid: %d\n", stats.DetailValid)
	fmt.Printf("    Expired: %d\n", stats.DetailTotal-stats.DetailValid)

	// Display stats for each list type with appropriate TTL labels
	listTypeInfo := []struct {
		listType cache.ListType
		name     string
		ttl      string
	}{
		{cache.ListTypeNotifications, "Notifications", "1h"},
		{cache.ListTypeReviewRequested, "Review PRs", "5m"},
		{cache.ListTypeAuthored, "Authored PRs", "5m"},
		{cache.ListTypeAssignedIssues, "Assigned Issues", "5m"},
		{cache.ListTypeAssignedPRs, "Assigned PRs", "5m"},
		{cache.ListTypeOrphaned, "Orphaned", "15m"},
	}

	for _, info := range listTypeInfo {
		ls := stats.ListStats[info.listType]
		fmt.Printf("  %s (TTL: %s):\n", info.name, info.ttl)
		fmt.Printf("    Total: %d\n", ls.Total)
		fmt.Printf("    Valid: %d\n", ls.Valid)
		fmt.Printf("    Expired: %d\n", ls.Total-ls.Valid)
	}

	return nil
}
