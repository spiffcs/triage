package config

import (
	"testing"
)

func TestDefaultScoreWeights(t *testing.T) {
	weights := DefaultScoreWeights()

	// Verify key default values
	tests := []struct {
		name  string
		got   int
		want  int
	}{
		{"ReviewRequested", weights.ReviewRequested, 100},
		{"Mention", weights.Mention, 90},
		{"TeamMention", weights.TeamMention, 85},
		{"Author", weights.Author, 70},
		{"Assign", weights.Assign, 60},
		{"Comment", weights.Comment, 30},
		{"StateChange", weights.StateChange, 25},
		{"Subscribed", weights.Subscribed, 10},
		{"CIActivity", weights.CIActivity, 5},
		{"OldUnreadBonus", weights.OldUnreadBonus, 2},
		{"HotTopicBonus", weights.HotTopicBonus, 15},
		{"LowHangingBonus", weights.LowHangingBonus, 20},
		{"OpenStateBonus", weights.OpenStateBonus, 10},
		{"ClosedStatePenalty", weights.ClosedStatePenalty, -30},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("DefaultScoreWeights().%s = %d, want %d", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestGetScoreWeights(t *testing.T) {
	t.Run("returns defaults when no overrides", func(t *testing.T) {
		cfg := &Config{}
		weights := cfg.GetScoreWeights()

		if weights.ReviewRequested != 100 {
			t.Errorf("GetScoreWeights().ReviewRequested = %d, want 100", weights.ReviewRequested)
		}
		if weights.Mention != 90 {
			t.Errorf("GetScoreWeights().Mention = %d, want 90", weights.Mention)
		}
	})

	t.Run("merges partial base score overrides", func(t *testing.T) {
		overriddenValue := 150
		cfg := &Config{
			Weights: &WeightOverrides{
				BaseScores: &BaseScoreOverrides{
					ReviewRequested: &overriddenValue,
				},
			},
		}
		weights := cfg.GetScoreWeights()

		// Overridden value
		if weights.ReviewRequested != 150 {
			t.Errorf("GetScoreWeights().ReviewRequested = %d, want 150", weights.ReviewRequested)
		}
		// Default value preserved
		if weights.Mention != 90 {
			t.Errorf("GetScoreWeights().Mention = %d, want 90", weights.Mention)
		}
	})

	t.Run("merges partial modifier overrides", func(t *testing.T) {
		overriddenValue := 50
		cfg := &Config{
			Weights: &WeightOverrides{
				Modifiers: &ModifierOverrides{
					HotTopicBonus: &overriddenValue,
				},
			},
		}
		weights := cfg.GetScoreWeights()

		// Overridden value
		if weights.HotTopicBonus != 50 {
			t.Errorf("GetScoreWeights().HotTopicBonus = %d, want 50", weights.HotTopicBonus)
		}
		// Default value preserved
		if weights.OldUnreadBonus != 2 {
			t.Errorf("GetScoreWeights().OldUnreadBonus = %d, want 2", weights.OldUnreadBonus)
		}
	})
}

func TestGetQuickWinLabels(t *testing.T) {
	t.Run("returns defaults when not configured", func(t *testing.T) {
		cfg := &Config{}
		labels := cfg.GetQuickWinLabels()

		// Should contain some expected defaults
		found := false
		for _, label := range labels {
			if label == "good first issue" {
				found = true
				break
			}
		}
		if !found {
			t.Error("GetQuickWinLabels() should contain 'good first issue' by default")
		}
	})

	t.Run("returns custom labels when set", func(t *testing.T) {
		customLabels := []string{"easy-fix", "starter"}
		cfg := &Config{
			QuickWinLabels: customLabels,
		}
		labels := cfg.GetQuickWinLabels()

		if len(labels) != 2 {
			t.Errorf("GetQuickWinLabels() returned %d labels, want 2", len(labels))
		}
		if labels[0] != "easy-fix" || labels[1] != "starter" {
			t.Errorf("GetQuickWinLabels() = %v, want %v", labels, customLabels)
		}
	})
}

func TestIsRepoExcluded(t *testing.T) {
	cfg := &Config{
		ExcludeRepos: []string{"owner/repo1", "owner/repo2"},
	}

	tests := []struct {
		name     string
		repoName string
		want     bool
	}{
		{
			name:     "returns true for excluded repo",
			repoName: "owner/repo1",
			want:     true,
		},
		{
			name:     "returns true for another excluded repo",
			repoName: "owner/repo2",
			want:     true,
		},
		{
			name:     "returns false for non-excluded repo",
			repoName: "owner/repo3",
			want:     false,
		},
		{
			name:     "returns false for partial match",
			repoName: "owner/repo",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cfg.IsRepoExcluded(tt.repoName)
			if got != tt.want {
				t.Errorf("IsRepoExcluded(%q) = %v, want %v", tt.repoName, got, tt.want)
			}
		})
	}
}

func TestDefaultQuickWinLabels(t *testing.T) {
	labels := DefaultQuickWinLabels()

	expectedLabels := []string{
		"good first issue",
		"good-first-issue",
		"help wanted",
		"help-wanted",
		"easy",
		"beginner",
		"trivial",
		"documentation",
		"docs",
		"typo",
	}

	if len(labels) != len(expectedLabels) {
		t.Errorf("DefaultQuickWinLabels() returned %d labels, want %d", len(labels), len(expectedLabels))
	}

	for i, label := range labels {
		if label != expectedLabels[i] {
			t.Errorf("DefaultQuickWinLabels()[%d] = %q, want %q", i, label, expectedLabels[i])
		}
	}
}
