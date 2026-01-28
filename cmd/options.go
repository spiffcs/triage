package cmd

// Options holds the shared command-line options for the triage CLI.
type Options struct {
	Format        string
	Since         string
	Priority      string
	Reason        string
	Repo          string
	Type          string
	Limit         int
	Verbosity     int
	Workers       int
	IncludeMerged bool
	IncludeClosed bool
	GreenCI       bool  // Filter to only show PRs with passing CI (excludes issues)
	TUI           *bool // nil = auto-detect, true = force TUI, false = disable TUI

	// Profiling options
	CPUProfile string // Write CPU profile to file
	MemProfile string // Write memory profile to file
	Trace      string // Write execution trace to file
}
