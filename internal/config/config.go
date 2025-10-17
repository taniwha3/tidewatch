package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	Device     DeviceConfig     `yaml:"device"`
	Storage    StorageConfig    `yaml:"storage"`
	Remote     RemoteConfig     `yaml:"remote"`
	Monitoring MonitoringConfig `yaml:"monitoring"`
	Logging    LoggingConfig    `yaml:"logging"`
	Metrics    []MetricConfig   `yaml:"metrics"`
}

// DeviceConfig contains device identification
type DeviceConfig struct {
	ID string `yaml:"id"`
}

// StorageConfig contains local storage settings
type StorageConfig struct {
	Path                     string `yaml:"path"`
	WALCheckpointIntervalStr string `yaml:"wal_checkpoint_interval"` // How often to checkpoint WAL (default: 1h)
	WALCheckpointSizeMB      int    `yaml:"wal_checkpoint_size_mb"`  // Checkpoint when WAL exceeds this size (default: 64)
}

// WALCheckpointInterval parses the checkpoint interval string to time.Duration
// Returns default of 1 hour if not configured
// Returns error if duration string is invalid or non-positive
func (s *StorageConfig) WALCheckpointInterval() (time.Duration, error) {
	if s.WALCheckpointIntervalStr == "" {
		return 1 * time.Hour, nil
	}
	duration, err := time.ParseDuration(s.WALCheckpointIntervalStr)
	if err != nil {
		return 0, fmt.Errorf("invalid wal_checkpoint_interval '%s': %w", s.WALCheckpointIntervalStr, err)
	}
	// Guard against non-positive intervals to prevent panic in time.NewTicker
	if duration <= 0 {
		return 0, fmt.Errorf("wal_checkpoint_interval must be positive, got %v", duration)
	}
	return duration, nil
}

// WALCheckpointSizeBytes returns the checkpoint size threshold in bytes
// Returns default of 64 MB if not configured
func (s *StorageConfig) WALCheckpointSizeBytes() int64 {
	if s.WALCheckpointSizeMB <= 0 {
		return 64 * 1024 * 1024 // Default: 64 MB
	}
	return int64(s.WALCheckpointSizeMB) * 1024 * 1024
}

// RetryConfig contains retry settings for uploads
type RetryConfig struct {
	Enabled           *bool   `yaml:"enabled"` // Pointer to distinguish "not set" from "explicitly false"
	MaxAttempts       int     `yaml:"max_attempts"`
	InitialBackoffStr string  `yaml:"initial_backoff"`
	MaxBackoffStr     string  `yaml:"max_backoff"`
	BackoffMultiplier float64 `yaml:"backoff_multiplier"`
	JitterPercent     *int    `yaml:"jitter_percent"` // Pointer to distinguish "not set" from "explicitly 0"
}

// InitialBackoff parses the initial backoff string to time.Duration
// Returns default of 1s if not configured
// Returns error if duration string is invalid or non-positive
func (r *RetryConfig) InitialBackoff() (time.Duration, error) {
	if r.InitialBackoffStr == "" {
		return 1 * time.Second, nil
	}
	duration, err := time.ParseDuration(r.InitialBackoffStr)
	if err != nil {
		return 0, fmt.Errorf("invalid retry.initial_backoff '%s': %w", r.InitialBackoffStr, err)
	}
	// Guard against non-positive durations: time.After with negative duration fires immediately
	if duration <= 0 {
		return 0, fmt.Errorf("retry.initial_backoff must be positive, got %v", duration)
	}
	return duration, nil
}

// MaxBackoff parses the max backoff string to time.Duration
// Returns default of 30s if not configured
// Returns error if duration string is invalid or non-positive
func (r *RetryConfig) MaxBackoff() (time.Duration, error) {
	if r.MaxBackoffStr == "" {
		return 30 * time.Second, nil
	}
	duration, err := time.ParseDuration(r.MaxBackoffStr)
	if err != nil {
		return 0, fmt.Errorf("invalid retry.max_backoff '%s': %w", r.MaxBackoffStr, err)
	}
	// Guard against non-positive durations: time.After with negative duration fires immediately
	if duration <= 0 {
		return 0, fmt.Errorf("retry.max_backoff must be positive, got %v", duration)
	}
	return duration, nil
}

// RemoteConfig contains remote endpoint settings
type RemoteConfig struct {
	URL               string      `yaml:"url"`
	Enabled           bool        `yaml:"enabled"`
	UploadIntervalStr string      `yaml:"upload_interval"`
	AuthToken         string      `yaml:"auth_token"`      // Bearer token for authentication (inline)
	AuthTokenFile     string      `yaml:"auth_token_file"` // Path to file containing bearer token
	BatchSize         int         `yaml:"batch_size"`      // Max metrics per batch query (default: 2500)
	ChunkSize         int         `yaml:"chunk_size"`      // Metrics per chunk upload (default: 50)
	Retry             RetryConfig `yaml:"retry"`           // Retry configuration
}

// GetBatchSize returns the batch size or default
func (r *RemoteConfig) GetBatchSize() int {
	if r.BatchSize <= 0 {
		return 2500 // Default
	}
	return r.BatchSize
}

