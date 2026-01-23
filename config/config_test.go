package config

import (
	"testing"
)

func TestDefaultScoreWeights(t *testing.T) {
	weights := DefaultScoreWeights()

	// Verify key default values
	tests := []struct {
		name string
		got  int
		want int
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
		// New authored PR modifiers
		{"ApprovedPRBonus", weights.ApprovedPRBonus, 25},
		{"MergeablePRBonus", weights.MergeablePRBonus, 15},
		{"ChangesRequestedBonus", weights.ChangesRequestedBonus, 20},
		{"ReviewCommentBonus", weights.ReviewCommentBonus, 3},
		{"ReviewCommentMaxBonus", weights.ReviewCommentMaxBonus, 15},
		{"StalePRThresholdDays", weights.StalePRThresholdDays, 7},
		{"StalePRBonusPerDay", weights.StalePRBonusPerDay, 2},
		{"StalePRMaxBonus", weights.StalePRMaxBonus, 20},
		{"DraftPRPenalty", weights.DraftPRPenalty, -15},
		// General scoring
		{"MaxAgeBonus", weights.MaxAgeBonus, 30},
		// Low-hanging fruit detection
		{"SmallPRMaxFiles", weights.SmallPRMaxFiles, 3},
		{"SmallPRMaxLines", weights.SmallPRMaxLines, 50},
		// Display threshold
		{"HotTopicDisplayThreshold", weights.HotTopicDisplayThreshold, 10},
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

	t.Run("merges new PR modifier overrides", func(t *testing.T) {
		approvedBonus := 50
		staleDays := 5
		smallFiles := 10
		cfg := &Config{
			Weights: &WeightOverrides{
				Modifiers: &ModifierOverrides{
					ApprovedPRBonus:      &approvedBonus,
					StalePRThresholdDays: &staleDays,
					SmallPRMaxFiles:      &smallFiles,
				},
			},
		}
		weights := cfg.GetScoreWeights()

		// Overridden values
		if weights.ApprovedPRBonus != 50 {
			t.Errorf("GetScoreWeights().ApprovedPRBonus = %d, want 50", weights.ApprovedPRBonus)
		}
		if weights.StalePRThresholdDays != 5 {
			t.Errorf("GetScoreWeights().StalePRThresholdDays = %d, want 5", weights.StalePRThresholdDays)
		}
		if weights.SmallPRMaxFiles != 10 {
			t.Errorf("GetScoreWeights().SmallPRMaxFiles = %d, want 10", weights.SmallPRMaxFiles)
		}

		// Default values preserved
		if weights.MergeablePRBonus != 15 {
			t.Errorf("GetScoreWeights().MergeablePRBonus = %d, want 15", weights.MergeablePRBonus)
		}
		if weights.DraftPRPenalty != -15 {
			t.Errorf("GetScoreWeights().DraftPRPenalty = %d, want -15", weights.DraftPRPenalty)
		}
		if weights.MaxAgeBonus != 30 {
			t.Errorf("GetScoreWeights().MaxAgeBonus = %d, want 30", weights.MaxAgeBonus)
		}
		if weights.HotTopicDisplayThreshold != 10 {
			t.Errorf("GetScoreWeights().HotTopicDisplayThreshold = %d, want 10", weights.HotTopicDisplayThreshold)
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

	// Labels no longer include hyphenated duplicates since matching
	// normalizes hyphens and spaces to be equivalent
	expectedLabels := []string{
		"good first issue",
		"help wanted",
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
