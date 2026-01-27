package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	DefaultFormat  string   `yaml:"default_format,omitempty"`
	ExcludeRepos   []string `yaml:"exclude_repos,omitempty"`
	ExcludeAuthors []string `yaml:"exclude_authors,omitempty"`
	QuickWinLabels []string `yaml:"quick_win_labels,omitempty"`

	// Top-level config sections
	BaseScores *BaseScoreOverrides `yaml:"base_scores,omitempty"`
	Scoring    *ScoringOverrides   `yaml:"scoring,omitempty"`
	PR         *PROverrides        `yaml:"pr,omitempty"`
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

// ScoringOverrides - general scoring modifiers
type ScoringOverrides struct {
	OldUnreadBonus              *int `yaml:"old_unread_bonus,omitempty"`
	MaxAgeBonus                 *int `yaml:"max_age_bonus,omitempty"`
	HotTopicBonus               *int `yaml:"hot_topic_bonus,omitempty"`
	HotTopicThreshold           *int `yaml:"hot_topic_threshold,omitempty"`
	FYIPromotionThreshold       *int `yaml:"fyi_promotion_threshold,omitempty"`
	NotablePromotionThreshold   *int `yaml:"notable_promotion_threshold,omitempty"`
	ImportantPromotionThreshold *int `yaml:"important_promotion_threshold,omitempty"`
	OpenStateBonus              *int `yaml:"open_state_bonus,omitempty"`
	ClosedStatePenalty          *int `yaml:"closed_state_penalty,omitempty"`
	LowHangingBonus             *int `yaml:"low_hanging_bonus,omitempty"`
}

