package cmd

import (
	"testing"
	"time"
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
			result, err := parseDuration(tt.input)
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
