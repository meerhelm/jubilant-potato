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