// PROverrides - PR-specific settings
type PROverrides struct {
	ApprovedBonus         *int `yaml:"approved_bonus,omitempty"`
	MergeableBonus        *int `yaml:"mergeable_bonus,omitempty"`
	ChangesRequestedBonus *int `yaml:"changes_requested_bonus,omitempty"`
	ReviewCommentBonus    *int `yaml:"review_comment_bonus,omitempty"`
	ReviewCommentMaxBonus *int `yaml:"review_comment_max_bonus,omitempty"`
	StaleThresholdDays    *int `yaml:"stale_threshold_days,omitempty"`
	StaleBonusPerDay      *int `yaml:"stale_bonus_per_day,omitempty"`
	StaleMaxBonus         *int `yaml:"stale_max_bonus,omitempty"`
	DraftPenalty          *int `yaml:"draft_penalty,omitempty"`
	SmallMaxFiles         *int `yaml:"small_max_files,omitempty"`
	SmallMaxLines         *int `yaml:"small_max_lines,omitempty"`
	SizeXS                *int `yaml:"size_xs,omitempty"`
	SizeS                 *int `yaml:"size_s,omitempty"`
	SizeM                 *int `yaml:"size_m,omitempty"`
	SizeL                 *int `yaml:"size_l,omitempty"`
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

	OldUnreadBonus              int
	HotTopicBonus               int
	HotTopicThreshold           int
	LowHangingBonus             int
	OpenStateBonus              int
	ClosedStatePenalty          int
	FYIPromotionThreshold       int
	NotablePromotionThreshold   int
	ImportantPromotionThreshold int

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

		OldUnreadBonus:              2,
		HotTopicBonus:               15,
		HotTopicThreshold:           7,
		LowHangingBonus:             20,
		OpenStateBonus:              10,
		ClosedStatePenalty:          -30,
		FYIPromotionThreshold:       35,  // FYI → Notable
		NotablePromotionThreshold:   60,  // Notable → Important
		ImportantPromotionThreshold: 100, // Important → Urgent

		// Authored PR modifiers
		ApprovedPRBonus:       25,
		MergeablePRBonus:      15,
		ChangesRequestedBonus: 20,
		ReviewCommentBonus:    3,
		ReviewCommentMaxBonus: 15,
		StalePRThresholdDays:  7,
		StalePRBonusPerDay:    2,
		StalePRMaxBonus:       20,
		DraftPRPenalty:        -25,

		// General scoring
		MaxAgeBonus: 30,

		// Low-hanging fruit detection
		SmallPRMaxFiles: 5,
		SmallPRMaxLines: 100,

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

	// Apply base score overrides
	if c.BaseScores != nil {
		bs := c.BaseScores
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

	// Apply scoring overrides
	if c.Scoring != nil {
		s := c.Scoring
		if s.OldUnreadBonus != nil {
			weights.OldUnreadBonus = *s.OldUnreadBonus
		}
		if s.MaxAgeBonus != nil {
			weights.MaxAgeBonus = *s.MaxAgeBonus
		}
		if s.HotTopicBonus != nil {
			weights.HotTopicBonus = *s.HotTopicBonus
		}
		if s.HotTopicThreshold != nil {
			weights.HotTopicThreshold = *s.HotTopicThreshold
		}
		if s.FYIPromotionThreshold != nil {
			weights.FYIPromotionThreshold = *s.FYIPromotionThreshold
		}
		if s.NotablePromotionThreshold != nil {
			weights.NotablePromotionThreshold = *s.NotablePromotionThreshold
		}
		if s.ImportantPromotionThreshold != nil {
			weights.ImportantPromotionThreshold = *s.ImportantPromotionThreshold
		}
		if s.OpenStateBonus != nil {
			weights.OpenStateBonus = *s.OpenStateBonus
		}
		if s.ClosedStatePenalty != nil {
			weights.ClosedStatePenalty = *s.ClosedStatePenalty
		}
		if s.LowHangingBonus != nil {
			weights.LowHangingBonus = *s.LowHangingBonus
		}
	}

	// Apply PR-specific overrides
	if c.PR != nil {
		pr := c.PR
		if pr.ApprovedBonus != nil {
			weights.ApprovedPRBonus = *pr.ApprovedBonus
		}
		if pr.MergeableBonus != nil {
			weights.MergeablePRBonus = *pr.MergeableBonus
		}
		if pr.ChangesRequestedBonus != nil {
			weights.ChangesRequestedBonus = *pr.ChangesRequestedBonus
		}
		if pr.ReviewCommentBonus != nil {
			weights.ReviewCommentBonus = *pr.ReviewCommentBonus
		}
		if pr.ReviewCommentMaxBonus != nil {
			weights.ReviewCommentMaxBonus = *pr.ReviewCommentMaxBonus
		}
		if pr.StaleThresholdDays != nil {
			weights.StalePRThresholdDays = *pr.StaleThresholdDays
		}
		if pr.StaleBonusPerDay != nil {
			weights.StalePRBonusPerDay = *pr.StaleBonusPerDay
		}
		if pr.StaleMaxBonus != nil {
			weights.StalePRMaxBonus = *pr.StaleMaxBonus
		}
		if pr.DraftPenalty != nil {
			weights.DraftPRPenalty = *pr.DraftPenalty
		}
		if pr.SmallMaxFiles != nil {
			weights.SmallPRMaxFiles = *pr.SmallMaxFiles
		}
		if pr.SmallMaxLines != nil {
			weights.SmallPRMaxLines = *pr.SmallMaxLines
		}
		if pr.SizeXS != nil {
			weights.PRSizeXS = *pr.SizeXS
		}
		if pr.SizeS != nil {
			weights.PRSizeS = *pr.SizeS
		}
		if pr.SizeM != nil {
			weights.PRSizeM = *pr.SizeM
		}
		if pr.SizeL != nil {
			weights.PRSizeL = *pr.SizeL
		}
	}

	return weights
}

// DefaultConfigDir returns the default config directory
func DefaultConfigDir() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return ".triage"
	}
	return filepath.Join(configDir, "triage")
}

// ConfigPath returns the path to the config file
func ConfigPath() string {
	return filepath.Join(DefaultConfigDir(), "config.yaml")
}

// LocalConfigPath returns the path to the local config file in the current directory
func LocalConfigPath() string {
	return ".triage.yaml"
}

// ConfigFileExists returns true if the config file exists on disk
func ConfigFileExists() bool {
	_, err := os.Stat(ConfigPath())
	return err == nil
}

