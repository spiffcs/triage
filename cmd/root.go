package cmd

import (
	"github.com/spf13/cobra"
)

// New creates the root command with all subcommands registered.
func New() *cobra.Command {
	opts := &Options{}

	rootCmd := &cobra.Command{
		Use:   "triage",
		Short: "GitHub notification triage manager",
		Long: `A CLI tool that analyzes your GitHub notifications to help you
triage your work. It uses heuristics to score notifications.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, args, opts)
		},
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
	}

	// Add list flags to root command so `triage` and `triage list` work identically
	addListFlags(rootCmd, opts)

	// Register subcommands
	rootCmd.AddCommand(NewCmdList(opts))
	rootCmd.AddCommand(NewCmdConfig())
	rootCmd.AddCommand(NewCmdCache())
	rootCmd.AddCommand(NewCmdVersion())
	rootCmd.AddCommand(NewCmdRateLimit())
	rootCmd.AddCommand(NewCmdOrphaned())

	return rootCmd
}
