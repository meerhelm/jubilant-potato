package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSystemDirUsesExistingFolders(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"Game Boy Advance", "arcade", "sfc", "Nintendo DS"} {
		os.Mkdir(filepath.Join(root, d), 0o755)
	}

	muos := Platform{Firmware: MuOS, RomRoot: root}
	muos.existing = scanExisting(MuOS, root)
	stock := Platform{Firmware: Stock, RomRoot: root}
	stock.existing = scanExisting(Stock, root)

	cases := []struct {
		p    Platform
		sys  string
		want string
	}{
		{muos, "gba", "Game Boy Advance"}, // free-form name matched by alias
		{muos, "fbneo", "arcade"},
		{muos, "mame", "arcade"},
		{muos, "snes", "sfc"},
		{muos, "psx", "PS"},    // default when nothing exists yet
		{stock, "snes", "sfc"}, // case-insensitive match of the stock name
		{stock, "gba", "GBA"},  // stock ignores free-form names
		{stock, "segacd", "MDCD"},
	}
	for _, c := range cases {
		if got := filepath.Base(c.p.SystemDir(c.sys)); got != c.want {
			t.Errorf("%s/%s = %q, want %q", c.p.Firmware, c.sys, got, c.want)
		}
	}
}

func TestMatchSystem(t *testing.T) {
	for name, want := range map[string]string{
		"Nintendo - Game Boy Advance": "gba",
		"GBA":                         "gba",
		"FC":                          "nes",
		"Sega - Mega Drive - Genesis": "md",
		"SEGA32X":                     "32x",
		"A2600":                       "atari2600",
		"ps":                          "psx",
	} {
		s, ok := MatchSystem(name)
		if !ok || s.ID != want {
			t.Errorf("MatchSystem(%q) = %q, %v; want %q", name, s.ID, ok, want)
		}
	}
	if _, ok := MatchSystem("readme"); ok {
		t.Error("MatchSystem(readme) should not match")
	}
}

func TestGuessSystem(t *testing.T) {
	for text, want := range map[string]string{
		"New Super Mario Land (Homebrew, SNES, SFC)":  "snes",
		"No-Intro Nintendo - Game Boy Advance (2024)": "gba",
		"Best of Game Boy homebrew":                   "gb",
		"Sega Mega Drive collection":                  "md",
		"PS1 Redump USA":                              "psx",
	} {
		if s, ok := GuessSystem(text); !ok || s.ID != want {
			t.Errorf("GuessSystem(%q) = %q, %v; want %q", text, s.ID, ok, want)
		}
	}
	if s, ok := GuessSystem("business records"); ok {
		t.Errorf("GuessSystem matched %q in unrelated text", s.ID)
	}
}

func TestSystemForFile(t *testing.T) {
	for p, want := range map[string]string{
		"Nintendo - Game Boy/Tetris (World).zip": "gb",
		"roms/Anguna.gba":                        "gba",
		"Super Mario World (USA).sfc":            "snes",
		"Game (USA).zip":                         "",
		"Game.bin":                               "",
	} {
		s, ok := SystemForFile(p)
		if (want == "" && ok) || (want != "" && s.ID != want) {
			t.Errorf("SystemForFile(%q) = %q, %v; want %q", p, s.ID, ok, want)
		}
	}
}

func TestFirmwareLanguage(t *testing.T) {
	if got := rocknixLanguage("system.hostname=RG34XX\nsystem.language=ru_RU\n"); got != "ru" {
		t.Errorf("rocknix ru = %q", got)
	}
	if got := rocknixLanguage("system.language=en_US"); got != "en" {
		t.Errorf("rocknix en = %q", got)
	}
	for name, want := range map[string]string{"Russian\n": "ru", "English (American)": "en", "German": "other", "": ""} {
		if got := muosLanguage(name); got != want {
			t.Errorf("muosLanguage(%q) = %q, want %q", name, got, want)
		}
	}
}