// Load loads the configuration from disk.
// It first loads the global config from XDG config directory, then merges
// any local .triage.yaml config on top (local values take precedence).
func Load() (*Config, error) {
	// Start with defaults
	cfg := &Config{
		DefaultFormat: "table",
	}

	// Load global config if it exists
	globalPath := ConfigPath()
	if _, err := os.Stat(globalPath); err == nil {
		data, err := os.ReadFile(globalPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read global config file: %w", err)
		}

		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse global config file: %w", err)
		}
	}

	// Load local config if it exists and merge on top
	localPath := LocalConfigPath()
	if _, err := os.Stat(localPath); err == nil {
		data, err := os.ReadFile(localPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read local config file: %w", err)
		}

		var localCfg Config
		if err := yaml.Unmarshal(data, &localCfg); err != nil {
			return nil, fmt.Errorf("failed to parse local config file: %w", err)
		}

		cfg = mergeConfig(cfg, &localCfg)
	}

	// Set defaults if still empty
	if cfg.DefaultFormat == "" {
		cfg.DefaultFormat = "table"
	}

	return cfg, nil
}

// mergeConfig merges local config on top of global config.
// Local values take precedence; unset local values preserve global values.
func mergeConfig(global, local *Config) *Config {
	result := &Config{}

	// Merge simple fields (local wins if set)
	if local.DefaultFormat != "" {
		result.DefaultFormat = local.DefaultFormat
	} else {
		result.DefaultFormat = global.DefaultFormat
	}

	// Merge arrays (local replaces if non-empty)
	if len(local.ExcludeRepos) > 0 {
		result.ExcludeRepos = local.ExcludeRepos
	} else {
		result.ExcludeRepos = global.ExcludeRepos
	}

	if len(local.ExcludeAuthors) > 0 {
		result.ExcludeAuthors = local.ExcludeAuthors
	} else {
		result.ExcludeAuthors = global.ExcludeAuthors
	}

	if len(local.QuickWinLabels) > 0 {
		result.QuickWinLabels = local.QuickWinLabels
	} else {
		result.QuickWinLabels = global.QuickWinLabels
	}

	// Merge BaseScores
	result.BaseScores = mergeBaseScores(global.BaseScores, local.BaseScores)

	// Merge Scoring
	result.Scoring = mergeScoringOverrides(global.Scoring, local.Scoring)

	// Merge PR
	result.PR = mergePROverrides(global.PR, local.PR)

	return result
}

func mergeBaseScores(global, local *BaseScoreOverrides) *BaseScoreOverrides {
	if global == nil && local == nil {
		return nil
	}
	result := &BaseScoreOverrides{}

	if global != nil {
		result.ReviewRequested = global.ReviewRequested
		result.Mention = global.Mention
		result.TeamMention = global.TeamMention
		result.Author = global.Author
		result.Assign = global.Assign
		result.Comment = global.Comment
		result.StateChange = global.StateChange
		result.Subscribed = global.Subscribed
		result.CIActivity = global.CIActivity
	}

	if local != nil {
		if local.ReviewRequested != nil {
			result.ReviewRequested = local.ReviewRequested
		}
		if local.Mention != nil {
			result.Mention = local.Mention
		}
		if local.TeamMention != nil {
			result.TeamMention = local.TeamMention
		}
		if local.Author != nil {
			result.Author = local.Author
		}
		if local.Assign != nil {
			result.Assign = local.Assign
		}
		if local.Comment != nil {
			result.Comment = local.Comment
		}
		if local.StateChange != nil {
			result.StateChange = local.StateChange
		}
		if local.Subscribed != nil {
			result.Subscribed = local.Subscribed
		}
		if local.CIActivity != nil {
			result.CIActivity = local.CIActivity
		}
	}

	// Return nil if all fields are nil
	if result.ReviewRequested == nil && result.Mention == nil && result.TeamMention == nil &&
		result.Author == nil && result.Assign == nil && result.Comment == nil &&
		result.StateChange == nil && result.Subscribed == nil && result.CIActivity == nil {
		return nil
	}

	return result
}

