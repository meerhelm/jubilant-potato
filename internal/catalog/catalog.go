// Package catalog groups regional versions, revisions and other variants of
// the same game and picks the one to offer by default ("1G1R").
package catalog

import (
	"sort"
	"strings"
	"unicode"

	"github.com/meerhelm/jubilant-potato/internal/romname"
	"github.com/meerhelm/jubilant-potato/internal/source"
)

// Prefs orders variants; earlier entries win.
type Prefs struct {
	Regions   []string // e.g. "USA", "Europe"
	Languages []string // ISO 639-1, e.g. "ru", "en"
}

// DefaultPrefs favours NTSC-U releases and the UI language, then English.
func DefaultPrefs(uiLang string) Prefs {
	p := Prefs{Regions: []string{"USA", "World", "Europe", "Japan"}, Languages: []string{"en"}}
	if uiLang != "" && uiLang != "en" {
		p.Languages = []string{uiLang, "en"}
	}
	return p
}

// Variant is one file of a game with its parsed name.
type Variant struct {
	Game source.Game
	Info romname.Info
}

// Group is one game with its variants, best first.
type Group struct {
	Title    string
	Variants []Variant
}

// Best returns the preferred variant.
func (g Group) Best() Variant { return g.Variants[0] }

// Build groups games by title (or by the source's grouping hint). Unless all
// is set, only retail variants are kept and games without one are dropped.
func Build(games []source.Game, p Prefs, all bool) []Group {
	index := map[string]int{}
	var groups []Group
	for _, g := range games {
		v := Variant{Game: g, Info: romname.Parse(g.File)}
		if !all && v.Info.Kind != romname.Retail {
			continue
		}
		key := g.Group
		if key == "" {
			key = g.System + "/" + normalize(v.Info.Title)
		}
		i, ok := index[key]
		if !ok {
			i = len(groups)
			index[key] = i
			groups = append(groups, Group{})
		}
		groups[i].Variants = append(groups[i].Variants, v)
	}

	for i := range groups {
		vs := groups[i].Variants
		sort.SliceStable(vs, func(a, b int) bool { return p.less(vs[a], vs[b]) })
		groups[i].Title = vs[0].Game.Title
		if groups[i].Title == "" {
			groups[i].Title = vs[0].Info.Title
		}
	}
	sort.SliceStable(groups, func(a, b int) bool {
		return strings.ToLower(groups[a].Title) < strings.ToLower(groups[b].Title)
	})
	return groups
}

func (p Prefs) less(a, b Variant) bool {
	ai, bi := a.Info, b.Info
	if ai.Kind != bi.Kind {
		return ai.Kind < bi.Kind
	}
	if x, y := rank(p.Languages, ai.Languages), rank(p.Languages, bi.Languages); x != y {
		return x < y
	}
	if x, y := rank(p.Regions, ai.Regions), rank(p.Regions, bi.Regions); x != y {
		return x < y
	}
	if ai.Revision != bi.Revision {
		return ai.Revision > bi.Revision
	}
	if ai.Verified != bi.Verified {
		return ai.Verified
	}
	if len(a.Game.File) != len(b.Game.File) {
		return len(a.Game.File) < len(b.Game.File)
	}
	return a.Game.File < b.Game.File
}

// rank is the position of the best-preferred value in have, or len(prefs)
// when none match.
func rank(prefs, have []string) int {
	best := len(prefs)
	for _, h := range have {
		for i, p := range prefs {
			if i < best && strings.EqualFold(p, h) {
				best = i
			}
		}
	}
	return best
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
