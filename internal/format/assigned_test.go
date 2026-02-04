package format

import (
	"testing"
)

func TestAssigned(t *testing.T) {
	tests := []struct {
		name     string
		input    AssignedOptions
		expected string
	}{
		{
			name:     "empty input",
			input:    AssignedOptions{},
			expected: "",
		},
		{
			name: "has assignee",
			input: AssignedOptions{
				Assignees: []string{"alice", "bob"},
			},
			expected: "alice",
		},
		{
			name: "PR with latest reviewer but no assignee returns empty",
			input: AssignedOptions{
				IsPR:           true,
				LatestReviewer: "reviewer1",
			},
			expected: "",
		},
		{
			name: "PR with requested reviewer but no assignee returns empty",
			input: AssignedOptions{
				IsPR:               true,
				RequestedReviewers: []string{"req1", "req2"},
			},
			expected: "",
		},
		{
			name: "issue with latest reviewer returns empty",
			input: AssignedOptions{
				IsPR:           false,
				LatestReviewer: "reviewer1",
			},
			expected: "",
		},
		{
			name: "assignee is returned even when reviewers exist",
			input: AssignedOptions{
				Assignees:      []string{"assignee1"},
				IsPR:           true,
				LatestReviewer: "reviewer1",
			},
			expected: "assignee1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Assigned(tt.input)
			if got != tt.expected {
				t.Errorf("Assigned() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestTruncateUsername(t *testing.T) {
	tests := []struct {
		name     string
		username string
		maxWidth int
		expected string
	}{
		{"short enough", "alice", 10, "alice"},
		{"exact fit", "alice", 5, "alice"},
		{"needs truncation", "verylongusername", 10, "verylongu…"},
		{"very short max", "alice", 2, "a…"},
		{"max width 1", "alice", 1, "a"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateUsername(tt.username, tt.maxWidth)
			if got != tt.expected {
				t.Errorf("TruncateUsername(%q, %d) = %q, want %q",
					tt.username, tt.maxWidth, got, tt.expected)
			}
		})
	}
}
