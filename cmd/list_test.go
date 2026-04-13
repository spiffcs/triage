package cmd

import (
	"testing"
	"time"
)

func TestFormatCacheAge(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		want     string
	}{
		{"seconds", 45 * time.Second, "45s"},
		{"minutes", 12 * time.Minute, "12m"},
		{"exact hours", 3 * time.Hour, "3h"},
		{"hours and minutes", 2*time.Hour + 30*time.Minute, "2h30m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatCacheAge(tt.duration)
			if got != tt.want {
				t.Errorf("formatCacheAge(%v) = %q, want %q", tt.duration, got, tt.want)
			}
		})
	}
}
