// Package config loads the user's config.json.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// Config is the on-disk configuration stored next to the binary.
type Config struct {
	Language string   `json:"language,omitempty"` // "en" or "ru"; empty = from $LANG
	RomRoot  string   `json:"rom_root,omitempty"` // overrides the detected ROM root
	SwapAB   bool     `json:"swap_ab,omitempty"`  // swap confirm/back if the pad mapping is positional
	Sources  []Source `json:"sources"`

	// JoystickButtons maps raw joystick button indices to actions for pads
	// SDL has no GameController mapping for, e.g. {"0": "b", "1": "a"}.
	JoystickButtons map[string]string `json:"joystick_buttons,omitempty"`
}

// Source configures one remote catalog.
type Source struct {
	Name     string `json:"name"`
	Type     string `json:"type"` // "http" or "archive.org"
	Disabled bool   `json:"disabled,omitempty"`

	// http: root URL of a directory listing with one folder per system.
	URL      string `json:"url,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`

	// http: optional explicit system ID -> path under URL. When empty,
	// subfolders of URL are matched to systems by name.
	// archive.org: system ID -> list of item identifiers.
	Systems map[string]StringList `json:"systems,omitempty"`
}

// StringList accepts either a JSON string or an array of strings.
type StringList []string

func (l *StringList) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*l = StringList{s}
		return nil
	}
	var arr []string
	if err := json.Unmarshal(b, &arr); err != nil {
		return err
	}
	*l = arr
	return nil
}

// Load reads path, creating a default config there if it doesn't exist.
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		c := &Config{Sources: []Source{}}
		return c, c.Save(path)
	}
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &c, nil
}

// Save writes the config as indented JSON.
func (c *Config) Save(path string) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}
