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
}

// WeightOverrides allows customizing priority weights
type WeightOverrides struct {
	ReviewRequested int `yaml:"review_requested,omitempty"`
	Mention         int `yaml:"mention,omitempty"`
	Author          int `yaml:"author,omitempty"`
	Assign          int `yaml:"assign,omitempty"`
	Comment         int `yaml:"comment,omitempty"`
	Subscribed      int `yaml:"subscribed,omitempty"`
}

// DefaultConfigDir returns the default config directory
func DefaultConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".priority"
	}
	return filepath.Join(home, ".config", "priority")
}

// ConfigPath returns the path to the config file
func ConfigPath() string {
	return filepath.Join(DefaultConfigDir(), "config.yaml")
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
