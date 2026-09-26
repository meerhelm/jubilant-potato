package ui

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/veandco/go-sdl2/sdl"

	"github.com/meerhelm/jubilant-potato/internal/catalog"
	"github.com/meerhelm/jubilant-potato/internal/download"
	"github.com/meerhelm/jubilant-potato/internal/platform"
	"github.com/meerhelm/jubilant-potato/internal/source"
)

// gamesScreen lists one row per game; variants (regions, revisions, betas,
// hacks) are folded into it and the preferred one is downloaded by default.
type gamesScreen struct {
	a    *app
	src  source.Source
	sys  source.System
	psys platform.System
	dest string

	t      *task[[]source.Game]
	all    bool // include prerelease, hacks and bad dumps
	groups []catalog.Group
	filter string
	view   []int // indices into groups matching filter
	l      list

	installed map[string]bool // lower-cased file names and stems in dest
	doneJobs  int
}

func newGamesScreen(a *app, src source.Source, sys source.System) *gamesScreen {
	psys, _ := platform.SystemByID(sys.ID)
	s := &gamesScreen{a: a, src: src, sys: sys, psys: psys, dest: a.opts.Platform.SystemDir(sys.ID)}
	s.t = startTask(func() ([]source.Game, error) { return src.Games(context.Background(), sys) })
	s.scanInstalled()
	return s
}

func (s *gamesScreen) Title() string { return s.sys.Label }

func (s *gamesScreen) Update() bool {
	changed := false
	if s.t.Poll() {
		s.rebuild()
		changed = true
	}
	// Rescan the ROM folder whenever a download finishes.
	done := 0
	for _, j := range s.a.opts.Manager.Jobs() {
		if j.State == download.Done {
			done++
		}
	}
	if done != s.doneJobs {
		s.doneJobs = done
		s.scanInstalled()
		changed = true
	}
	return changed
}

// rebuild regroups the games, keeping the selection on the same title.
func (s *gamesScreen) rebuild() {
	var keep string
	if g, ok := s.selected(); ok {
		keep = g.Title
	}
	s.groups = catalog.Build(s.t.val, s.a.prefs, s.all)
	s.applyFilter()
	for i, gi := range s.view {
		if s.groups[gi].Title == keep {
			s.l.Sel = i
			break
		}
	}
}

func (s *gamesScreen) scanInstalled() {
	s.installed = map[string]bool{}
	entries, _ := os.ReadDir(s.dest)
	for _, e := range entries {
		n := strings.ToLower(e.Name())
		s.installed[n] = true
		s.installed[stem(n)] = true
	}
}

func (s *gamesScreen) isInstalled(g source.Game) bool {
	n := strings.ToLower(g.File)
	return s.installed[n] || s.installed[stem(n)]
}

func stem(n string) string { return strings.TrimSuffix(n, filepath.Ext(n)) }

// status reports whether any of the variants is on the device or queued.
func (s *gamesScreen) status(vs []catalog.Variant) (installed, pending bool) {
	for _, v := range vs {
		if s.isInstalled(v.Game) {
			return true, false
		}
		pending = pending || s.a.opts.Manager.Pending(v.Game.URL)
	}
	return false, pending
}

// mark is the row color for status: green installed, gold queued.
func (s *gamesScreen) mark(vs []catalog.Variant) Color {
	switch installed, pending := s.status(vs); {
	case installed:
		return colOK
	case pending:
		return colAccent
	}
	return Color{}
}

func (s *gamesScreen) applyFilter() {
	s.view = s.view[:0]
	words := strings.Fields(strings.ToLower(s.filter))
next:
	for i, g := range s.groups {
		text := strings.ToLower(g.Title)
		for _, v := range g.Variants {
			text += "\n" + strings.ToLower(v.Game.Name)
		}
		for _, w := range words {
			if !strings.Contains(text, w) {
				continue next
			}
		}
		s.view = append(s.view, i)
	}
	s.l.SetN(len(s.view))
}

