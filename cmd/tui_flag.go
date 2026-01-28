package cmd

import (
	"fmt"

	"github.com/spiffcs/triage/internal/tui"
)

// TUIFlagProvider allows sharing TUI flag logic across commands.
type TUIFlagProvider interface {
	GetTUI() *bool
	SetTUI(v *bool)
	GetVerbosity() int
}

// TUIFlag implements pflag.Value for tri-state TUI flag.
type TUIFlag struct {
	provider TUIFlagProvider
}

// NewTUIFlag creates a new TUIFlag with the given provider.
func NewTUIFlag(provider TUIFlagProvider) *TUIFlag {
	return &TUIFlag{provider: provider}
}

func (f *TUIFlag) String() string {
	if f.provider.GetTUI() == nil {
		return "auto"
	}
	if *f.provider.GetTUI() {
		return "true"
	}
	return "false"
}

func (f *TUIFlag) Set(s string) error {
	switch s {
	case "true", "1", "yes":
		v := true
		f.provider.SetTUI(&v)
	case "false", "0", "no":
		v := false
		f.provider.SetTUI(&v)
	case "auto":
		f.provider.SetTUI(nil)
	default:
		return fmt.Errorf("invalid value %q: use true, false, or auto", s)
	}
	return nil
}

func (f *TUIFlag) Type() string {
	return "bool"
}

func (f *TUIFlag) IsBoolFlag() bool {
	return true
}

// ShouldUseTUI determines whether to use TUI based on provider settings.
func ShouldUseTUI(provider TUIFlagProvider) bool {
	// Disable TUI when verbose logging is requested so logs are visible
	if provider.GetVerbosity() > 0 {
		return false
	}
	if t := provider.GetTUI(); t != nil {
		return *t
	}
	return tui.ShouldUseTUI()
}
