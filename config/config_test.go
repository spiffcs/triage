package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
		{"DraftPRPenalty", weights.DraftPRPenalty, -25},
		// General scoring
		{"MaxAgeBonus", weights.MaxAgeBonus, 30},
		// Low-hanging fruit detection
		{"SmallPRMaxFiles", weights.SmallPRMaxFiles, 5},
		{"SmallPRMaxLines", weights.SmallPRMaxLines, 100},
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
			BaseScores: &BaseScoreOverrides{
				ReviewRequested: &overriddenValue,
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

	t.Run("merges partial scoring overrides", func(t *testing.T) {
		overriddenValue := 50
		cfg := &Config{
			Scoring: &ScoringOverrides{
				HotTopicBonus: &overriddenValue,
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

	t.Run("merges PR overrides", func(t *testing.T) {
		approvedBonus := 50
		staleDays := 5
		smallFiles := 10
		cfg := &Config{
			PR: &PROverrides{
				ApprovedBonus:      &approvedBonus,
				StaleThresholdDays: &staleDays,
				SmallMaxFiles:      &smallFiles,
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
		if weights.DraftPRPenalty != -25 {
			t.Errorf("GetScoreWeights().DraftPRPenalty = %d, want -25", weights.DraftPRPenalty)
		}
		if weights.MaxAgeBonus != 30 {
			t.Errorf("GetScoreWeights().MaxAgeBonus = %d, want 30", weights.MaxAgeBonus)
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
			if label == "good-first-issue" {
				found = true
				break
			}
		}
		if !found {
			t.Error("GetQuickWinLabels() should contain 'good-first-issue' by default")
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

	// Labels use hyphenated format; matching normalizes hyphens and spaces
	expectedLabels := []string{
		"good-first-issue",
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

func TestDefaultConfigDir_XDG(t *testing.T) {
	// Only test XDG on Linux where it's the standard
	if runtime.GOOS != "linux" {
		t.Skip("XDG_CONFIG_HOME is only used on Linux")
	}

	t.Run("respects XDG_CONFIG_HOME", func(t *testing.T) {
		// Save and restore original value
		original := os.Getenv("XDG_CONFIG_HOME")
		defer os.Setenv("XDG_CONFIG_HOME", original)

		customDir := "/tmp/custom-xdg-config"
		os.Setenv("XDG_CONFIG_HOME", customDir)

		got := DefaultConfigDir()
		want := filepath.Join(customDir, "triage")

		if got != want {
			t.Errorf("DefaultConfigDir() = %q, want %q", got, want)
		}
	})

	t.Run("uses default when XDG_CONFIG_HOME unset", func(t *testing.T) {
		// Save and restore original value
		original := os.Getenv("XDG_CONFIG_HOME")
		defer os.Setenv("XDG_CONFIG_HOME", original)

		os.Unsetenv("XDG_CONFIG_HOME")

		got := DefaultConfigDir()
		home, _ := os.UserHomeDir()
		want := filepath.Join(home, ".config", "triage")

		if got != want {
			t.Errorf("DefaultConfigDir() = %q, want %q", got, want)
		}
	})
}

func TestDefaultConfigDir_CrossPlatform(t *testing.T) {
	got := DefaultConfigDir()

	// Should always end with "triage"
	if !strings.HasSuffix(got, "triage") {
		t.Errorf("DefaultConfigDir() = %q, should end with 'triage'", got)
	}

	// Should be an absolute path (not the fallback)
	if got == ".triage" {
		t.Error("DefaultConfigDir() returned fallback '.triage', expected absolute path")
	}
}

func TestLocalConfigPath(t *testing.T) {
	got := LocalConfigPath()
	want := ".triage.yaml"

	if got != want {
		t.Errorf("LocalConfigPath() = %q, want %q", got, want)
	}
}

func TestLoad_LocalConfigOverride(t *testing.T) {
	// Create a temp directory and change to it
	tmpDir := t.TempDir()
	original, _ := os.Getwd()
	defer os.Chdir(original)
	os.Chdir(tmpDir)

	t.Run("loads local config when present", func(t *testing.T) {
		// Create local config with custom value
		localConfig := `default_format: json
quick_win_labels:
  - local-label
`
		if err := os.WriteFile(".triage.yaml", []byte(localConfig), 0600); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(".triage.yaml")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if cfg.DefaultFormat != "json" {
			t.Errorf("Load().DefaultFormat = %q, want 'json'", cfg.DefaultFormat)
		}
		if len(cfg.QuickWinLabels) != 1 || cfg.QuickWinLabels[0] != "local-label" {
			t.Errorf("Load().QuickWinLabels = %v, want ['local-label']", cfg.QuickWinLabels)
		}
	})

	t.Run("returns defaults when no config exists", func(t *testing.T) {
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if cfg.DefaultFormat != "table" {
			t.Errorf("Load().DefaultFormat = %q, want 'table'", cfg.DefaultFormat)
		}
	})
}

func TestMergeConfig(t *testing.T) {
	t.Run("local values override global", func(t *testing.T) {
		globalVal := 50
		localVal := 100
		global := &Config{
			DefaultFormat: "table",
			BaseScores: &BaseScoreOverrides{
				ReviewRequested: &globalVal,
				Mention:         &globalVal,
			},
		}
		local := &Config{
			DefaultFormat: "json",
			BaseScores: &BaseScoreOverrides{
				ReviewRequested: &localVal,
			},
		}

		result := mergeConfig(global, local)

		if result.DefaultFormat != "json" {
			t.Errorf("mergeConfig().DefaultFormat = %q, want 'json'", result.DefaultFormat)
		}
		if *result.BaseScores.ReviewRequested != 100 {
			t.Errorf("mergeConfig().BaseScores.ReviewRequested = %d, want 100", *result.BaseScores.ReviewRequested)
		}
		// Global value should be preserved when not overridden
		if *result.BaseScores.Mention != 50 {
			t.Errorf("mergeConfig().BaseScores.Mention = %d, want 50", *result.BaseScores.Mention)
		}
	})

	t.Run("local arrays replace global arrays", func(t *testing.T) {
		global := &Config{
			ExcludeRepos:   []string{"global/repo1", "global/repo2"},
			QuickWinLabels: []string{"global-label"},
		}
		local := &Config{
			ExcludeRepos: []string{"local/repo"},
		}

		result := mergeConfig(global, local)

		if len(result.ExcludeRepos) != 1 || result.ExcludeRepos[0] != "local/repo" {
			t.Errorf("mergeConfig().ExcludeRepos = %v, want ['local/repo']", result.ExcludeRepos)
		}
		// Unset arrays should preserve global
		if len(result.QuickWinLabels) != 1 || result.QuickWinLabels[0] != "global-label" {
			t.Errorf("mergeConfig().QuickWinLabels = %v, want ['global-label']", result.QuickWinLabels)
		}
	})
}
