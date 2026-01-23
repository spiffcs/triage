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

	// Authored PR modifiers
	ApprovedPRBonus       *int `yaml:"approved_pr_bonus,omitempty"`
	MergeablePRBonus      *int `yaml:"mergeable_pr_bonus,omitempty"`
	ChangesRequestedBonus *int `yaml:"changes_requested_bonus,omitempty"`
	ReviewCommentBonus    *int `yaml:"review_comment_bonus,omitempty"`
	ReviewCommentMaxBonus *int `yaml:"review_comment_max_bonus,omitempty"`
	StalePRThresholdDays  *int `yaml:"stale_pr_threshold_days,omitempty"`
	StalePRBonusPerDay    *int `yaml:"stale_pr_bonus_per_day,omitempty"`
	StalePRMaxBonus       *int `yaml:"stale_pr_max_bonus,omitempty"`
	DraftPRPenalty        *int `yaml:"draft_pr_penalty,omitempty"`

	// General scoring
	MaxAgeBonus *int `yaml:"max_age_bonus,omitempty"`

	// Low-hanging fruit detection
	SmallPRMaxFiles *int `yaml:"small_pr_max_files,omitempty"`
	SmallPRMaxLines *int `yaml:"small_pr_max_lines,omitempty"`

	// Display threshold for fire emoji (separate from scoring threshold)
	HotTopicDisplayThreshold *int `yaml:"hot_topic_display_threshold,omitempty"`

	// PR size thresholds for T-shirt sizing
	PRSizeXS *int `yaml:"pr_size_xs,omitempty"`
	PRSizeS  *int `yaml:"pr_size_s,omitempty"`
	PRSizeM  *int `yaml:"pr_size_m,omitempty"`
	PRSizeL  *int `yaml:"pr_size_l,omitempty"`
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

	// Authored PR modifiers
	ApprovedPRBonus       int
	MergeablePRBonus      int
	ChangesRequestedBonus int
	ReviewCommentBonus    int
	ReviewCommentMaxBonus int
	StalePRThresholdDays  int
	StalePRBonusPerDay    int
	StalePRMaxBonus       int
	DraftPRPenalty        int

	// General scoring
	MaxAgeBonus int

	// Low-hanging fruit detection
	SmallPRMaxFiles int
	SmallPRMaxLines int

	// Display threshold for fire emoji
	HotTopicDisplayThreshold int

	// PR size thresholds for T-shirt sizing
	PRSizeXS int // <= this = XS
	PRSizeS  int // <= this = S
	PRSizeM  int // <= this = M
	PRSizeL  int // > M and <= this = L (> this = XL)
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

		// Authored PR modifiers
		ApprovedPRBonus:       25,
		MergeablePRBonus:      15,
		ChangesRequestedBonus: 20,
		ReviewCommentBonus:    3,
		ReviewCommentMaxBonus: 15,
		StalePRThresholdDays:  7,
		StalePRBonusPerDay:    2,
		StalePRMaxBonus:       20,
		DraftPRPenalty:        -15,

		// General scoring
		MaxAgeBonus: 30,

		// Low-hanging fruit detection
		SmallPRMaxFiles: 3,
		SmallPRMaxLines: 50,

		// Display threshold for fire emoji
		HotTopicDisplayThreshold: 10,

		// PR size thresholds for T-shirt sizing
		PRSizeXS: 10,
		PRSizeS:  50,
		PRSizeM:  200,
		PRSizeL:  500,
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

		// Authored PR modifiers
		if m.ApprovedPRBonus != nil {
			weights.ApprovedPRBonus = *m.ApprovedPRBonus
		}
		if m.MergeablePRBonus != nil {
			weights.MergeablePRBonus = *m.MergeablePRBonus
		}
		if m.ChangesRequestedBonus != nil {
			weights.ChangesRequestedBonus = *m.ChangesRequestedBonus
		}
		if m.ReviewCommentBonus != nil {
			weights.ReviewCommentBonus = *m.ReviewCommentBonus
		}
		if m.ReviewCommentMaxBonus != nil {
			weights.ReviewCommentMaxBonus = *m.ReviewCommentMaxBonus
		}
		if m.StalePRThresholdDays != nil {
			weights.StalePRThresholdDays = *m.StalePRThresholdDays
		}
		if m.StalePRBonusPerDay != nil {
			weights.StalePRBonusPerDay = *m.StalePRBonusPerDay
		}
		if m.StalePRMaxBonus != nil {
			weights.StalePRMaxBonus = *m.StalePRMaxBonus
		}
		if m.DraftPRPenalty != nil {
			weights.DraftPRPenalty = *m.DraftPRPenalty
		}

		// General scoring
		if m.MaxAgeBonus != nil {
			weights.MaxAgeBonus = *m.MaxAgeBonus
		}

		// Low-hanging fruit detection
		if m.SmallPRMaxFiles != nil {
			weights.SmallPRMaxFiles = *m.SmallPRMaxFiles
		}
		if m.SmallPRMaxLines != nil {
			weights.SmallPRMaxLines = *m.SmallPRMaxLines
		}

		// Display threshold
		if m.HotTopicDisplayThreshold != nil {
			weights.HotTopicDisplayThreshold = *m.HotTopicDisplayThreshold
		}

		// PR size thresholds
		if m.PRSizeXS != nil {
			weights.PRSizeXS = *m.PRSizeXS
		}
		if m.PRSizeS != nil {
			weights.PRSizeS = *m.PRSizeS
		}
		if m.PRSizeM != nil {
			weights.PRSizeM = *m.PRSizeM
		}
		if m.PRSizeL != nil {
			weights.PRSizeL = *m.PRSizeL
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
		"good-first-issue",
		"help-wanted",
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

// DefaultConfig returns a fully populated config with all default values.
// This is useful for generating a complete config file template.
func DefaultConfig() *Config {
	weights := DefaultScoreWeights()
	labels := DefaultQuickWinLabels()

	return &Config{
		DefaultFormat:  "table",
		ExcludeRepos:   []string{},
		QuickWinLabels: labels,
		Weights: &WeightOverrides{
			BaseScores: &BaseScoreOverrides{
				ReviewRequested: &weights.ReviewRequested,
				Mention:         &weights.Mention,
				TeamMention:     &weights.TeamMention,
				Author:          &weights.Author,
				Assign:          &weights.Assign,
				Comment:         &weights.Comment,
				StateChange:     &weights.StateChange,
				Subscribed:      &weights.Subscribed,
				CIActivity:      &weights.CIActivity,
			},
			Modifiers: &ModifierOverrides{
				OldUnreadBonus:        &weights.OldUnreadBonus,
				HotTopicBonus:         &weights.HotTopicBonus,
				HotTopicThreshold:     &weights.HotTopicThreshold,
				LowHangingBonus:       &weights.LowHangingBonus,
				OpenStateBonus:        &weights.OpenStateBonus,
				ClosedStatePenalty:    &weights.ClosedStatePenalty,
				FYIPromotionThreshold: &weights.FYIPromotionThreshold,

				// Authored PR modifiers
				ApprovedPRBonus:       &weights.ApprovedPRBonus,
				MergeablePRBonus:      &weights.MergeablePRBonus,
				ChangesRequestedBonus: &weights.ChangesRequestedBonus,
				ReviewCommentBonus:    &weights.ReviewCommentBonus,
				ReviewCommentMaxBonus: &weights.ReviewCommentMaxBonus,
				StalePRThresholdDays:  &weights.StalePRThresholdDays,
				StalePRBonusPerDay:    &weights.StalePRBonusPerDay,
				StalePRMaxBonus:       &weights.StalePRMaxBonus,
				DraftPRPenalty:        &weights.DraftPRPenalty,

				// General scoring
				MaxAgeBonus: &weights.MaxAgeBonus,

				// Low-hanging fruit detection
				SmallPRMaxFiles: &weights.SmallPRMaxFiles,
				SmallPRMaxLines: &weights.SmallPRMaxLines,

				// Display threshold
				HotTopicDisplayThreshold: &weights.HotTopicDisplayThreshold,

				// PR size thresholds for T-shirt sizing
				PRSizeXS: &weights.PRSizeXS,
				PRSizeS:  &weights.PRSizeS,
				PRSizeM:  &weights.PRSizeM,
				PRSizeL:  &weights.PRSizeL,
			},
		},
	}
}

// ToYAML returns the config as a YAML string
func (c *Config) ToYAML() (string, error) {
	data, err := yaml.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("failed to marshal config: %w", err)
	}
	return string(data), nil
}