func (s *gamesScreen) Hints() []Hint {
	toggle := T("All")
	if s.all {
		toggle = T("Releases")
	}
	return []Hint{{"A", T("Download")}, {"X", T("Versions")}, {"Y", T("Search")}, {"START", toggle}, {"B", T("Back")}}
}

func (s *gamesScreen) Handle(act Action) {
	switch act {
	case Up:
		s.l.Move(-1)
	case Down:
		s.l.Move(1)
	case Left:
		s.l.Move(-s.l.Page())
	case Right:
		s.l.Move(s.l.Page())
	case L1:
		s.jumpLetter(-1)
	case R1:
		s.jumpLetter(1)
	case A:
		if g, ok := s.selected(); ok {
			s.download(g.Variants)
		}
	case X:
		if g, ok := s.selected(); ok {
			s.a.push(newVariantsScreen(s.a, s, g))
		}
	case Y:
		if s.t.done {
			s.a.push(newKeyboardScreen(s.a, T("Search"), s.filter, func(text string) {
				s.filter = text
				s.applyFilter()
			}))
		}
	case Start:
		if s.t.done && s.t.err == nil {
			s.all = !s.all
			s.rebuild()
			if s.all {
				s.a.notify(T("Showing betas, demos and hacks"))
			} else {
				s.a.notify(T("Showing releases only"))
			}
		}
	case Select:
		s.a.push(newDownloadsScreen(s.a))
	case B:
		s.a.pop()
	}
}

func (s *gamesScreen) selected() (catalog.Group, bool) {
	if !s.t.done || s.l.N == 0 {
		return catalog.Group{}, false
	}
	return s.groups[s.view[s.l.Sel]], true
}

// download fetches the first variant, unless one of them is already on the
// device or queued.
func (s *gamesScreen) download(vs []catalog.Variant) {
	switch installed, pending := s.status(vs); {
	case installed:
		s.a.notify(T("Already installed"))
	case pending:
		s.a.notify(T("Already in queue"))
	default:
		g := vs[0].Game
		s.a.opts.Manager.Enqueue(g, s.dest, s.psys.Extract || g.Extract)
		s.a.notify(T("Queued: %s", g.Name))
	}
}

func letterOf(name string) rune {
	for _, r := range name {
		if unicode.IsLetter(r) {
			return unicode.ToUpper(r)
		}
		return '#'
	}
	return '#'
}

// jumpLetter moves to the first game of the next/previous initial letter.
func (s *gamesScreen) jumpLetter(dir int) {
	if s.l.N == 0 {
		return
	}
	letter := func(i int) rune { return letterOf(s.groups[s.view[i]].Title) }
	i := s.l.Sel
	cur := letter(i)
	if dir > 0 {
		for i < s.l.N && letter(i) == cur {
			i++
		}
		if i == s.l.N {
			i = 0
		}
	} else {
		start := i
		for start > 0 && letter(start-1) == cur {
			start--
		}
		if start != i {
			i = start
		} else {
			i = (start - 1 + s.l.N) % s.l.N
			for i > 0 && letter(i-1) == letter(i) {
				i--
			}
		}
	}
	s.l.Sel = i
}

var regionShort = map[string]string{
	"USA": "US", "Europe": "EU", "Japan": "JP", "World": "W", "Russia": "RU",
	"Germany": "DE", "France": "FR", "Spain": "ES", "Italy": "IT", "Korea": "KR",
	"China": "CN", "Brazil": "BR", "Australia": "AU", "UK": "UK", "Asia": "AS",
}

// variantLabel summarises a variant's regions, e.g. "US/EU".
func variantLabel(v catalog.Variant) string {
	var parts []string
	for _, r := range v.Info.Regions {
		if sh, ok := regionShort[r]; ok {
			r = sh
		}
		parts = append(parts, r)
	}
	return strings.Join(parts, "/")
}

