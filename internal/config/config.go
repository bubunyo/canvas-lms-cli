// Package config holds non-secret CLI settings (base URL).
// Secrets live in the OS keychain, see package creds.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const DefaultBaseURL = "https://onlinestudy.york.ac.uk"

type Config struct {
	BaseURL string `json:"base_url"`
}

func Dir() (string, error) {
	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "canvas-cli"), nil
}

func path() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.json"), nil
}

// Load reads the config file, applying defaults and the CANVAS_BASE_URL override.
func Load() (*Config, error) {
	c := &Config{BaseURL: DefaultBaseURL}
	p, err := path()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return nil, err
	default:
		if err := json.Unmarshal(b, c); err != nil {
			return nil, err
		}
	}
	if v := os.Getenv("CANVAS_BASE_URL"); v != "" {
		c.BaseURL = v
	}
	c.BaseURL = strings.TrimRight(c.BaseURL, "/")
	return c, nil
}

func (c *Config) Save() error {
	p, err := path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o600)
}
