package ui

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/veandco/go-sdl2/sdl"

	"github.com/meerhelm/jubilant-potato/internal/source"
)

// ---- Sources ---------------------------------------------------------------

type sourcesScreen struct {
	a *app
	l list
}

// Rows after the sources.
const (
	rowAdd = iota
	rowDownloads
	rowButtons
	extraRows
)

func newSourcesScreen(a *app) *sourcesScreen {
	return &sourcesScreen{a: a, l: list{N: len(a.opts.Sources) + extraRows}}
}

func (s *sourcesScreen) Title() string { return "Jubilant Potato" }
func (s *sourcesScreen) Update() bool  { return false }

func (s *sourcesScreen) Hints() []Hint {
	h := []Hint{{"A", T("Open")}}
	if s.l.Sel < len(s.a.opts.Sources) {
		h = append(h, Hint{"X", T("Remove")})
	}
	return append(h, Hint{"B", T("Quit")})
}

func (s *sourcesScreen) Handle(act Action) {
	srcs := s.a.opts.Sources
	switch act {
	case Up:
		s.l.Move(-1)
	case Down:
		s.l.Move(1)
	case A:
		if s.l.Sel < len(srcs) {
			s.a.push(openSource(s.a, srcs[s.l.Sel]))
			return
		}
		switch s.l.Sel - len(srcs) {
		case rowAdd:
			s.a.push(newAddSourceScreen(s.a))
		case rowDownloads:
			s.a.push(newDownloadsScreen(s.a))
		case rowButtons:
			s.a.push(newButtonsScreen(s.a))
		}
	case X:
		if s.l.Sel < len(srcs) {
			name := srcs[s.l.Sel].Name()
			confirm(s.a, T("Remove %s?", name), func() { s.a.removeSource(name) })
		}
	case Select:
		s.a.push(newDownloadsScreen(s.a))
	case B:
		s.a.pop()
	}
}

func (s *sourcesScreen) Draw(g *Gfx, area sdl.Rect) {
	srcs := s.a.opts.Sources
	area.Y += g.S(8)
	area.H -= g.S(8)
	s.l.Draw(g, area, g.S(40), func(i int, r sdl.Rect, sel bool) {
		if i < len(srcs) {
			rowText(g, r, srcs[i].Name(), srcs[i].Kind(), sel)
			return
		}
		switch i - len(srcs) {
		case rowAdd:
			rowText(g, r, "+  "+T("Add source"), "", sel)
		case rowDownloads:
			right := ""
			if n := s.a.opts.Manager.Active(); n > 0 {
				right = T("Downloads (%d active)", n)
			}
			rowText(g, r, "↓  "+T("Downloads"), right, sel)
		case rowButtons:
			rowText(g, r, "≡  "+T("Button setup"), "", sel)
		}
	})
}

// openSource returns the first screen for a source: pairing if it needs
// credentials, otherwise its system list.
func openSource(a *app, src source.Source) Screen {
	if p, ok := src.(source.Pairer); ok && p.NeedsPairing() {
		return newPairingScreen(a, src, p)
	}
	return newSystemsScreen(a, src)
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
		if p, ok := s.src.(source.Pairer); ok && errors.Is(s.t.err, source.ErrNeedsPairing) {
			// Token missing or revoked on the server: pair again.
			s.a.pop()
			s.a.push(newPairingScreen(s.a, s.src, p))
			return true
		}
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
