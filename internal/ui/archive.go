package ui

import (
	"context"
	"slices"

	"github.com/veandco/go-sdl2/sdl"

	"github.com/meerhelm/jubilant-potato/internal/config"
	"github.com/meerhelm/jubilant-potato/internal/platform"
	"github.com/meerhelm/jubilant-potato/internal/source"
)

const archiveSourceName = "archive.org"

// archiveSearchScreen shows archive.org items matching a query.
type archiveSearchScreen struct {
	a     *app
	query string
	t     *task[[]source.ArchiveItem]
	l     list
}

func newArchiveSearch(a *app) {
	a.push(newInputScreen(a, T("Search archive.org"), "", false, func(q string) {
		if q == "" {
			return
		}
		s := &archiveSearchScreen{a: a, query: q}
		s.t = startTask(func() ([]source.ArchiveItem, error) {
			return source.ArchiveSearch(context.Background(), a.opts.HTTPClient, q)
		})
		a.push(s)
	}))
}

func (s *archiveSearchScreen) Title() string { return "archive.org: " + s.query }

func (s *archiveSearchScreen) Update() bool {
	if s.t.Poll() {
		s.l.SetN(len(s.t.val))
		return true
	}
	return false
}

func (s *archiveSearchScreen) Hints() []Hint {
	return []Hint{{"A", T("Select")}, {"B", T("Back")}}
}

func (s *archiveSearchScreen) Handle(act Action) {
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
		if s.t.done && s.l.N > 0 {
			s.a.push(newArchiveItemScreen(s.a, s.t.val[s.l.Sel]))
		}
	case B:
		s.a.pop()
	}
}

func (s *archiveSearchScreen) Draw(g *Gfx, area sdl.Rect) {
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
		s.l.Draw(g, area, g.S(36), func(i int, r sdl.Rect, sel bool) {
			it := s.t.val[i]
			rowText(g, r, it.Title, formatBytes(it.Size), sel)
		})
	}
}

// archiveDetected is the result of inspecting an item.
type archiveDetected struct {
	title  string
	counts []source.SystemCount
}

// archiveItemScreen lets the user pick which system an item's games are
// for: detected systems first with their file counts, then all others.
type archiveItemScreen struct {
	a    *app
	item source.ArchiveItem
	t    *task[archiveDetected]
	ids  []string
	l    list
}

func newArchiveItemScreen(a *app, item source.ArchiveItem) *archiveItemScreen {
	s := &archiveItemScreen{a: a, item: item}
	s.t = startTask(func() (archiveDetected, error) {
		title, counts, err := source.ArchiveDetect(context.Background(), a.opts.HTTPClient, item.ID)
		return archiveDetected{title, counts}, err
	})
	return s
}

func (s *archiveItemScreen) Title() string { return s.item.Title }

func (s *archiveItemScreen) Update() bool {
	if !s.t.Poll() {
		return false
	}
	for _, c := range s.t.val.counts {
		s.ids = append(s.ids, c.ID)
	}
	for _, sys := range platform.Systems() {
		if !slices.Contains(s.ids, sys.ID) {
			s.ids = append(s.ids, sys.ID)
		}
	}
	s.l.SetN(len(s.ids))
	return true
}

func (s *archiveItemScreen) Hints() []Hint {
	return []Hint{{"A", T("Add")}, {"B", T("Back")}}
}

func (s *archiveItemScreen) Handle(act Action) {
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
			s.a.addArchiveItem(s.ids[s.l.Sel], s.item.ID)
		}
	case B:
		s.a.pop()
	}
}

func (s *archiveItemScreen) Draw(g *Gfx, area sdl.Rect) {
	switch {
	case !s.t.done:
		drawCentered(g, area, colDim, T("Looking at the files…"))
		return
	case s.t.err != nil:
		drawCentered(g, area, colErr, T("Error: %s", s.t.err.Error()))
		return
	}
	infoH := g.S(28)
	info := T("Which system are these games for?")
	g.Text(g.Fit(info, SizeSmall, false, area.W-g.S(32)), g.S(16), area.Y+(infoH-g.LineHeight(SizeSmall))/2, SizeSmall, false, colAccent)
	area.Y += infoH
	area.H -= infoH
	files := map[string]int{}
	for _, c := range s.t.val.counts {
		files[c.ID] = c.Files
	}
	s.l.Draw(g, area, g.S(36), func(i int, r sdl.Rect, sel bool) {
		sys, _ := platform.SystemByID(s.ids[i])
		right := ""
		if n := files[sys.ID]; n > 0 {
			right = T("files: %d", n)
		}
		rowText(g, r, sys.Name, right, sel)
	})
}

// addArchiveItem adds an item under a system to the archive.org source,
// creating the source on first use, then opens that system.
func (a *app) addArchiveItem(systemID, item string) {
	cfg := a.opts.Config
	idx := -1
	for i, c := range cfg.Sources {
		if c.Type == "archive.org" && c.Name == archiveSourceName {
			idx = i
			break
		}
	}
	if idx < 0 {
		a.addSource(config.Source{
			Name: archiveSourceName, Type: "archive.org",
			Systems: map[string]config.StringList{systemID: {item}},
		})
		return
	}
	src := &cfg.Sources[idx]
	if src.Systems == nil {
		src.Systems = map[string]config.StringList{}
	}
	if !slices.Contains(src.Systems[systemID], item) {
		src.Systems[systemID] = append(src.Systems[systemID], item)
	}
	if err := a.saveConfig(); err != nil {
		a.notify(T("Error: %s", err.Error()))
		return
	}
	a.reloadSources()
	a.stack = a.stack[:1]
	for _, s := range a.opts.Sources {
		if s.Name() == archiveSourceName {
			a.push(newSystemsScreen(a, s))
		}
	}
	a.notify(T("Added to %s", archiveSourceName))
}
