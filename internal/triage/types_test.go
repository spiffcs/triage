package triage

import (
	"testing"
)

func TestPriorityLevelDisplay(t *testing.T) {
	tests := []struct {
		priority PriorityLevel
		want     string
	}{
		{PriorityUrgent, "Urgent"},
		{PriorityImportant, "Important"},
		{PriorityQuickWin, "Quick Win"},
		{PriorityFYI, "FYI"},
		{PriorityLevel("unknown"), "unknown"},
	}

	for _, tt := range tests {
		t.Run(string(tt.priority), func(t *testing.T) {
			got := tt.priority.Display()
			if got != tt.want {
				t.Errorf("PriorityLevel(%q).Display() = %q, want %q", tt.priority, got, tt.want)
			}
		})
	}
}
