package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/veandco/go-sdl2/sdl"

	"github.com/meerhelm/jubilant-potato/internal/download"
	"github.com/meerhelm/jubilant-potato/internal/platform"
	"github.com/meerhelm/jubilant-potato/internal/source"
)

// ---- Sources ---------------------------------------------------------------

type sourcesScreen struct {
	a *app
	l list
}

func newSourcesScreen(a *app) *sourcesScreen {
	return &sourcesScreen{a: a, l: list{N: len(a.opts.Sources) + 1}}
}

func (s *sourcesScreen) Title() string { return "Jubilant Potato" }
func (s *sourcesScreen) Update() bool  { return false }

func (s *sourcesScreen) Hints() []Hint {
	return []Hint{{"A", T("Open")}, {"B", T("Quit")}}
}

func (s *sourcesScreen) Handle(act Action) {
	switch act {
	case Up:
		s.l.Move(-1)
	case Down:
		s.l.Move(1)
	case A:
		if s.l.Sel < len(s.a.opts.Sources) {
			s.a.push(newSystemsScreen(s.a, s.a.opts.Sources[s.l.Sel]))
		} else {
			s.a.push(newDownloadsScreen(s.a))
		}
	case Select:
		s.a.push(newDownloadsScreen(s.a))
	case B:
		s.a.pop()
	}
}

func (s *sourcesScreen) Draw(g *Gfx, area sdl.Rect) {
	srcs := s.a.opts.Sources
	rowH := g.S(40)
	listArea := area
	listArea.Y += g.S(8)
	s.l.Draw(g, listArea, rowH, func(i int, r sdl.Rect, sel bool) {
		if i < len(srcs) {
			rowText(g, r, srcs[i].Name(), "", sel)
			return
		}
		right := ""
		if n := s.a.opts.Manager.Active(); n > 0 {
			right = T("Downloads (%d active)", n)
		}
		rowText(g, r, "↓  "+T("Downloads"), right, sel)
	})
	if len(srcs) == 0 {
		msg := area
		msg.Y += rowH + g.S(8)
		msg.H -= rowH + g.S(8)
		drawCentered(g, msg, colDim, T("No sources configured."), T("Add them to config.json next to the app"))
	}
}

// ---- Systems ---------------------------------------------------------------

type systemsScreen struct {
	a   *app
	src source.Source
	t   *task[[]source.System]
	l   list
}

func newSystemsScreen(a *app, src source.Source) *systemsScreen {
	s := &systemsScreen{a: a, src: src}
	s.load()
	return s
}

func (s *systemsScreen) load() {
	s.t = startTask(func() ([]source.System, error) { return s.src.Systems(context.Background()) })
}

func (s *systemsScreen) Title() string { return s.src.Name() }

func (s *systemsScreen) Update() bool {
	if s.t.Poll() {
		s.l.SetN(len(s.t.val))
		return true
	}
	return false
}

func (s *systemsScreen) Hints() []Hint {
	return []Hint{{"A", T("Open")}, {"B", T("Back")}, {"SEL", T("Downloads")}}
}

func (s *systemsScreen) Handle(act Action) {
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
		if s.t.done && s.t.err == nil && s.l.N > 0 {
			s.a.push(newGamesScreen(s.a, s.src, s.t.val[s.l.Sel]))
		}
	case X:
		if s.t.done {
			s.load()
		}
	case Select:
		s.a.push(newDownloadsScreen(s.a))
	case B:
		s.a.pop()
	}
}

func (s *systemsScreen) Draw(g *Gfx, area sdl.Rect) {
	switch {
	case !s.t.done:
		drawCentered(g, area, colDim, T("Loading…"))
	case s.t.err != nil:
		drawCentered(g, area, colErr, T("Error: %s", s.t.err.Error()))
	case len(s.t.val) == 0:
		drawCentered(g, area, colDim, T("Nothing here"))
	default:
		area.Y += g.S(8)
		area.H -= g.S(8)
		s.l.Draw(g, area, g.S(40), func(i int, r sdl.Rect, sel bool) {
			sys := s.t.val[i]
			dir := filepath.Base(s.a.opts.Platform.SystemDir(sys.ID))
			rowText(g, r, sys.Label, "→ "+dir, sel)
		})
	}
}

// ---- Games -----------------------------------------------------------------

type gamesScreen struct {
	a    *app
	src  source.Source
	sys  source.System
	psys platform.System
	dest string

	t      *task[[]source.Game]
	filter string
	view   []int // indices into t.val matching filter
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
		s.applyFilter()
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

func (s *gamesScreen) applyFilter() {
	s.view = s.view[:0]
	words := strings.Fields(strings.ToLower(s.filter))
next:
	for i, g := range s.t.val {
		name := strings.ToLower(g.Name)
		for _, w := range words {
			if !strings.Contains(name, w) {
				continue next
			}
		}
		s.view = append(s.view, i)
	}
	s.l.SetN(len(s.view))
}

func (s *gamesScreen) Hints() []Hint {
	h := []Hint{{"A", T("Download")}, {"B", T("Back")}, {"Y", T("Search")}}
	if s.filter != "" {
		h = append(h, Hint{"X", T("Clear")})
	}
	return append(h, Hint{"L/R", T("Letter")})
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
		s.download()
	case Y:
		if s.t.done {
			s.a.push(newKeyboardScreen(s.a, T("Search"), s.filter, func(text string) {
				s.filter = text
				s.applyFilter()
			}))
		}
	case X:
		if s.filter != "" {
			s.filter = ""
			s.applyFilter()
		}
	case Select:
		s.a.push(newDownloadsScreen(s.a))
	case B:
		s.a.pop()
	}
}

func (s *gamesScreen) selected() (source.Game, bool) {
	if !s.t.done || s.l.N == 0 {
		return source.Game{}, false
	}
	return s.t.val[s.view[s.l.Sel]], true
}

func (s *gamesScreen) download() {
	g, ok := s.selected()
	switch {
	case !ok:
	case s.isInstalled(g):
		s.a.notify(T("Already installed"))
	case s.a.opts.Manager.Pending(g.URL):
		s.a.notify(T("Already in queue"))
	default:
		s.a.opts.Manager.Enqueue(g, s.dest, s.psys.Extract)
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
	letter := func(i int) rune { return letterOf(s.t.val[s.view[i]].Name) }
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
	mgr := s.a.opts.Manager
	s.l.Draw(g, area, g.S(34), func(i int, r sdl.Rect, sel bool) {
		game := s.t.val[s.view[i]]
		right := ""
		if game.Size > 0 {
			right = formatBytes(game.Size)
		}
		var mark Color
		switch {
		case s.isInstalled(game):
			mark = colOK
		case mgr.Pending(game.URL):
			mark = colAccent
		}
		if mark.A != 0 {
			d := g.S(8)
			g.Fill(r.X+r.W-g.S(12)-d, r.Y+(r.H-d)/2, d, d, mark)
			r.W -= g.S(16)
		}
		rowText(g, r, game.Name, right, sel)
	})
}
