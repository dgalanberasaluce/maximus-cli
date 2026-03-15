// Package config manages application configuration and settings persistence.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	// AppName is the application identifier used for directory naming.
	AppName = "maximux-cli"
	// DBName is the SQLite3 database filename.
	DBName = "maximux.db"
	// configFileName is the JSON settings file inside the config dir.
	configFileName = "config.json"
)

// fileSettings is the on-disk JSON structure for user settings.
type fileSettings struct {
	BrewfilePath string `json:"brewfile_path"`
}

// Config holds all application configuration.
type Config struct {
	// ConfigDir is the base configuration directory (~/.config/maximux-cli).
	ConfigDir string
	// BrewfilePath is the full path to the Brewfile (from config.json).
	BrewfilePath string
	// DBPath is the full path to the SQLite3 database.
	DBPath string
}

// Load reads or creates the application configuration.
// It ensures the config directory and config.json exist before returning.
func Load() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("config: home dir: %w", err)
	}

	configDir := filepath.Join(home, ".config", AppName)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return nil, fmt.Errorf("config: create dir %s: %w", configDir, err)
	}

	configFile := filepath.Join(configDir, configFileName)
	settings, err := loadOrCreateSettings(configFile, home)
	if err != nil {
		return nil, fmt.Errorf("config: settings: %w", err)
	}

	// Expand ~ in the brewfile path (in case it was stored with a tilde).
	brewfilePath := settings.BrewfilePath
	if len(brewfilePath) >= 2 && brewfilePath[:2] == "~/" {
		brewfilePath = filepath.Join(home, brewfilePath[2:])
	}

	return &Config{
		ConfigDir:    configDir,
		BrewfilePath: brewfilePath,
		DBPath:       filepath.Join(configDir, DBName),
	}, nil
}

// loadOrCreateSettings reads config.json or writes a default if it doesn't exist.
func loadOrCreateSettings(path, home string) (fileSettings, error) {
	// Default brewfile path points to the user's dotfiles repo.
	defaults := fileSettings{
		BrewfilePath: filepath.Join(home, "git/github/dgalanberasaluce/dotfiles/app/brew/Brewfile"),
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		// First run — write the default config and return it.
		if err := writeSettings(path, defaults); err != nil {
			return fileSettings{}, err
		}
		return defaults, nil
	}
	if err != nil {
		return fileSettings{}, fmt.Errorf("read %s: %w", path, err)
	}

	var s fileSettings
	if err := json.Unmarshal(data, &s); err != nil {
		return fileSettings{}, fmt.Errorf("parse %s: %w", path, err)
	}
	// If brewfile_path is somehow empty, restore the default.
	if s.BrewfilePath == "" {
		s.BrewfilePath = defaults.BrewfilePath
	}
	return s, nil
}

// writeSettings serialises the settings struct to JSON on disk.
func writeSettings(path string, s fileSettings) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// Describe returns a human-readable summary of the current configuration.
func (c *Config) Describe() string {
	return fmt.Sprintf(
		"Config Directory : %s\nBrewfile Path    : %s\nDatabase Path    : %s",
		c.ConfigDir,
		c.BrewfilePath,
		c.DBPath,
	)
}
