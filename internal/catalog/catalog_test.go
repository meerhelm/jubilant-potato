package catalog

import (
	"testing"

	"github.com/meerhelm/jubilant-potato/internal/source"
)

func files(names ...string) []source.Game {
	var out []source.Game
	for _, n := range names {
		out = append(out, source.Game{File: n, System: "snes"})
	}
	return out
}

func bests(groups []Group) map[string]string {
	m := map[string]string{}
	for _, g := range groups {
		m[g.Title] = g.Best().Game.File
	}
	return m
}

func TestBuildPicksPreferredVariant(t *testing.T) {
	games := files(
		"Super Mario World (Japan).zip",
		"Super Mario World (Europe) (En,Fr,De).zip",
		"Super Mario World (USA).zip",
		"Chrono Trigger (Japan).zip",
		"Chrono Trigger (USA).zip",
		"Chrono Trigger (U) [T+Rus].zip",
		"Star Fox 2 (Japan) (Beta).zip",
		"Street Fighter II Turbo (USA).zip",
		"Street Fighter II Turbo (USA) (Rev 1).zip",
		"Tetris Attack (Europe) (En,Ja).zip",
	)

	groups := Build(games, DefaultPrefs("ru"), false)
	want := map[string]string{
		"Super Mario World":       "Super Mario World (USA).zip",
		"Chrono Trigger":          "Chrono Trigger (USA).zip", // retail beats translation
		"Street Fighter II Turbo": "Street Fighter II Turbo (USA) (Rev 1).zip",
		"Tetris Attack":           "Tetris Attack (Europe) (En,Ja).zip",
	}
	got := bests(groups)
	if len(got) != len(want) {
		t.Fatalf("groups = %v, want %v (beta-only game hidden)", got, want)
	}
	for title, file := range want {
		if got[title] != file {
			t.Errorf("%s: best = %q, want %q", title, got[title], file)
		}
	}
	if n := len(groups[0].Variants); groups[0].Title != "Chrono Trigger" || n != 2 {
		t.Errorf("first group %q has %d variants, want Chrono Trigger with 2 retail", groups[0].Title, n)
	}

	all := bests(Build(games, DefaultPrefs("ru"), true))
	if all["Star Fox 2"] != "Star Fox 2 (Japan) (Beta).zip" {
		t.Errorf("all=true should include prerelease-only games: %v", all)
	}

	// Preferring Europe and French changes the pick.
	eu := bests(Build(games, Prefs{Regions: []string{"Europe"}, Languages: []string{"fr"}}, false))
	if eu["Super Mario World"] != "Super Mario World (Europe) (En,Fr,De).zip" {
		t.Errorf("fr/Europe best = %q", eu["Super Mario World"])
	}
}

func TestBuildUsesSourceGroupAndTitle(t *testing.T) {
	games := []source.Game{
		{File: "Pocket Monsters - Midori (Japan).gb", Title: "Pokémon Green", Group: "romm:7", System: "gb"},
		{File: "Pokemon - Red Version (USA, Europe).gb", Title: "Pokémon Red", Group: "romm:9", System: "gb"},
		{File: "Pokemon - Rote Edition (Germany).gb", Title: "Pokémon Red", Group: "romm:9", System: "gb"},
	}
	groups := Build(games, DefaultPrefs("en"), false)
	if len(groups) != 2 || groups[1].Title != "Pokémon Red" || len(groups[1].Variants) != 2 {
		t.Fatalf("groups = %+v", groups)
	}
}
