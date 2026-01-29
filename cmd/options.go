package cmd

// Options holds the shared command-line options for the triage CLI.
type Options struct {
	Format    string
	Since     string
	Verbosity int
	TUI       *bool // nil = auto-detect, true = force TUI, false = disable TUI

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
		Since: "1w",
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

// WithVerbosity sets the verbosity level.
func WithVerbosity(v int) Option {
	return func(o *Options) {
		o.Verbosity = v
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
