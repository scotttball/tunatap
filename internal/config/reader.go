package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

// ReadConfig loads configuration from a YAML file.
func ReadConfig(path string) (*Config, error) {
	// Expand ~ to home directory
	if len(path) > 0 && path[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		path = filepath.Join(home, path[1:])
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			log.Info().Msgf("Config file not found at %s, using defaults", path)
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Handle empty config file
	if len(data) == 0 {
		log.Info().Msg("Config file is empty, using defaults")
		return DefaultConfig(), nil
	}

	config := DefaultConfig()
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Apply defaults for nil pointer fields
	if config.SshConnectionPoolSize == nil {
		poolSize := 5
		config.SshConnectionPoolSize = &poolSize
	}
	if config.SshConnectionWarmupCount == nil {
		warmupCount := 2
		config.SshConnectionWarmupCount = &warmupCount
	}
	if config.SshConnectionMaxConcurrentUse == nil {
		maxConcurrent := 10
		config.SshConnectionMaxConcurrentUse = &maxConcurrent
	}

	return config, nil
}

// SaveConfig writes configuration to a YAML file.
func SaveConfig(path string, config *Config) error {
	// Expand ~ to home directory
	if len(path) > 0 && path[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		path = filepath.Join(home, path[1:])
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	log.Debug().Msgf("Config saved to %s", path)
	return nil
}

// GetDefaultConfigPath returns the default config file path.
func GetDefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".tunatap", "config.yaml"), nil
}
