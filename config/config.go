package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"

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
	Urgency    *UrgencyOverrides   `yaml:"urgency,omitempty"`
	Orphaned   *OrphanedConfig     `yaml:"orphaned,omitempty"`
}

// OrphanedConfig configures orphaned contribution detection
type OrphanedConfig struct {
	Enabled                   bool     `yaml:"enabled,omitempty"`
	Repos                     []string `yaml:"repos,omitempty"`
	StaleDays                 int      `yaml:"stale_days,omitempty"`                  // Default: 7
	ConsecutiveAuthorComments int      `yaml:"consecutive_author_comments,omitempty"` // Default: 2
	MaxItemsPerRepo           int      `yaml:"max_items_per_repo,omitempty"`          // Default: 20
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
	Orphaned        *int `yaml:"orphaned,omitempty"`
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

// UrgencyOverrides allows disabling specific urgency triggers
type UrgencyOverrides struct {
	ReviewRequested     *bool `yaml:"review_requested,omitempty"`
	Mention             *bool `yaml:"mention,omitempty"`
	ApprovedMergeablePR *bool `yaml:"approved_mergeable_pr,omitempty"`
	ChangesRequestedPR  *bool `yaml:"changes_requested_pr,omitempty"`
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
	Orphaned        int

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

	// Urgency trigger settings
	ReviewRequestedIsUrgent     bool
	MentionIsUrgent             bool
	ApprovedMergeablePRIsUrgent bool
	ChangesRequestedPRIsUrgent  bool
}

// DefaultScoreWeights returns the default scoring weights
func DefaultScoreWeights() ScoreWeights {
	return ScoreWeights{
		ReviewRequested: 100,
		Mention:         90,
		TeamMention:     85,
		Orphaned:        80,
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

		// Urgency triggers
		ReviewRequestedIsUrgent:     true,
		MentionIsUrgent:             false,
		ApprovedMergeablePRIsUrgent: true,
		ChangesRequestedPRIsUrgent:  false,
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
		if bs.Orphaned != nil {
			weights.Orphaned = *bs.Orphaned
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

	// Apply urgency overrides
	if c.Urgency != nil {
		u := c.Urgency
		if u.ReviewRequested != nil {
			weights.ReviewRequestedIsUrgent = *u.ReviewRequested
		}
		if u.Mention != nil {
			weights.MentionIsUrgent = *u.Mention
		}
		if u.ApprovedMergeablePR != nil {
			weights.ApprovedMergeablePRIsUrgent = *u.ApprovedMergeablePR
		}
		if u.ChangesRequestedPR != nil {
			weights.ChangesRequestedPRIsUrgent = *u.ChangesRequestedPR
		}
	}

	return weights
}

// defaultConfigDir returns the default config directory
func defaultConfigDir() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return ".triage"
	}
	return filepath.Join(configDir, "triage")
}

// configPath returns the path to the config file
func configPath() string {
	return filepath.Join(defaultConfigDir(), "config.yaml")
}

// localConfigPath returns the path to the local config file in the current directory
func localConfigPath() string {
	return ".triage.yaml"
}

// configFileExists returns true if the config file exists on disk
func configFileExists() bool {
	_, err := os.Stat(configPath())
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
	globalPath := configPath()
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
	localPath := localConfigPath()
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

	// Merge Urgency
	result.Urgency = mergeUrgencyOverrides(global.Urgency, local.Urgency)

	// Merge Orphaned
	result.Orphaned = mergeOrphanedConfig(global.Orphaned, local.Orphaned)

	return result
}

// mergePointerStruct merges two structs with pointer fields, where local values
// override global values. Both global and local must be pointers to structs of
// the same type. Returns nil if both inputs are nil or if all fields are nil.
func mergePointerStruct[T any](global, local *T) *T {
	if global == nil && local == nil {
		return nil
	}

	result := new(T)
	resultVal := reflect.ValueOf(result).Elem()

	// Copy global fields first
	if global != nil {
		globalVal := reflect.ValueOf(global).Elem()
		for i := 0; i < resultVal.NumField(); i++ {
			resultVal.Field(i).Set(globalVal.Field(i))
		}
	}

	// Override with local fields if they're non-nil
	if local != nil {
		localVal := reflect.ValueOf(local).Elem()
		for i := 0; i < resultVal.NumField(); i++ {
			localField := localVal.Field(i)
			if !localField.IsNil() {
				resultVal.Field(i).Set(localField)
			}
		}
	}

	// Check if all fields are nil
	allNil := true
	for i := 0; i < resultVal.NumField(); i++ {
		if !resultVal.Field(i).IsNil() {
			allNil = false
			break
		}
	}
	if allNil {
		return nil
	}

	return result
}

func mergeBaseScores(global, local *BaseScoreOverrides) *BaseScoreOverrides {
	return mergePointerStruct(global, local)
}

func mergeScoringOverrides(global, local *ScoringOverrides) *ScoringOverrides {
	return mergePointerStruct(global, local)
}

func mergePROverrides(global, local *PROverrides) *PROverrides {
	return mergePointerStruct(global, local)
}

func mergeUrgencyOverrides(global, local *UrgencyOverrides) *UrgencyOverrides {
	return mergePointerStruct(global, local)
}

func mergeOrphanedConfig(global, local *OrphanedConfig) *OrphanedConfig {
	if global == nil && local == nil {
		return nil
	}
	result := &OrphanedConfig{}

	if global != nil {
		result.Enabled = global.Enabled
		result.Repos = global.Repos
		result.StaleDays = global.StaleDays
		result.ConsecutiveAuthorComments = global.ConsecutiveAuthorComments
		result.MaxItemsPerRepo = global.MaxItemsPerRepo
	}

	if local != nil {
		// Local enabled overrides global
		if local.Enabled {
			result.Enabled = local.Enabled
		}
		// Local repos replace global if non-empty
		if len(local.Repos) > 0 {
			result.Repos = local.Repos
		}
		// Local numeric values override if non-zero
		if local.StaleDays > 0 {
			result.StaleDays = local.StaleDays
		}
		if local.ConsecutiveAuthorComments > 0 {
			result.ConsecutiveAuthorComments = local.ConsecutiveAuthorComments
		}
		if local.MaxItemsPerRepo > 0 {
			result.MaxItemsPerRepo = local.MaxItemsPerRepo
		}
	}

	// Return nil if effectively empty
	if !result.Enabled && len(result.Repos) == 0 && result.StaleDays == 0 &&
		result.ConsecutiveAuthorComments == 0 && result.MaxItemsPerRepo == 0 {
		return nil
	}

	return result
}

// Save saves the configuration to disk
func (c *Config) Save() error {
	configDir := defaultConfigDir()

	// Create config directory if it doesn't exist
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	configPath := configPath()
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
			Orphaned:        &weights.Orphaned,
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
		Urgency: &UrgencyOverrides{
			ReviewRequested:     &weights.ReviewRequestedIsUrgent,
			Mention:             &weights.MentionIsUrgent,
			ApprovedMergeablePR: &weights.ApprovedMergeablePRIsUrgent,
			ChangesRequestedPR:  &weights.ChangesRequestedPRIsUrgent,
		},
		Orphaned: &OrphanedConfig{
			Repos:                     []string{},
			StaleDays:                 7,
			ConsecutiveAuthorComments: 2,
			MaxItemsPerRepo:           50,
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
	globalPath := configPath()
	localPath := localConfigPath()

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

# Orphaned contribution detection (enabled by default, disable with --no-orphaned)
# Requires repos to be specified - no auto-discovery
# orphaned:
#   repos:                              # Repos to monitor for orphaned contributions (required)
#     - myorg/repo1
#     - myorg/repo2
#   stale_days: 7                       # Days without team response
#   consecutive_author_comments: 2      # Consecutive unanswered comments
#   max_items_per_repo: 50              # Limit per repository

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
