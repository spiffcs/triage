package cmd

import (
	"testing"
	"time"

	"github.com/spiffcs/triage/internal/duration"
)

func TestNew(t *testing.T) {
	cmd := New()
	if cmd == nil {
		t.Fatal("New() returned nil")
	}
	if cmd.Use != "triage" {
		t.Errorf("expected Use to be 'triage', got %q", cmd.Use)
	}
}

func TestNewCmdList(t *testing.T) {
	opts := &Options{}
	cmd := NewCmdList(opts)
	if cmd == nil {
		t.Fatal("NewCmdList() returned nil")
	}
	if cmd.Use != "list" {
		t.Errorf("expected Use to be 'list', got %q", cmd.Use)
	}
}

func TestNewCmdConfig(t *testing.T) {
	cmd := NewCmdConfig()
	if cmd == nil {
		t.Fatal("NewCmdConfig() returned nil")
	}
	if cmd.Use != "config" {
		t.Errorf("expected Use to be 'config', got %q", cmd.Use)
	}
}

func TestNewCmdCache(t *testing.T) {
	cmd := NewCmdCache()
	if cmd == nil {
		t.Fatal("NewCmdCache() returned nil")
	}
	if cmd.Use != "cache" {
		t.Errorf("expected Use to be 'cache', got %q", cmd.Use)
	}
}

func TestNewCmdVersion(t *testing.T) {
	cmd := NewCmdVersion()
	if cmd == nil {
		t.Fatal("NewCmdVersion() returned nil")
	}
	if cmd.Use != "version" {
		t.Errorf("expected Use to be 'version', got %q", cmd.Use)
	}
}

func TestSetVersionInfo(t *testing.T) {
	SetVersionInfo("1.0.0", "abc123", "2024-01-01")
	// Just verify it doesn't panic - version info is in package vars
}

func TestOptions(t *testing.T) {
	opts := &Options{
		Format:    "json",
		Since:     "1w",
		Priority:  "urgent",
		Verbosity: 1,
		Workers:   10,
	}
	if opts.Format != "json" {
		t.Errorf("expected Format to be 'json', got %q", opts.Format)
	}
}

func TestNewOptionsDefaults(t *testing.T) {
	opts := NewOptions()
	if opts.Since != "1w" {
		t.Errorf("expected default Since '1w', got %q", opts.Since)
	}
	if opts.Workers != 20 {
		t.Errorf("expected default Workers 20, got %d", opts.Workers)
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
		WithPriority("urgent"),
		WithReason("mention"),
		WithRepo("owner/repo"),
		WithType("pr"),
		WithLimit(50),
		WithVerbosity(2),
		WithWorkers(10),
		WithIncludeMerged(true),
		WithIncludeClosed(true),
		WithGreenCI(true),
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
	if opts.Priority != "urgent" {
		t.Errorf("expected Priority 'urgent', got %q", opts.Priority)
	}
	if opts.Reason != "mention" {
		t.Errorf("expected Reason 'mention', got %q", opts.Reason)
	}
	if opts.Repo != "owner/repo" {
		t.Errorf("expected Repo 'owner/repo', got %q", opts.Repo)
	}
	if opts.Type != "pr" {
		t.Errorf("expected Type 'pr', got %q", opts.Type)
	}
	if opts.Limit != 50 {
		t.Errorf("expected Limit 50, got %d", opts.Limit)
	}
	if opts.Verbosity != 2 {
		t.Errorf("expected Verbosity 2, got %d", opts.Verbosity)
	}
	if opts.Workers != 10 {
		t.Errorf("expected Workers 10, got %d", opts.Workers)
	}
	if !opts.IncludeMerged {
		t.Error("expected IncludeMerged true")
	}
	if !opts.IncludeClosed {
		t.Error("expected IncludeClosed true")
	}
	if !opts.GreenCI {
		t.Error("expected GreenCI true")
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
	cmd := New(WithFormat("json"), WithWorkers(5))
	if cmd == nil {
		t.Fatal("New() returned nil")
	}
	// The options are applied internally, we can verify New() accepts them without panic
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input    string
		wantErr  bool
		checkAge time.Duration
	}{
		{"1d", false, 24 * time.Hour},
		{"1w", false, 7 * 24 * time.Hour},
		{"30d", false, 30 * 24 * time.Hour},
		{"1mo", false, 30 * 24 * time.Hour},
		{"1h", false, time.Hour},
		{"invalid", true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := duration.Parse(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// Verify the result is approximately the expected age from now
			age := time.Since(result)
			// Allow 1 second tolerance for test execution time
			if age < tt.checkAge-time.Second || age > tt.checkAge+time.Second {
				t.Errorf("expected age ~%v, got %v", tt.checkAge, age)
			}
		})
	}
}

