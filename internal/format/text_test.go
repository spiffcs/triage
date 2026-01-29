package format

import (
	"testing"
)

func TestStripAnsi(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"no ansi", "hello", "hello"},
		{"single color", "\x1b[31mred\x1b[0m", "red"},
		{"multiple colors", "\x1b[31mred\x1b[0m \x1b[32mgreen\x1b[0m", "red green"},
		{"bold", "\x1b[1mbold\x1b[0m", "bold"},
		{"complex", "\x1b[1;31;40mbold red on black\x1b[0m", "bold red on black"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripAnsi(tt.input)
			if got != tt.expected {
				t.Errorf("StripAnsi(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestDisplayWidth(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"empty", "", 0},
		{"ascii", "hello", 5},
		{"with ansi", "\x1b[31mred\x1b[0m", 3},
		{"emoji fire", "🔥", 2},
		{"emoji lightning with VS16", "⚡\uFE0F", 2},
		{"emoji and text", "🔥 hot", 6},
		{"wide chars", "日本語", 6},
		{"mixed", "Hello, 世界!", 12},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DisplayWidth(tt.input)
			if got != tt.expected {
				t.Errorf("DisplayWidth(%q) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}

func TestTruncateToWidth(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		maxWidth      int
		expectedStr   string
		expectedWidth int
	}{
		{"no truncation needed", "hello", 10, "hello", 5},
		{"exact fit", "hello", 5, "hello", 5},
		{"truncate ascii", "hello world", 8, "hello...", 8},
		{"truncate with emoji", "🔥 fire", 5, "🔥...", 5},
		{"preserve ansi", "\x1b[31mred text\x1b[0m", 6, "\x1b[31mred...\x1b[0m", 6},
		{"very short max", "hello", 3, "...", 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStr, gotWidth := TruncateToWidth(tt.input, tt.maxWidth)
			if gotStr != tt.expectedStr {
				t.Errorf("TruncateToWidth(%q, %d) string = %q, want %q", tt.input, tt.maxWidth, gotStr, tt.expectedStr)
			}
			if gotWidth != tt.expectedWidth {
				t.Errorf("TruncateToWidth(%q, %d) width = %d, want %d", tt.input, tt.maxWidth, gotWidth, tt.expectedWidth)
			}
		})
	}
}

func TestPadRight(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		visibleWidth int
		targetWidth  int
		expected     string
	}{
		{"no padding needed", "hello", 5, 5, "hello"},
		{"add padding", "hi", 2, 5, "hi   "},
		{"already exceeds", "hello", 5, 3, "hello"},
		{"with ansi", "\x1b[31mred\x1b[0m", 3, 5, "\x1b[31mred\x1b[0m  "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PadRight(tt.input, tt.visibleWidth, tt.targetWidth)
			if got != tt.expected {
				t.Errorf("PadRight(%q, %d, %d) = %q, want %q", tt.input, tt.visibleWidth, tt.targetWidth, got, tt.expected)
			}
		})
	}
}
