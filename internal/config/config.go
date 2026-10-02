// Package config stores the CLI settings in $XDG_CONFIG_HOME/md/config.json.
package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Token   string `json:"token,omitempty"`
	BaseURL string `json:"baseUrl,omitempty"`
}

// Dir returns the config directory, honouring MD_CONFIG_DIR and XDG_CONFIG_HOME.
func Dir() string {
	if d := os.Getenv("MD_CONFIG_DIR"); d != "" {
		return d
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "md")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "md")
}

func path() string { return filepath.Join(Dir(), "config.json") }

// Load returns the saved config; a missing file is an empty config.
func Load() (Config, error) {
	var c Config
	data, err := os.ReadFile(path())
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	return c, json.Unmarshal(data, &c)
}

// Save writes the config with owner-only permissions: it holds the token.
func Save(c Config) error {
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path(), append(data, '\n'), 0o600)
}

// OrionDir is where the orion CLI, which used to host these commands, keeps its files.
func OrionDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "orion")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".orion")
}

// OrionToken returns the upload token saved by `orion md setup`, if any.
func OrionToken() string {
	if t := os.Getenv("ORION_MD_TOKEN"); t != "" {
		return t
	}
	data, err := os.ReadFile(filepath.Join(OrionDir(), "config.yaml"))
	if err != nil {
		return ""
	}
	var c struct {
		MD struct {
			UploadToken string `yaml:"upload_token"`
		} `yaml:"md"`
	}
	if yaml.Unmarshal(data, &c) != nil {
		return ""
	}
	return strings.TrimSpace(c.MD.UploadToken)
}
