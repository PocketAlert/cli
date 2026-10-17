package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const DefaultBaseURL = "https://api.pocketalert.app"

const (
	EnvAPIKey  = "POCKETALERT_API_KEY"
	EnvBaseURL = "POCKETALERT_API_URL"
	EnvConfig  = "POCKETALERT_CONFIG"
)

// Config is persisted as ~/.pocketalert/config.json. The JSON keys match the
// 1.x CLI so existing installs keep their credentials after an upgrade.
type Config struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
}

// Source tells where the effective API key came from, so `config` can explain
// why a stored key is being ignored.
type Source string

const (
	SourceNone Source = ""
	SourceFile Source = "config file"
	SourceEnv  Source = "env " + EnvAPIKey
)

func Path() string {
	if p := os.Getenv(EnvConfig); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".pocketalert", "config.json")
	}
	return filepath.Join(home, ".pocketalert", "config.json")
}

// LoadFile reads only the stored config, without environment overrides.
func LoadFile() (*Config, error) {
	data, err := os.ReadFile(Path())
	if errors.Is(err, fs.ErrNotExist) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", Path(), err)
	}
	return &c, nil
}

// Load returns the effective config: environment variables win over the file,
// which lets CI jobs run without `pocketalert auth`.
func Load() (*Config, Source, error) {
	c, err := LoadFile()
	if err != nil {
		return nil, SourceNone, err
	}
	source := SourceNone
	if c.APIKey != "" {
		source = SourceFile
	}
	if v := strings.TrimSpace(os.Getenv(EnvAPIKey)); v != "" {
		c.APIKey = v
		source = SourceEnv
	}
	if v := strings.TrimSpace(os.Getenv(EnvBaseURL)); v != "" {
		c.BaseURL = v
	}
	if c.BaseURL == "" {
		c.BaseURL = DefaultBaseURL
	}
	c.BaseURL = strings.TrimRight(c.BaseURL, "/")
	return c, source, nil
}

func Save(c *Config) error {
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// MaskKey hides all but the edges of a key; short keys are fully masked
// instead of panicking on slicing.
func MaskKey(key string) string {
	if key == "" {
		return "(not set)"
	}
	if len(key) < 12 {
		return strings.Repeat("*", len(key))
	}
	return key[:6] + "…" + key[len(key)-4:]
}
