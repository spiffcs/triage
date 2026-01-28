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

	// Sorting options
	SortByAge   bool   // Sort by age instead of priority
	OldestFirst bool   // When sorting by age, show oldest items first
	SortBy      string // Sort orphaned pane: column with optional direction prefix (+/-)

	// Orphaned contribution options
	SkipOrphaned        bool     // Skip orphaned contribution fetching (included by default)
	OrphanedRepos       []string // Explicit repos for orphaned (overrides config)
	StaleDays           int      // Days without team response to be considered orphaned
	ConsecutiveComments int      // Consecutive author comments without response

	// Profiling options
	CPUProfile string // Write CPU profile to file
	MemProfile string // Write memory profile to file
	Trace      string // Write execution trace to file
}

// Option is a functional option for configuring Options.
type Option func(*Options)

// NewOptions creates a new Options with defaults and applies any provided options.
func NewOptions(opts ...Option) *Options {
	o := &Options{
		Since:   "1w",
		Workers: 20,
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// WithFormat sets the output format (table, json).
func WithFormat(format string) Option {
	return func(o *Options) {
		o.Format = format
	}
}

// WithSince sets the time window for notifications (e.g., "1w", "30d", "6mo").
func WithSince(since string) Option {
	return func(o *Options) {
		o.Since = since
	}
}

// WithPriority sets the priority filter.
func WithPriority(priority string) Option {
	return func(o *Options) {
		o.Priority = priority
	}
}

// WithReason sets the reason filter.
func WithReason(reason string) Option {
	return func(o *Options) {
		o.Reason = reason
	}
}

// WithRepo sets the repository filter.
func WithRepo(repo string) Option {
	return func(o *Options) {
		o.Repo = repo
	}
}

// WithType sets the type filter (pr, issue).
func WithType(t string) Option {
	return func(o *Options) {
		o.Type = t
	}
}

// WithLimit sets the maximum number of results.
func WithLimit(limit int) Option {
	return func(o *Options) {
		o.Limit = limit
	}
}

// WithVerbosity sets the verbosity level.
func WithVerbosity(v int) Option {
	return func(o *Options) {
		o.Verbosity = v
	}
}

// WithWorkers sets the number of concurrent workers.
func WithWorkers(workers int) Option {
	return func(o *Options) {
		o.Workers = workers
	}
}

// WithIncludeMerged includes merged PRs in the results.
func WithIncludeMerged(include bool) Option {
	return func(o *Options) {
		o.IncludeMerged = include
	}
}

// WithIncludeClosed includes closed issues/PRs in the results.
func WithIncludeClosed(include bool) Option {
	return func(o *Options) {
		o.IncludeClosed = include
	}
}

// WithGreenCI filters to only show PRs with passing CI.
func WithGreenCI(greenCI bool) Option {
	return func(o *Options) {
		o.GreenCI = greenCI
	}
}

// WithTUI controls TUI mode (nil = auto-detect, true = force, false = disable).
func WithTUI(tui *bool) Option {
	return func(o *Options) {
		o.TUI = tui
	}
}

// WithCPUProfile sets the CPU profile output file.
func WithCPUProfile(path string) Option {
	return func(o *Options) {
		o.CPUProfile = path
	}
}

// WithMemProfile sets the memory profile output file.
func WithMemProfile(path string) Option {
	return func(o *Options) {
		o.MemProfile = path
	}
}

// WithTrace sets the execution trace output file.
func WithTrace(path string) Option {
	return func(o *Options) {
		o.Trace = path
	}
}
