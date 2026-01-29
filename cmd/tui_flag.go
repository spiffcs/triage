package cmd

import (
	"fmt"

	"github.com/spiffcs/triage/internal/tui"
)

// tuiFlag implements pflag.Value for tri-state TUI flag.
type tuiFlag struct {
	opts *Options
}

// newTUIFlag creates a new tuiFlag with the given options.
func newTUIFlag(opts *Options) *tuiFlag {
	return &tuiFlag{opts: opts}
}

func (f *tuiFlag) String() string {
	if f.opts.TUI == nil {
		return "auto"
	}
	if *f.opts.TUI {
		return "true"
	}
	return "false"
}

func (f *tuiFlag) Set(s string) error {
	switch s {
	case "true", "1", "yes":
		v := true
		f.opts.TUI = &v
	case "false", "0", "no":
		v := false
		f.opts.TUI = &v
	case "auto":
		f.opts.TUI = nil
	default:
		return fmt.Errorf("invalid value %q: use true, false, or auto", s)
	}
	return nil
}

func (f *tuiFlag) Type() string {
	return "bool"
}

func (f *tuiFlag) IsBoolFlag() bool {
	return true
}

// shouldUseTUI determines whether to use TUI based on options.
func shouldUseTUI(opts *Options) bool {
	// Disable TUI when verbose logging is requested so logs are visible
	if opts.Verbosity > 0 {
		return false
	}
	if opts.TUI != nil {
		return *opts.TUI
	}
	return tui.ShouldUseTUI()
}
