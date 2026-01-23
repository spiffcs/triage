package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	DefaultFormat string   `yaml:"default_format,omitempty"`
	ExcludeRepos  []string `yaml:"exclude_repos,omitempty"`

	// Priority weights (optional overrides)
	Weights *WeightOverrides `yaml:"weights,omitempty"`

	// Quick win label patterns (optional override)
	QuickWinLabels []string `yaml:"quick_win_labels,omitempty"`
}

// BaseScoreOverrides allows customizing base scores for notification reasons
type BaseScoreOverrides struct {
	ReviewRequested *int `yaml:"review_requested,omitempty"`
	Mention         *int `yaml:"mention,omitempty"`
	TeamMention     *int `yaml:"team_mention,omitempty"`
	Author          *int `yaml:"author,omitempty"`
	Assign          *int `yaml:"assign,omitempty"`
	Comment         *int `yaml:"comment,omitempty"`
	StateChange     *int `yaml:"state_change,omitempty"`
	Subscribed      *int `yaml:"subscribed,omitempty"`
	CIActivity      *int `yaml:"ci_activity,omitempty"`
}

// ModifierOverrides allows customizing score modifiers
type ModifierOverrides struct {
	OldUnreadBonus        *int `yaml:"old_unread_bonus,omitempty"`
	HotTopicBonus         *int `yaml:"hot_topic_bonus,omitempty"`
	HotTopicThreshold     *int `yaml:"hot_topic_threshold,omitempty"`
	LowHangingBonus       *int `yaml:"low_hanging_bonus,omitempty"`
	OpenStateBonus        *int `yaml:"open_state_bonus,omitempty"`
	ClosedStatePenalty    *int `yaml:"closed_state_penalty,omitempty"`
	FYIPromotionThreshold *int `yaml:"fyi_promotion_threshold,omitempty"`
}

// WeightOverrides allows customizing priority weights
type WeightOverrides struct {
	BaseScores *BaseScoreOverrides `yaml:"base_scores,omitempty"`
	Modifiers  *ModifierOverrides  `yaml:"modifiers,omitempty"`
}

// ScoreWeights defines the complete set of scoring weights
type ScoreWeights struct {
	ReviewRequested int
	Mention         int
	TeamMention     int
	Author          int
	Assign          int
	Comment         int
	Subscribed      int
	StateChange     int
	CIActivity      int

	OldUnreadBonus        int
	HotTopicBonus         int
	HotTopicThreshold     int
	LowHangingBonus       int
	OpenStateBonus        int
	ClosedStatePenalty    int
	FYIPromotionThreshold int
}

// DefaultScoreWeights returns the default scoring weights
func DefaultScoreWeights() ScoreWeights {
	return ScoreWeights{
		ReviewRequested: 100,
		Mention:         90,
		TeamMention:     85,
		Author:          70,
		Assign:          60,
		Comment:         30,
		StateChange:     25,
		Subscribed:      10,
		CIActivity:      5,

		OldUnreadBonus:        2,
		HotTopicBonus:         15,
		HotTopicThreshold:     6,
		LowHangingBonus:       20,
		OpenStateBonus:        10,
		ClosedStatePenalty:    -30,
		FYIPromotionThreshold: 55,
	}
}

// GetScoreWeights returns score weights with user overrides merged with defaults
func (c *Config) GetScoreWeights() ScoreWeights {
	weights := DefaultScoreWeights()

	if c.Weights == nil {
		return weights
	}

	// Apply base score overrides
	if c.Weights.BaseScores != nil {
		bs := c.Weights.BaseScores
		if bs.ReviewRequested != nil {
			weights.ReviewRequested = *bs.ReviewRequested
		}
		if bs.Mention != nil {
			weights.Mention = *bs.Mention
		}
		if bs.TeamMention != nil {
			weights.TeamMention = *bs.TeamMention
		}
		if bs.Author != nil {
			weights.Author = *bs.Author
		}
		if bs.Assign != nil {
			weights.Assign = *bs.Assign
		}
		if bs.Comment != nil {
			weights.Comment = *bs.Comment
		}
		if bs.StateChange != nil {
			weights.StateChange = *bs.StateChange
		}
		if bs.Subscribed != nil {
			weights.Subscribed = *bs.Subscribed
		}
		if bs.CIActivity != nil {
			weights.CIActivity = *bs.CIActivity
		}
	}

	// Apply modifier overrides
	if c.Weights.Modifiers != nil {
		m := c.Weights.Modifiers
		if m.OldUnreadBonus != nil {
			weights.OldUnreadBonus = *m.OldUnreadBonus
		}
		if m.HotTopicBonus != nil {
			weights.HotTopicBonus = *m.HotTopicBonus
		}
		if m.HotTopicThreshold != nil {
			weights.HotTopicThreshold = *m.HotTopicThreshold
		}
		if m.LowHangingBonus != nil {
			weights.LowHangingBonus = *m.LowHangingBonus
		}
		if m.OpenStateBonus != nil {
			weights.OpenStateBonus = *m.OpenStateBonus
		}
		if m.ClosedStatePenalty != nil {
			weights.ClosedStatePenalty = *m.ClosedStatePenalty
		}
		if m.FYIPromotionThreshold != nil {
			weights.FYIPromotionThreshold = *m.FYIPromotionThreshold
		}
	}

	return weights
}

// DefaultConfigDir returns the default config directory
func DefaultConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".triage"
	}
	return filepath.Join(home, ".config", "triage")
}

// ConfigPath returns the path to the config file
func ConfigPath() string {
	return filepath.Join(DefaultConfigDir(), "config.yaml")
}

// ConfigFileExists returns true if the config file exists on disk
func ConfigFileExists() bool {
	_, err := os.Stat(ConfigPath())
	return err == nil
}

// Load loads the configuration from disk
func Load() (*Config, error) {
	configPath := ConfigPath()

	// Check if file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return &Config{
			DefaultFormat: "table",
		}, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Set defaults
	if cfg.DefaultFormat == "" {
		cfg.DefaultFormat = "table"
	}

	return &cfg, nil
}

// Save saves the configuration to disk
func (c *Config) Save() error {
	configDir := DefaultConfigDir()

	// Create config directory if it doesn't exist
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	configPath := ConfigPath()
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// GetGitHubToken returns the GitHub token from the GITHUB_TOKEN environment variable.
// Following 12-factor app best practices, tokens are only read from the environment.
func (c *Config) GetGitHubToken() string {
	return os.Getenv("GITHUB_TOKEN")
}

// SetDefaultFormat sets the default output format and saves
func (c *Config) SetDefaultFormat(format string) error {
	c.DefaultFormat = format
	return c.Save()
}

// IsRepoExcluded checks if a repo is in the exclude list
func (c *Config) IsRepoExcluded(repoFullName string) bool {
	for _, excluded := range c.ExcludeRepos {
		if excluded == repoFullName {
			return true
		}
	}
	return false
}

// DefaultQuickWinLabels returns the default labels that indicate quick wins.
// Labels are matched case-insensitively and hyphens/spaces are treated as equivalent,
// so "good first issue" will match "good-first-issue", "Good First Issue", etc.
func DefaultQuickWinLabels() []string {
	return []string{
		"good first issue",
		"help wanted",
		"easy",
		"beginner",
		"trivial",
		"documentation",
		"docs",
		"typo",
	}
}

// GetQuickWinLabels returns the quick win labels, using defaults if not configured
func (c *Config) GetQuickWinLabels() []string {
	if len(c.QuickWinLabels) > 0 {
		return c.QuickWinLabels
	}
	return DefaultQuickWinLabels()
}
