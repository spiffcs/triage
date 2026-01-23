package cmd

import (
	"fmt"
	"os"

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