// GetChunkSize returns the chunk size or default
func (r *RemoteConfig) GetChunkSize() int {
	if r.ChunkSize <= 0 {
		return 50 // Default
	}
	return r.ChunkSize
}

// MonitoringConfig contains monitoring and health check settings
type MonitoringConfig struct {
	ClockSkewURL             string `yaml:"clock_skew_url"`               // URL for clock skew detection (e.g., http://localhost:8428/health)
	ClockSkewCheckInterval   string `yaml:"clock_skew_check_interval"`    // How often to check clock skew (default: 5m)
	ClockSkewWarnThresholdMs int    `yaml:"clock_skew_warn_threshold_ms"` // Warn when skew exceeds this (default: 2000ms)
	HealthAddress            string `yaml:"health_address"`               // Address for health endpoint server (e.g., ":9100")
}

// LoggingConfig contains logging settings
type LoggingConfig struct {
	Level  string `yaml:"level"`  // debug, info, warn, error (default: info)
	Format string `yaml:"format"` // json, console (default: console)
}

// UploadInterval parses the upload interval string to time.Duration
// Returns default of 30s if not configured
// Returns error if duration string is invalid or non-positive
func (r *RemoteConfig) UploadInterval() (time.Duration, error) {
	if r.UploadIntervalStr == "" {
		return 30 * time.Second, nil
	}
	duration, err := time.ParseDuration(r.UploadIntervalStr)
	if err != nil {
		return 0, fmt.Errorf("invalid remote.upload_interval '%s': %w", r.UploadIntervalStr, err)
	}
	// Guard against non-positive intervals to prevent panic in time.NewTicker
	if duration <= 0 {
		return 0, fmt.Errorf("remote.upload_interval must be positive, got %v", duration)
	}
	return duration, nil
}

// MetricConfig defines a metric to collect
type MetricConfig struct {
	Name     string `yaml:"name"`
	Interval string `yaml:"interval"`
	Enabled  bool   `yaml:"enabled"`
}

// IntervalDuration parses the interval string to time.Duration
func (m *MetricConfig) IntervalDuration() (time.Duration, error) {
	return time.ParseDuration(m.Interval)
}

// Load reads and parses a YAML configuration file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Validate that both auth_token and auth_token_file are not specified
	if cfg.Remote.AuthToken != "" && cfg.Remote.AuthTokenFile != "" {
		return nil, fmt.Errorf("cannot specify both remote.auth_token and remote.auth_token_file")
	}

	// Load auth token from file if auth_token_file is specified
	if cfg.Remote.AuthTokenFile != "" {
		token, err := loadAuthTokenFromFile(cfg.Remote.AuthTokenFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load auth token from file: %w", err)
		}
		cfg.Remote.AuthToken = token
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

// loadAuthTokenFromFile reads an auth token from a file and returns it trimmed
func loadAuthTokenFromFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read token file %s: %w", path, err)
	}

	// Trim whitespace and newlines from the token
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("token file %s is empty", path)
	}

	return token, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.Device.ID == "" {
		return fmt.Errorf("device.id is required")
	}
	if c.Storage.Path == "" {
		return fmt.Errorf("storage.path is required")
	}
	if c.Remote.Enabled && c.Remote.URL == "" {
		return fmt.Errorf("remote.url is required when remote is enabled")
	}

	// Validate storage timing values
	if _, err := c.Storage.WALCheckpointInterval(); err != nil {
		return err
	}

	// Validate remote timing values (always validate, even if remote is disabled)
	// This prevents runtime crashes when code calls these methods before checking enabled flag
	if _, err := c.Remote.UploadInterval(); err != nil {
		return err
	}

	// Validate retry timing values if configured
	if c.Remote.Retry.InitialBackoffStr != "" {
		if _, err := c.Remote.Retry.InitialBackoff(); err != nil {
			return err
		}
	}
	if c.Remote.Retry.MaxBackoffStr != "" {
		if _, err := c.Remote.Retry.MaxBackoff(); err != nil {
			return err
		}
	}

	// Validate backoff_multiplier if configured
	// Guard against ≤0 or <1 values: math.Pow yields zero/negative delays causing immediate retry hammering
	if c.Remote.Retry.BackoffMultiplier != 0 {
		if c.Remote.Retry.BackoffMultiplier < 1.0 {
			return fmt.Errorf("retry.backoff_multiplier must be >= 1.0, got %v", c.Remote.Retry.BackoffMultiplier)
		}
	}

	// Validate metric intervals
	for _, m := range c.Metrics {
		if m.Enabled {
			interval, err := m.IntervalDuration()
			if err != nil {
				return fmt.Errorf("invalid interval for metric %s: %w", m.Name, err)
			}
			// Guard against non-positive intervals to prevent panic in time.NewTicker
			if interval <= 0 {
				return fmt.Errorf("metric %s: interval must be positive (got %v)", m.Name, interval)
			}
		}
	}

	return nil
}

// EnabledMetrics returns only the enabled metrics
func (c *Config) EnabledMetrics() []MetricConfig {
	var enabled []MetricConfig
	for _, m := range c.Metrics {
		if m.Enabled {
			enabled = append(enabled, m)
		}
	}
	return enabled
}
