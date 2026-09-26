package platform

import (
	"path"
	"strings"
	"unicode"
)

// System describes an emulated platform independent of any firmware.
type System struct {
	ID      string
	Name    string
	Exts    []string // accepted file extensions (lower case, with dot), archives included
	Extract bool     // unpack .zip after download (disc/NDS images must be extracted)
	Aliases []string // names used by servers/archives, matched after normalization
}

var systems = []System{
	{ID: "nes", Name: "Nintendo Entertainment System", Exts: exts(".nes", ".fds", ".unf", ".unif"),
		Aliases: []string{"fc", "famicom", "nintendo", "Nintendo - Nintendo Entertainment System", "Nintendo - Family Computer Disk System"}},
	{ID: "snes", Name: "Super Nintendo", Exts: exts(".sfc", ".smc", ".fig", ".swc", ".bs"),
		Aliases: []string{"sfc", "sfam", "superfamicom", "super nintendo", "Nintendo - Super Nintendo Entertainment System"}},
	{ID: "gb", Name: "Game Boy", Exts: exts(".gb"),
		Aliases: []string{"gameboy", "Nintendo - Game Boy"}},
	{ID: "gbc", Name: "Game Boy Color", Exts: exts(".gbc", ".gb"),
		Aliases: []string{"gameboycolor", "Nintendo - Game Boy Color"}},
	{ID: "gba", Name: "Game Boy Advance", Exts: exts(".gba"),
		Aliases: []string{"gameboyadvance", "Nintendo - Game Boy Advance"}},
	{ID: "nds", Name: "Nintendo DS", Exts: exts(".nds", ".dsi"), Extract: true,
		Aliases: []string{"ds", "nintendods", "Nintendo - Nintendo DS", "Nintendo - Nintendo DS (Decrypted)"}},
	{ID: "n64", Name: "Nintendo 64", Exts: exts(".n64", ".z64", ".v64"),
		Aliases: []string{"nintendo64", "Nintendo - Nintendo 64"}},
	{ID: "md", Name: "Mega Drive / Genesis", Exts: exts(".md", ".gen", ".smd", ".bin"),
		Aliases: []string{"megadrive", "genesis", "Sega - Mega Drive - Genesis"}},
	{ID: "sms", Name: "Master System", Exts: exts(".sms"),
		Aliases: []string{"mastersystem", "Sega - Master System - Mark III"}},
	{ID: "gg", Name: "Game Gear", Exts: exts(".gg"),
		Aliases: []string{"gamegear", "Sega - Game Gear"}},
	{ID: "segacd", Name: "Sega CD / Mega-CD", Exts: exts(".chd", ".cue", ".bin", ".iso", ".m3u"), Extract: true,
		Aliases: []string{"megacd", "mdcd", "scd", "Sega - Mega-CD - Sega CD"}},
	{ID: "32x", Name: "Sega 32X", Exts: exts(".32x"),
		Aliases: []string{"sega32x", "sega32", "Sega - 32X"}},
	{ID: "saturn", Name: "Sega Saturn", Exts: exts(".chd", ".cue", ".bin", ".iso", ".m3u"), Extract: true,
		Aliases: []string{"segasaturn", "Sega - Saturn"}},
	{ID: "dc", Name: "Dreamcast", Exts: exts(".chd", ".cdi", ".gdi", ".m3u"), Extract: true,
		Aliases: []string{"dreamcast", "Sega - Dreamcast"}},
	{ID: "psx", Name: "PlayStation", Exts: exts(".chd", ".pbp", ".cue", ".bin", ".img", ".iso", ".m3u"), Extract: true,
		Aliases: []string{"ps", "ps1", "playstation", "Sony - PlayStation"}},
	{ID: "psp", Name: "PlayStation Portable", Exts: exts(".iso", ".cso", ".chd", ".pbp"), Extract: true,
		Aliases: []string{"playstationportable", "Sony - PlayStation Portable"}},
	{ID: "pce", Name: "PC Engine / TurboGrafx-16", Exts: exts(".pce"),
		Aliases: []string{"pcengine", "tg16", "turbografx16", "NEC - PC Engine - TurboGrafx 16"}},
	{ID: "atari2600", Name: "Atari 2600", Exts: exts(".a26", ".bin"),
		Aliases: []string{"atari", "a2600", "2600", "Atari - 2600"}},
	{ID: "neogeo", Name: "Neo Geo", Exts: nil,
		Aliases: []string{"neo geo", "neogeoaes", "neogeomvs", "SNK - Neo Geo"}},
	{ID: "fbneo", Name: "Arcade (FinalBurn Neo)", Exts: nil,
		Aliases: []string{"fba", "finalburn", "finalburnneo", "arcade"}},
	{ID: "mame", Name: "Arcade (MAME)", Exts: nil,
		Aliases: []string{"mame2003", "mame2003plus", "mame2010"}},
}

var archiveExts = []string{".zip", ".7z"}

func exts(e ...string) []string { return append(e, archiveExts...) }

// Systems returns the known systems in display order.
func Systems() []System { return systems }

// SystemByID returns the system with the given ID.
func SystemByID(id string) (System, bool) {
	for _, s := range systems {
		if s.ID == id {
			return s, true
		}
	}
	return System{}, false
}

// MatchSystem guesses a system from a directory or collection name such as
// "gba", "GBA" or "Nintendo - Game Boy Advance".
func MatchSystem(name string) (System, bool) {
	n := normalize(name)
	if n == "" {
		return System{}, false
	}
	for _, s := range systems {
		if normalize(s.ID) == n || normalize(s.Name) == n {
			return s, true
		}
		for _, a := range s.Aliases {
			if normalize(a) == n {
				return s, true
			}
		}
	}
	return System{}, false
}

// Accepts reports whether a file name looks like a ROM for this system.
// Systems without an extension list (arcade) accept archives only.
func (s System) Accepts(file string) bool {
	ext := strings.ToLower(path.Ext(file))
	if ext == "" {
		return false
	}
	list := s.Exts
	if len(list) == 0 {
		list = archiveExts
	}
	for _, e := range list {
		if e == ext {
			return true
		}
	}
	return false
}

func normalize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// GuessSystem finds the system named in free text such as an archive title
// ("New Super Mario Land (Homebrew, SNES, SFC)"). The longest matching
// alias wins, so "Game Boy Advance" beats "Game Boy"; aliases of four or
// fewer letters only match whole words.
func GuessSystem(text string) (System, bool) {
	norm := normalize(text)
	words := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		words[w] = true
	}
	var best System
	bestLen := 0
	for _, s := range systems {
		for _, a := range append([]string{s.ID, s.Name}, s.Aliases...) {
			n := normalize(a)
			hit := false
			if len(n) <= 4 {
				hit = words[n]
			} else {
				hit = strings.Contains(norm, n)
			}
			if hit && len(n) > bestLen {
				best, bestLen = s, len(n)
			}
		}
	}
	return best, bestLen > 0
}

// SystemForFile guesses a file's system from a folder in its path or an
// extension only one system uses; archives and shared extensions give no
// answer.
func SystemForFile(p string) (System, bool) {
	dirs := strings.Split(path.Dir(p), "/")
	for _, d := range dirs {
		if s, ok := MatchSystem(d); ok {
			return s, true
		}
	}
	ext := strings.ToLower(path.Ext(p))
	var found System
	n := 0
	for _, s := range systems {
		for _, e := range s.Exts {
			if e == ext && e != ".zip" && e != ".7z" {
				found = s
				n++
				break
			}
		}
	}
	return found, n == 1
}
