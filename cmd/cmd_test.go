package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestCommands(t *testing.T) {
	tests := []struct {
		name    string
		newFunc func() *cobra.Command
		wantUse string
	}{
		{"New", func() *cobra.Command { return New() }, "triage"},
		{"NewCmdList", func() *cobra.Command { return NewCmdList(&Options{}) }, "list"},
		{"NewCmdConfig", func() *cobra.Command { return NewCmdConfig() }, "config"},
		{"NewCmdCache", func() *cobra.Command { return NewCmdCache() }, "cache"},
		{"NewCmdVersion", func() *cobra.Command { return NewCmdVersion() }, "version"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.newFunc()
			if cmd == nil {
				t.Fatal("returned nil")
			}
			if cmd.Use != tt.wantUse {
				t.Errorf("expected Use to be %q, got %q", tt.wantUse, cmd.Use)
			}
		})
	}
}

func TestSetVersionInfo(t *testing.T) {
	SetVersionInfo("1.0.0", "abc123", "2024-01-01")
	// Just verify it doesn't panic - version info is in package vars
}

func TestNewOptionsDefaults(t *testing.T) {
	opts := NewOptions()
	if opts.Since != "1w" {
		t.Errorf("expected default Since '1w', got %q", opts.Since)
	}
	if opts.Format != "" {
		t.Errorf("expected default Format empty, got %q", opts.Format)
	}
}

func TestNewOptionsWithOptions(t *testing.T) {
	tui := true
	opts := NewOptions(
		WithFormat("json"),
		WithSince("30d"),
		WithVerbosity(2),
		WithTUI(&tui),
		WithCPUProfile("cpu.prof"),
		WithMemProfile("mem.prof"),
		WithTrace("trace.out"),
	)

	if opts.Format != "json" {
		t.Errorf("expected Format 'json', got %q", opts.Format)
	}
	if opts.Since != "30d" {
		t.Errorf("expected Since '30d', got %q", opts.Since)
	}
	if opts.Verbosity != 2 {
		t.Errorf("expected Verbosity 2, got %d", opts.Verbosity)
	}
	if opts.TUI == nil || !*opts.TUI {
		t.Error("expected TUI true")
	}
	if opts.CPUProfile != "cpu.prof" {
		t.Errorf("expected CPUProfile 'cpu.prof', got %q", opts.CPUProfile)
	}
	if opts.MemProfile != "mem.prof" {
		t.Errorf("expected MemProfile 'mem.prof', got %q", opts.MemProfile)
	}
	if opts.Trace != "trace.out" {
		t.Errorf("expected Trace 'trace.out', got %q", opts.Trace)
	}
}

func TestNewWithOptions(t *testing.T) {
	// Test that New() accepts options and passes them through
	cmd := New(WithFormat("json"))
	if cmd == nil {
		t.Fatal("New() returned nil")
	}
	// The options are applied internally, we can verify New() accepts them without panic
}

