package format

import (
	"testing"
	"time"
)

func TestFormatAge(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		// Now (sub-minute)
		{"zero", 0, "now"},
		{"30 seconds", 30 * time.Second, "now"},
		{"59 seconds", 59 * time.Second, "now"},

		// Minutes
		{"1 minute", time.Minute, "1m"},
		{"30 minutes", 30 * time.Minute, "30m"},
		{"59 minutes", 59 * time.Minute, "59m"},

		// Hours
		{"1 hour", time.Hour, "1h"},
		{"12 hours", 12 * time.Hour, "12h"},
		{"23 hours", 23 * time.Hour, "23h"},

		// Days
		{"1 day", 24 * time.Hour, "1d"},
		{"3 days", 3 * 24 * time.Hour, "3d"},
		{"6 days", 6 * 24 * time.Hour, "6d"},

		// Weeks
		{"7 days (1 week)", 7 * 24 * time.Hour, "1w"},
		{"14 days (2 weeks)", 14 * 24 * time.Hour, "2w"},
		{"21 days (3 weeks)", 21 * 24 * time.Hour, "3w"},
		{"28 days (4 weeks)", 28 * 24 * time.Hour, "4w"},
		{"29 days", 29 * 24 * time.Hour, "4w"},

		// Months
		{"30 days (1 month)", 30 * 24 * time.Hour, "1mo"},
		{"60 days (2 months)", 60 * 24 * time.Hour, "2mo"},
		{"90 days (3 months)", 90 * 24 * time.Hour, "3mo"},
		{"365 days (12 months)", 365 * 24 * time.Hour, "12mo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatAge(tt.duration)
			if got != tt.expected {
				t.Errorf("FormatAge(%v) = %q, want %q", tt.duration, got, tt.expected)
			}
		})
	}
}