func mergeScoringOverrides(global, local *ScoringOverrides) *ScoringOverrides {
	if global == nil && local == nil {
		return nil
	}
	result := &ScoringOverrides{}

	if global != nil {
		result.OldUnreadBonus = global.OldUnreadBonus
		result.MaxAgeBonus = global.MaxAgeBonus
		result.HotTopicBonus = global.HotTopicBonus
		result.HotTopicThreshold = global.HotTopicThreshold
		result.FYIPromotionThreshold = global.FYIPromotionThreshold
		result.NotablePromotionThreshold = global.NotablePromotionThreshold
		result.ImportantPromotionThreshold = global.ImportantPromotionThreshold
		result.OpenStateBonus = global.OpenStateBonus
		result.ClosedStatePenalty = global.ClosedStatePenalty
		result.LowHangingBonus = global.LowHangingBonus
	}

	if local != nil {
		if local.OldUnreadBonus != nil {
			result.OldUnreadBonus = local.OldUnreadBonus
		}
		if local.MaxAgeBonus != nil {
			result.MaxAgeBonus = local.MaxAgeBonus
		}
		if local.HotTopicBonus != nil {
			result.HotTopicBonus = local.HotTopicBonus
		}
		if local.HotTopicThreshold != nil {
			result.HotTopicThreshold = local.HotTopicThreshold
		}
		if local.FYIPromotionThreshold != nil {
			result.FYIPromotionThreshold = local.FYIPromotionThreshold
		}
		if local.NotablePromotionThreshold != nil {
			result.NotablePromotionThreshold = local.NotablePromotionThreshold
		}
		if local.ImportantPromotionThreshold != nil {
			result.ImportantPromotionThreshold = local.ImportantPromotionThreshold
		}
		if local.OpenStateBonus != nil {
			result.OpenStateBonus = local.OpenStateBonus
		}
		if local.ClosedStatePenalty != nil {
			result.ClosedStatePenalty = local.ClosedStatePenalty
		}
		if local.LowHangingBonus != nil {
			result.LowHangingBonus = local.LowHangingBonus
		}
	}

	// Return nil if all fields are nil
	if result.OldUnreadBonus == nil && result.MaxAgeBonus == nil && result.HotTopicBonus == nil &&
		result.HotTopicThreshold == nil && result.FYIPromotionThreshold == nil &&
		result.NotablePromotionThreshold == nil && result.ImportantPromotionThreshold == nil &&
		result.OpenStateBonus == nil && result.ClosedStatePenalty == nil && result.LowHangingBonus == nil {
		return nil
	}

	return result
}

