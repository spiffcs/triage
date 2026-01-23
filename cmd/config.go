package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/spiffcs/triage/config"
)

// NewCmdConfig creates the config command with subcommands.
func NewCmdConfig() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
	}

	cmd.AddCommand(NewCmdConfigSet())
	cmd.AddCommand(NewCmdConfigShow())

	return cmd
}

// NewCmdConfigSet creates the config set subcommand.
func NewCmdConfigSet() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value",
		Long: `Set a configuration value. Available keys:
  format      - Default output format (table, json)`,
		Args: cobra.ExactArgs(2),
		RunE: runConfigSet,
	}
}

// NewCmdConfigShow creates the config show subcommand.
func NewCmdConfigShow() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show current configuration",
		RunE:  runConfigShow,
	}
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
	if config.ConfigFileExists() {
		fmt.Printf("  Config file: %s\n", config.ConfigPath())
	}
	fmt.Printf("  Default format: %s\n", cfg.DefaultFormat)

	// Show excluded repos
	fmt.Println("  Excluded repos:")
	if len(cfg.ExcludeRepos) > 0 {
		for _, repo := range cfg.ExcludeRepos {
			fmt.Printf("    - %s\n", repo)
		}
	} else {
		fmt.Println("    (none)")
	}

	// Show quick win labels
	fmt.Println("  Quick win labels:")
	for _, label := range cfg.GetQuickWinLabels() {
		fmt.Printf("    - %s\n", label)
	}

	// Show effective weights (merged defaults + overrides)
	weights := cfg.GetScoreWeights()
	fmt.Println("  Weights:")
	fmt.Println("    Base scores:")
	fmt.Printf("      review_requested: %d\n", weights.ReviewRequested)
	fmt.Printf("      mention: %d\n", weights.Mention)
	fmt.Printf("      team_mention: %d\n", weights.TeamMention)
	fmt.Printf("      author: %d\n", weights.Author)
	fmt.Printf("      assign: %d\n", weights.Assign)
	fmt.Printf("      comment: %d\n", weights.Comment)
	fmt.Printf("      state_change: %d\n", weights.StateChange)
	fmt.Printf("      subscribed: %d\n", weights.Subscribed)
	fmt.Printf("      ci_activity: %d\n", weights.CIActivity)
	fmt.Println("    Modifiers:")
	fmt.Printf("      old_unread_bonus: %d\n", weights.OldUnreadBonus)
	fmt.Printf("      hot_topic_bonus: %d\n", weights.HotTopicBonus)
	fmt.Printf("      low_hanging_bonus: %d\n", weights.LowHangingBonus)
	fmt.Printf("      open_state_bonus: %d\n", weights.OpenStateBonus)
	fmt.Printf("      closed_state_penalty: %d\n", weights.ClosedStatePenalty)
	fmt.Printf("      fyi_promotion_threshold: %d\n", weights.FYIPromotionThreshold)

	return nil
}
