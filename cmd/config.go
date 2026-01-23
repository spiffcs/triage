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
		Short: "Show or manage configuration",
		Long: `Show or manage configuration.

When run without arguments, outputs the complete configuration as YAML.
This can be redirected to create a config file with all defaults:

  triage config > ~/.config/triage/config.yaml

Use 'triage config set' to modify individual settings.`,
		RunE: runConfig,
	}

	cmd.AddCommand(NewCmdConfigSet())

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

func runConfig(cmd *cobra.Command, args []string) error {
	// Output a complete config with all defaults as YAML
	cfg := config.DefaultConfig()

	yamlStr, err := cfg.ToYAML()
	if err != nil {
		return err
	}

	fmt.Print(yamlStr)
	return nil
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
