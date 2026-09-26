// Package config loads the user's config.json.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// Config is the on-disk configuration stored next to the binary.
type Config struct {
	Language string `json:"language,omitempty"` // "en" or "ru"; empty = from $LANG
	RomRoot  string `json:"rom_root,omitempty"` // overrides the detected ROM root
	SwapAB   bool   `json:"swap_ab,omitempty"`  // swap confirm/back if the pad mapping is positional

	// Variant preferences, best first, e.g. ["USA", "Europe"] and ["ru", "en"].
	// Defaults: USA, World, Europe, Japan; the UI language, then English.
	PreferRegions   []string `json:"prefer_regions,omitempty"`
	PreferLanguages []string `json:"prefer_languages,omitempty"`
	Sources         []Source `json:"sources"`

	// ItchClientID overrides the built-in itch.io OAuth client for QR login.
	ItchClientID string `json:"itch_client_id,omitempty"`

	// DeviceID identifies this handheld to servers that pair devices (RomM).
	DeviceID string `json:"device_id,omitempty"`

	// JoystickButtons maps raw joystick button indices to actions for pads
	// SDL has no GameController mapping for, e.g. {"0": "b", "1": "a"}.
	JoystickButtons map[string]string `json:"joystick_buttons,omitempty"`

	// ControllerMappings are SDL GameController mappings by joystick GUID,
	// written by the in-app button setup.
	ControllerMappings map[string]string `json:"controller_mappings,omitempty"`
}

// Source configures one remote catalog.
type Source struct {
	Name     string `json:"name"`
	Type     string `json:"type"` // "http", "smb", "archive.org" or "romm"
	Disabled bool   `json:"disabled,omitempty"`

	// http: root URL of a directory listing with one folder per system.
	// romm: server URL, e.g. http://192.168.1.10:8080.
	URL      string `json:"url,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`

	// smb: server, share name and folder inside the share ("roms", "a/b");
	// Username/Password are optional (guest access is tried first).
	Host  string `json:"host,omitempty"`
	Share string `json:"share,omitempty"`
	Path  string `json:"path,omitempty"`

	// romm: API token obtained by pairing; filled in by the app.
	Token string `json:"token,omitempty"`

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
		b, err = []byte(`{"sources": []}`), nil
	}
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if c.DeviceID == "" {
		c.DeviceID = newDeviceID()
		if err := c.Save(path); err != nil {
			return nil, err
		}
	}
	return &c, nil
}

func newDeviceID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return "potato-" + hex.EncodeToString(b)
}

// Save writes the config as indented JSON, atomically so a power loss on a
// handheld can't leave a truncated file behind.
func (c *Config) Save(path string) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
