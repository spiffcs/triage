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
}
