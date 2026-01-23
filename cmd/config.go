package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spiffcs/triage/config"
)

// NewCmdConfig creates the config command with subcommands.
func NewCmdConfig() *cobra.Command {
	var outputFormat string

	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show or manage configuration",
		Long: `Show or manage configuration.

When run without arguments, shows the current merged configuration.

Subcommands:
  init      Create a minimal config file
  path      Show config file locations
  defaults  Show all default values
  show      Show current merged config (same as bare 'triage config')
  set       Set a configuration value`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigShow(cmd, args, outputFormat)
		},
	}

	cmd.Flags().StringVarP(&outputFormat, "output", "o", "yaml", "Output format (yaml, json)")

	cmd.AddCommand(NewCmdConfigInit())
	cmd.AddCommand(NewCmdConfigPath())
	cmd.AddCommand(NewCmdConfigDefaults())
	cmd.AddCommand(NewCmdConfigShow())
	cmd.AddCommand(NewCmdConfigSet())

	return cmd
}

// NewCmdConfigInit creates the config init subcommand.
func NewCmdConfigInit() *cobra.Command {
	var global, local bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create a minimal config file",
		Long: `Create a minimal config file with starter settings.

Use --global to create in ~/.config/triage/config.yaml (applies everywhere)
Use --local to create in ./.triage.yaml (applies only in this directory)
Without flags, you'll be prompted to choose.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigInit(global, local)
		},
	}

	cmd.Flags().BoolVar(&global, "global", false, "Create global config file (~/.config/triage/config.yaml)")
	cmd.Flags().BoolVar(&local, "local", false, "Create local config file (./.triage.yaml)")

	return cmd
}

// NewCmdConfigPath creates the config path subcommand.
func NewCmdConfigPath() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Show config file locations",
		Long:  `Show the paths to global and local config files and indicate which exist.`,
		RunE:  runConfigPath,
	}
}

// NewCmdConfigDefaults creates the config defaults subcommand.
func NewCmdConfigDefaults() *cobra.Command {
	var outputFormat string

	cmd := &cobra.Command{
		Use:   "defaults",
		Short: "Show all default configuration values",
		Long: `Show a complete configuration with all default values.

This can be redirected to create a config file with all defaults:
  triage config defaults > ~/.config/triage/config.yaml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigDefaults(outputFormat)
		},
	}

	cmd.Flags().StringVarP(&outputFormat, "output", "o", "yaml", "Output format (yaml, json)")

	return cmd
}

// NewCmdConfigShow creates the config show subcommand.
func NewCmdConfigShow() *cobra.Command {
	var outputFormat string

	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show current merged configuration",
		Long:  `Show the current configuration after merging defaults, global, and local configs.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigShow(cmd, args, outputFormat)
		},
	}

	cmd.Flags().StringVarP(&outputFormat, "output", "o", "yaml", "Output format (yaml, json)")

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

func runConfigInit(global, local bool) error {
	if global && local {
		return fmt.Errorf("cannot specify both --global and --local")
	}

	paths := config.GetConfigPaths()
	var targetPath string
	var location string

	if global {
		targetPath = paths.GlobalPath
		location = "global"
	} else if local {
		targetPath = paths.LocalPath
		location = "local"
	} else {
		// Prompt user to choose
		fmt.Println("Where would you like to create the config file?")
		fmt.Printf("  [1] Global (%s) - applies everywhere\n", paths.GlobalPath)
		fmt.Printf("  [2] Local (%s) - applies only in this directory\n", paths.LocalPath)
		fmt.Print("Choose [1/2]: ")

		reader := bufio.NewReader(os.Stdin)
		choice, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}

		choice = strings.TrimSpace(choice)
		switch choice {
		case "1":
			targetPath = paths.GlobalPath
			location = "global"
		case "2":
			targetPath = paths.LocalPath
			location = "local"
		default:
			return fmt.Errorf("invalid choice: %s (must be 1 or 2)", choice)
		}
		fmt.Println()
	}

	// Check if file already exists
	if _, err := os.Stat(targetPath); err == nil {
		return fmt.Errorf("config file already exists: %s\nUse 'triage config show' to view current config", targetPath)
	}

	// Write the minimal config
	if err := config.SaveTo(targetPath, config.MinimalConfig()); err != nil {
		return err
	}

	fmt.Printf("Created %s config file: %s\n\n", location, targetPath)
	fmt.Println("Edit this file to customize triage behavior.")
	fmt.Println("Run 'triage config defaults' to see all available options.")

	return nil
}

func runConfigPath(_ *cobra.Command, _ []string) error {
	paths := config.GetConfigPaths()

	fmt.Println("Configuration file locations:")
	fmt.Println()

	globalStatus := "not found"
	if paths.GlobalExists {
		globalStatus = "exists"
	}
	fmt.Printf("  Global: %s (%s)\n", paths.GlobalPath, globalStatus)

	localStatus := "not found"
	if paths.LocalExists {
		localStatus = "exists"
	}
	fmt.Printf("  Local:  %s (%s)\n", paths.LocalPath, localStatus)

	fmt.Println()
	fmt.Println("Load order: defaults -> global -> local (local overrides global)")

	return nil
}

func runConfigDefaults(format string) error {
	cfg := config.DefaultConfig()

	switch format {
	case "yaml":
		yamlStr, err := cfg.ToYAML()
		if err != nil {
			return err
		}
		fmt.Print(yamlStr)
	case "json":
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal config to JSON: %w", err)
		}
		fmt.Println(string(data))
	default:
		return fmt.Errorf("invalid format: %s (must be yaml or json)", format)
	}

	return nil
}

func runConfigShow(_ *cobra.Command, _ []string, format string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	switch format {
	case "yaml":
		yamlStr, err := cfg.ToYAML()
		if err != nil {
			return err
		}
		fmt.Print(yamlStr)
	case "json":
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal config to JSON: %w", err)
		}
		fmt.Println(string(data))
	default:
		return fmt.Errorf("invalid format: %s (must be yaml or json)", format)
	}

	return nil
}

func runConfigSet(_ *cobra.Command, args []string) error {
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
