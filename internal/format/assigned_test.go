package format

import (
	"testing"
)

func TestGetAssignedUser(t *testing.T) {
	tests := []struct {
		name     string
		input    AssignedInput
		expected string
	}{
		{
			name:     "empty input",
			input:    AssignedInput{},
			expected: "",
		},
		{
			name: "has assignee",
			input: AssignedInput{
				Assignees: []string{"alice", "bob"},
			},
			expected: "alice",
		},
		{
			name: "PR with latest reviewer, no assignee",
			input: AssignedInput{
				IsPR:           true,
				LatestReviewer: "reviewer1",
			},
			expected: "reviewer1",
		},
		{
			name: "PR with requested reviewer, no assignee or latest",
			input: AssignedInput{
				IsPR:               true,
				RequestedReviewers: []string{"req1", "req2"},
			},
			expected: "req1",
		},
		{
			name: "issue with latest reviewer is ignored",
			input: AssignedInput{
				IsPR:           false,
				LatestReviewer: "reviewer1",
			},
			expected: "",
		},
		{
			name: "assignee takes precedence over reviewer",
			input: AssignedInput{
				Assignees:      []string{"assignee1"},
				IsPR:           true,
				LatestReviewer: "reviewer1",
			},
			expected: "assignee1",
		},
		{
			name: "latest reviewer takes precedence over requested",
			input: AssignedInput{
				IsPR:               true,
				LatestReviewer:     "latest",
				RequestedReviewers: []string{"requested"},
			},
			expected: "latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetAssignedUser(tt.input)
			if got != tt.expected {
				t.Errorf("GetAssignedUser() = %q, want %q", got, tt.expected)
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