func (s *gamesScreen) Draw(g *Gfx, area sdl.Rect) {
	switch {
	case !s.t.done:
		drawCentered(g, area, colDim, T("Loading…"))
		return
	case s.t.err != nil:
		drawCentered(g, area, colErr, T("Error: %s", s.t.err.Error()))
		return
	}

	// Info line: count, filter and destination folder.
	pad := g.S(16)
	infoH := g.S(28)
	info := T("%d games", len(s.view))
	if s.all {
		info += " · " + T("all versions")
	}
	if s.filter != "" {
		info += "   " + T("Filter: %s", s.filter)
	}
	iw := g.Text(info, pad, area.Y+(infoH-g.LineHeight(SizeSmall))/2, SizeSmall, false, colAccent)
	dest := s.dest
	if rel, err := filepath.Rel(filepath.Dir(s.a.opts.Platform.RomRoot), s.dest); err == nil {
		dest = rel
	}
	dest = g.Fit(dest, SizeSmall, false, area.W-iw-3*pad)
	g.TextRight(dest, area.W-pad, area.Y+(infoH-g.LineHeight(SizeSmall))/2, SizeSmall, false, colDim)
	area.Y += infoH
	area.H -= infoH

	if len(s.view) == 0 {
		drawCentered(g, area, colDim, T("Nothing here"))
		return
	}
	s.l.Draw(g, area, g.S(34), func(i int, r sdl.Rect, sel bool) {
		grp := s.groups[s.view[i]]
		best := grp.Best()
		var right []string
		if l := variantLabel(best); l != "" {
			right = append(right, l)
		}
		if n := len(grp.Variants) - 1; n > 0 {
			right = append(right, "+"+strconv.Itoa(n))
		}
		if best.Game.Size > 0 {
			right = append(right, formatBytes(best.Game.Size))
		}
		drawMarkedRow(g, r, grp.Title, strings.Join(right, "  "), sel, s.mark(grp.Variants))
	})
}

// drawMarkedRow draws a list row with an optional status square on the right.
func drawMarkedRow(g *Gfx, r sdl.Rect, text, right string, sel bool, mark Color) {
	if mark.A != 0 {
		d := g.S(8)
		g.Fill(r.X+r.W-g.S(12)-d, r.Y+(r.H-d)/2, d, d, mark)
		r.W -= g.S(16)
	}
	rowText(g, r, text, right, sel)
}

// ---- Variants --------------------------------------------------------------

// variantsScreen lists every version of one game, preferred first.
type variantsScreen struct {
	a     *app
	games *gamesScreen
	group catalog.Group
	l     list
}

func newVariantsScreen(a *app, games *gamesScreen, group catalog.Group) *variantsScreen {
	return &variantsScreen{a: a, games: games, group: group, l: list{N: len(group.Variants)}}
}

func (s *variantsScreen) Title() string { return s.group.Title }
func (s *variantsScreen) Update() bool  { return s.games.Update() }

func (s *variantsScreen) Hints() []Hint {
	return []Hint{{"A", T("Download")}, {"B", T("Back")}}
}

func (s *variantsScreen) Handle(act Action) {
	switch act {
	case Up:
		s.l.Move(-1)
	case Down:
		s.l.Move(1)
	case Left:
		s.l.Move(-s.l.Page())
	case Right:
		s.l.Move(s.l.Page())
	case A:
		if s.l.N > 0 {
			s.games.download(s.group.Variants[s.l.Sel : s.l.Sel+1])
		}
	case B:
		s.a.pop()
	}
}

func (s *variantsScreen) Draw(g *Gfx, area sdl.Rect) {
	area.Y += g.S(8)
	area.H -= g.S(8)
	s.l.Draw(g, area, g.S(34), func(i int, r sdl.Rect, sel bool) {
		v := s.group.Variants[i]
		right := ""
		if v.Game.Size > 0 {
			right = formatBytes(v.Game.Size)
		}
		if i == 0 {
			right = strings.TrimSpace(T("preferred") + "  " + right)
		}
		drawMarkedRow(g, r, v.Game.Name, right, sel, s.games.mark([]catalog.Variant{v}))
	})
}
