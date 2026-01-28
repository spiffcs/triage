package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spiffcs/triage/internal/github"
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
	fmt.Printf("  Notification lists (TTL: 1h):\n")
	fmt.Printf("    Total: %d\n", stats.NotifListTotal)
	fmt.Printf("    Valid: %d\n", stats.NotifListValid)
	fmt.Printf("    Expired: %d\n", stats.NotifListTotal-stats.NotifListValid)
	fmt.Printf("  PR lists (TTL: 5m):\n")
	fmt.Printf("    Total: %d\n", stats.PRListTotal)
	fmt.Printf("    Valid: %d\n", stats.PRListValid)
	fmt.Printf("    Expired: %d\n", stats.PRListTotal-stats.PRListValid)
	return nil
}
