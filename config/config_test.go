package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
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

		got := defaultConfigDir()
		want := filepath.Join(customDir, "triage")

		if got != want {
			t.Errorf("defaultConfigDir() = %q, want %q", got, want)
		}
	})

	t.Run("uses default when XDG_CONFIG_HOME unset", func(t *testing.T) {
		// Save and restore original value
		original := os.Getenv("XDG_CONFIG_HOME")
		defer os.Setenv("XDG_CONFIG_HOME", original)

		os.Unsetenv("XDG_CONFIG_HOME")

		got := defaultConfigDir()
		home, _ := os.UserHomeDir()
		want := filepath.Join(home, ".config", "triage")

		if got != want {
			t.Errorf("defaultConfigDir() = %q, want %q", got, want)
		}
	})
}

func TestDefaultConfigDir_CrossPlatform(t *testing.T) {
	got := defaultConfigDir()

	// Should always end with "triage"
	if !strings.HasSuffix(got, "triage") {
		t.Errorf("defaultConfigDir() = %q, should end with 'triage'", got)
	}

	// Should be an absolute path (not the fallback)
	if got == ".triage" {
		t.Error("defaultConfigDir() returned fallback '.triage', expected absolute path")
	}
}

func TestLocalConfigPath(t *testing.T) {
	got := localConfigPath()
	want := ".triage.yaml"

	if got != want {
		t.Errorf("localConfigPath() = %q, want %q", got, want)
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

func TestGetConfigPaths(t *testing.T) {
	paths := GetConfigPaths()

	t.Run("returns valid global path", func(t *testing.T) {
		if paths.GlobalPath == "" {
			t.Error("GetConfigPaths().GlobalPath should not be empty")
		}
		if !strings.HasSuffix(paths.GlobalPath, "config.yaml") {
			t.Errorf("GetConfigPaths().GlobalPath = %q, should end with 'config.yaml'", paths.GlobalPath)
		}
	})

	t.Run("returns absolute local path", func(t *testing.T) {
		if paths.LocalPath == "" {
			t.Error("GetConfigPaths().LocalPath should not be empty")
		}
		if !filepath.IsAbs(paths.LocalPath) {
			t.Errorf("GetConfigPaths().LocalPath = %q, should be absolute", paths.LocalPath)
		}
		if !strings.HasSuffix(paths.LocalPath, ".triage.yaml") {
			t.Errorf("GetConfigPaths().LocalPath = %q, should end with '.triage.yaml'", paths.LocalPath)
		}
	})

	t.Run("detects file existence correctly", func(t *testing.T) {
		// Create a temp directory and change to it
		tmpDir := t.TempDir()
		original, _ := os.Getwd()
		defer os.Chdir(original)
		os.Chdir(tmpDir)

		// Initially no local config should exist
		pathsBefore := GetConfigPaths()
		if pathsBefore.LocalExists {
			t.Error("GetConfigPaths().LocalExists should be false when file doesn't exist")
		}

		// Create local config
		if err := os.WriteFile(".triage.yaml", []byte("default_format: table"), 0600); err != nil {
			t.Fatal(err)
		}

		// Now it should exist
		pathsAfter := GetConfigPaths()
		if !pathsAfter.LocalExists {
			t.Error("GetConfigPaths().LocalExists should be true when file exists")
		}
	})
}

func TestMinimalConfig(t *testing.T) {
	content := MinimalConfig()

	t.Run("contains expected sections", func(t *testing.T) {
		expectedStrings := []string{
			"# Triage configuration file",
			"default_format: table",
			"# exclude_repos:",
			"# base_scores:",
			"triage config defaults",
		}

		for _, expected := range expectedStrings {
			if !strings.Contains(content, expected) {
				t.Errorf("MinimalConfig() should contain %q", expected)
			}
		}
	})

	t.Run("is valid YAML", func(t *testing.T) {
		var cfg Config
		// The minimal config with comments should still be parseable
		// (YAML comments are ignored)
		if err := os.WriteFile(filepath.Join(t.TempDir(), "test.yaml"), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}

		// Parse it to verify it's valid YAML
		data := []byte(content)
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			t.Errorf("MinimalConfig() is not valid YAML: %v", err)
		}
	})
}

func TestSaveTo(t *testing.T) {
	t.Run("creates file in existing directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "test-config.yaml")
		content := "default_format: json"

		if err := SaveTo(path, content); err != nil {
			t.Fatalf("SaveTo() error = %v", err)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("Failed to read written file: %v", err)
		}

		if string(data) != content {
			t.Errorf("SaveTo() wrote %q, want %q", string(data), content)
		}
	})

	t.Run("creates nested directories", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "nested", "dirs", "config.yaml")
		content := "default_format: table"

		if err := SaveTo(path, content); err != nil {
			t.Fatalf("SaveTo() error = %v", err)
		}

		// Verify file exists and has correct content
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("Failed to read written file: %v", err)
		}

		if string(data) != content {
			t.Errorf("SaveTo() wrote %q, want %q", string(data), content)
		}
	})

	t.Run("sets appropriate permissions", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "secure-config.yaml")
		content := "secret: value"

		if err := SaveTo(path, content); err != nil {
			t.Fatalf("SaveTo() error = %v", err)
		}

		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Failed to stat written file: %v", err)
		}

		// File should be readable/writable only by owner (0600)
		perm := info.Mode().Perm()
		if perm != 0600 {
			t.Errorf("SaveTo() set permissions %o, want 0600", perm)
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

func TestDefaultScoreWeightsUrgency(t *testing.T) {
	weights := DefaultScoreWeights()

	if !weights.ReviewRequestedIsUrgent {
		t.Error("DefaultScoreWeights().ReviewRequestedIsUrgent should be true")
	}
	if weights.MentionIsUrgent {
		t.Error("DefaultScoreWeights().MentionIsUrgent should be false")
	}
	if !weights.ApprovedMergeablePRIsUrgent {
		t.Error("DefaultScoreWeights().ApprovedMergeablePRIsUrgent should be true")
	}
	if weights.ChangesRequestedPRIsUrgent {
		t.Error("DefaultScoreWeights().ChangesRequestedPRIsUrgent should be false")
	}
}

func TestGetScoreWeightsUrgency(t *testing.T) {
	t.Run("returns default urgency settings when no overrides", func(t *testing.T) {
		cfg := &Config{}
		weights := cfg.GetScoreWeights()

		if !weights.ReviewRequestedIsUrgent {
			t.Error("GetScoreWeights().ReviewRequestedIsUrgent should be true by default")
		}
		if weights.MentionIsUrgent {
			t.Error("GetScoreWeights().MentionIsUrgent should be false by default")
		}
		if !weights.ApprovedMergeablePRIsUrgent {
			t.Error("GetScoreWeights().ApprovedMergeablePRIsUrgent should be true by default")
		}
		if weights.ChangesRequestedPRIsUrgent {
			t.Error("GetScoreWeights().ChangesRequestedPRIsUrgent should be false by default")
		}
	})

	t.Run("applies urgency overrides", func(t *testing.T) {
		falseVal := false
		trueVal := true
		cfg := &Config{
			Urgency: &UrgencyOverrides{
				ReviewRequested:     &falseVal,
				Mention:             &trueVal,
				ApprovedMergeablePR: &falseVal,
			},
		}
		weights := cfg.GetScoreWeights()

		if weights.ReviewRequestedIsUrgent {
			t.Error("GetScoreWeights().ReviewRequestedIsUrgent should be false when overridden")
		}
		if !weights.MentionIsUrgent {
			t.Error("GetScoreWeights().MentionIsUrgent should be true when overridden")
		}
		if weights.ApprovedMergeablePRIsUrgent {
			t.Error("GetScoreWeights().ApprovedMergeablePRIsUrgent should be false when overridden")
		}
		// Non-overridden values should remain at defaults (false)
		if weights.ChangesRequestedPRIsUrgent {
			t.Error("GetScoreWeights().ChangesRequestedPRIsUrgent should be false (not overridden)")
		}
	})

	t.Run("all urgency triggers can be disabled", func(t *testing.T) {
		falseVal := false
		cfg := &Config{
			Urgency: &UrgencyOverrides{
				ReviewRequested:     &falseVal,
				Mention:             &falseVal,
				ApprovedMergeablePR: &falseVal,
				ChangesRequestedPR:  &falseVal,
			},
		}
		weights := cfg.GetScoreWeights()

		if weights.ReviewRequestedIsUrgent {
			t.Error("GetScoreWeights().ReviewRequestedIsUrgent should be false")
		}
		if weights.MentionIsUrgent {
			t.Error("GetScoreWeights().MentionIsUrgent should be false")
		}
		if weights.ApprovedMergeablePRIsUrgent {
			t.Error("GetScoreWeights().ApprovedMergeablePRIsUrgent should be false")
		}
		if weights.ChangesRequestedPRIsUrgent {
			t.Error("GetScoreWeights().ChangesRequestedPRIsUrgent should be false")
		}
	})
}

func TestMergeUrgencyOverrides(t *testing.T) {
	t.Run("returns nil when both nil", func(t *testing.T) {
		result := mergeUrgencyOverrides(nil, nil)
		if result != nil {
			t.Error("mergeUrgencyOverrides(nil, nil) should return nil")
		}
	})

	t.Run("local overrides global", func(t *testing.T) {
		trueVal := true
		falseVal := false
		global := &UrgencyOverrides{
			ReviewRequested: &trueVal,
			Mention:         &trueVal,
		}
		local := &UrgencyOverrides{
			ReviewRequested: &falseVal,
		}

		result := mergeUrgencyOverrides(global, local)

		if *result.ReviewRequested != false {
			t.Error("local should override global for ReviewRequested")
		}
		if *result.Mention != true {
			t.Error("global should be preserved for Mention")
		}
	})

	t.Run("preserves global when local is nil", func(t *testing.T) {
		trueVal := true
		global := &UrgencyOverrides{
			ReviewRequested: &trueVal,
		}

		result := mergeUrgencyOverrides(global, nil)

		if *result.ReviewRequested != true {
			t.Error("global value should be preserved")
		}
	})
}