func mergePROverrides(global, local *PROverrides) *PROverrides {
	if global == nil && local == nil {
		return nil
	}
	result := &PROverrides{}

	if global != nil {
		result.ApprovedBonus = global.ApprovedBonus
		result.MergeableBonus = global.MergeableBonus
		result.ChangesRequestedBonus = global.ChangesRequestedBonus
		result.ReviewCommentBonus = global.ReviewCommentBonus
		result.ReviewCommentMaxBonus = global.ReviewCommentMaxBonus
		result.StaleThresholdDays = global.StaleThresholdDays
		result.StaleBonusPerDay = global.StaleBonusPerDay
		result.StaleMaxBonus = global.StaleMaxBonus
		result.DraftPenalty = global.DraftPenalty
		result.SmallMaxFiles = global.SmallMaxFiles
		result.SmallMaxLines = global.SmallMaxLines
		result.SizeXS = global.SizeXS
		result.SizeS = global.SizeS
		result.SizeM = global.SizeM
		result.SizeL = global.SizeL
	}

	if local != nil {
		if local.ApprovedBonus != nil {
			result.ApprovedBonus = local.ApprovedBonus
		}
		if local.MergeableBonus != nil {
			result.MergeableBonus = local.MergeableBonus
		}
		if local.ChangesRequestedBonus != nil {
			result.ChangesRequestedBonus = local.ChangesRequestedBonus
		}
		if local.ReviewCommentBonus != nil {
			result.ReviewCommentBonus = local.ReviewCommentBonus
		}
		if local.ReviewCommentMaxBonus != nil {
			result.ReviewCommentMaxBonus = local.ReviewCommentMaxBonus
		}
		if local.StaleThresholdDays != nil {
			result.StaleThresholdDays = local.StaleThresholdDays
		}
		if local.StaleBonusPerDay != nil {
			result.StaleBonusPerDay = local.StaleBonusPerDay
		}
		if local.StaleMaxBonus != nil {
			result.StaleMaxBonus = local.StaleMaxBonus
		}
		if local.DraftPenalty != nil {
			result.DraftPenalty = local.DraftPenalty
		}
		if local.SmallMaxFiles != nil {
			result.SmallMaxFiles = local.SmallMaxFiles
		}
		if local.SmallMaxLines != nil {
			result.SmallMaxLines = local.SmallMaxLines
		}
		if local.SizeXS != nil {
			result.SizeXS = local.SizeXS
		}
		if local.SizeS != nil {
			result.SizeS = local.SizeS
		}
		if local.SizeM != nil {
			result.SizeM = local.SizeM
		}
		if local.SizeL != nil {
			result.SizeL = local.SizeL
		}
	}

	// Return nil if all fields are nil
	if result.ApprovedBonus == nil && result.MergeableBonus == nil && result.ChangesRequestedBonus == nil &&
		result.ReviewCommentBonus == nil && result.ReviewCommentMaxBonus == nil &&
		result.StaleThresholdDays == nil && result.StaleBonusPerDay == nil && result.StaleMaxBonus == nil &&
		result.DraftPenalty == nil && result.SmallMaxFiles == nil && result.SmallMaxLines == nil &&
		result.SizeXS == nil && result.SizeS == nil && result.SizeM == nil && result.SizeL == nil {
		return nil
	}

	return result
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
		ExcludeAuthors: []string{},
		QuickWinLabels: labels,
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
		Scoring: &ScoringOverrides{
			OldUnreadBonus:              &weights.OldUnreadBonus,
			MaxAgeBonus:                 &weights.MaxAgeBonus,
			HotTopicBonus:               &weights.HotTopicBonus,
			HotTopicThreshold:           &weights.HotTopicThreshold,
			FYIPromotionThreshold:       &weights.FYIPromotionThreshold,
			NotablePromotionThreshold:   &weights.NotablePromotionThreshold,
			ImportantPromotionThreshold: &weights.ImportantPromotionThreshold,
			OpenStateBonus:              &weights.OpenStateBonus,
			ClosedStatePenalty:          &weights.ClosedStatePenalty,
			LowHangingBonus:             &weights.LowHangingBonus,
		},
		PR: &PROverrides{
			ApprovedBonus:         &weights.ApprovedPRBonus,
			MergeableBonus:        &weights.MergeablePRBonus,
			ChangesRequestedBonus: &weights.ChangesRequestedBonus,
			ReviewCommentBonus:    &weights.ReviewCommentBonus,
			ReviewCommentMaxBonus: &weights.ReviewCommentMaxBonus,
			StaleThresholdDays:    &weights.StalePRThresholdDays,
			StaleBonusPerDay:      &weights.StalePRBonusPerDay,
			StaleMaxBonus:         &weights.StalePRMaxBonus,
			DraftPenalty:          &weights.DraftPRPenalty,
			SmallMaxFiles:         &weights.SmallPRMaxFiles,
			SmallMaxLines:         &weights.SmallPRMaxLines,
			SizeXS:                &weights.PRSizeXS,
			SizeS:                 &weights.PRSizeS,
			SizeM:                 &weights.PRSizeM,
			SizeL:                 &weights.PRSizeL,
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

// ConfigPathInfo contains information about config file paths
type ConfigPathInfo struct {
	GlobalPath   string
	GlobalExists bool
	LocalPath    string
	LocalExists  bool
}

// GetConfigPaths returns path info for both global and local configs
func GetConfigPaths() ConfigPathInfo {
	globalPath := ConfigPath()
	localPath := LocalConfigPath()

	// Get absolute path for local config
	absLocalPath, err := filepath.Abs(localPath)
	if err != nil {
		absLocalPath = localPath
	}

	_, globalErr := os.Stat(globalPath)
	_, localErr := os.Stat(localPath)

	return ConfigPathInfo{
		GlobalPath:   globalPath,
		GlobalExists: globalErr == nil,
		LocalPath:    absLocalPath,
		LocalExists:  localErr == nil,
	}
}

// MinimalConfig returns a minimal config template with comments
func MinimalConfig() string {
	return `# Triage configuration file
# See: triage config defaults  (for all available options)

# Output format: table or json
default_format: table

# Exclude noisy repositories (optional)
# exclude_repos:
#   - owner/noisy-repo

# Exclude bot authors (optional)
# exclude_authors:
#   - dependabot[bot]
#   - renovate[bot]

# Override scoring weights (optional)
# base_scores:
#   review_requested: 100
#   mention: 90

# See README.md for full configuration options
`
}

// SaveTo writes content to a specific path, creating directories as needed
func SaveTo(path string, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write file %s: %w", path, err)
	}

	return nil
}
