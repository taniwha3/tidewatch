package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfig(t *testing.T) {
	yamlContent := `
device:
  id: test-device-001

storage:
  path: /tmp/test.db

remote:
  url: http://localhost:8080/api/metrics
  enabled: true

metrics:
  - name: cpu.temperature
    interval: 30s
    enabled: true
  - name: srt.packet_loss
    interval: 5s
    enabled: true
  - name: disabled.metric
    interval: 10s
    enabled: false
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Test device config
	if cfg.Device.ID != "test-device-001" {
		t.Errorf("Expected device ID test-device-001, got %s", cfg.Device.ID)
	}

	// Test storage config
	if cfg.Storage.Path != "/tmp/test.db" {
		t.Errorf("Expected storage path /tmp/test.db, got %s", cfg.Storage.Path)
	}

	// Test remote config
	if !cfg.Remote.Enabled {
		t.Error("Expected remote to be enabled")
	}
	if cfg.Remote.URL != "http://localhost:8080/api/metrics" {
		t.Errorf("Expected remote URL http://localhost:8080/api/metrics, got %s", cfg.Remote.URL)
	}

	// Test metrics
	if len(cfg.Metrics) != 3 {
		t.Errorf("Expected 3 metrics, got %d", len(cfg.Metrics))
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid config",
			config: Config{
				Device:  DeviceConfig{ID: "device-001"},
				Storage: StorageConfig{Path: "/tmp/test.db"},
				Remote:  RemoteConfig{URL: "http://localhost", Enabled: true},
				Metrics: []MetricConfig{
					{Name: "test", Interval: "30s", Enabled: true},
				},
			},
			expectError: false,
		},
		{
			name: "missing device ID",
			config: Config{
				Device:  DeviceConfig{ID: ""},
				Storage: StorageConfig{Path: "/tmp/test.db"},
			},
			expectError: true,
			errorMsg:    "device.id is required",
		},
		{
			name: "missing storage path",
			config: Config{
				Device:  DeviceConfig{ID: "device-001"},
				Storage: StorageConfig{Path: ""},
			},
			expectError: true,
			errorMsg:    "storage.path is required",
		},
		{
			name: "remote enabled but no URL",
			config: Config{
				Device:  DeviceConfig{ID: "device-001"},
				Storage: StorageConfig{Path: "/tmp/test.db"},
				Remote:  RemoteConfig{URL: "", Enabled: true},
			},
			expectError: true,
			errorMsg:    "remote.url is required",
		},
		{
			name: "invalid interval",
			config: Config{
				Device:  DeviceConfig{ID: "device-001"},
				Storage: StorageConfig{Path: "/tmp/test.db"},
				Metrics: []MetricConfig{
					{Name: "test", Interval: "invalid", Enabled: true},
				},
			},
			expectError: true,
			errorMsg:    "invalid interval",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestMetricConfigInterval(t *testing.T) {
	m := MetricConfig{Name: "test", Interval: "30s", Enabled: true}

	duration, err := m.IntervalDuration()
	if err != nil {
		t.Fatalf("Failed to parse interval: %v", err)
	}

	expected := 30 * time.Second
	if duration != expected {
		t.Errorf("Expected duration %v, got %v", expected, duration)
	}
}

func TestEnabledMetrics(t *testing.T) {
	cfg := Config{
		Metrics: []MetricConfig{
			{Name: "metric1", Interval: "10s", Enabled: true},
			{Name: "metric2", Interval: "20s", Enabled: false},
			{Name: "metric3", Interval: "30s", Enabled: true},
		},
	}

	enabled := cfg.EnabledMetrics()
	if len(enabled) != 2 {
		t.Errorf("Expected 2 enabled metrics, got %d", len(enabled))
	}

	if enabled[0].Name != "metric1" || enabled[1].Name != "metric3" {
		t.Error("Enabled metrics not filtered correctly")
	}
}

func TestLoadConfigFileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestLoadConfigInvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(configPath, []byte("invalid: yaml: content:"), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("Expected error for invalid YAML")
	}
}
