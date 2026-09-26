package ui

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/veandco/go-sdl2/sdl"

	"github.com/meerhelm/jubilant-potato/internal/config"
	"github.com/meerhelm/jubilant-potato/internal/discover"
	"github.com/meerhelm/jubilant-potato/internal/platform"
	"github.com/meerhelm/jubilant-potato/internal/source"
)

// addSource saves a new source, rebuilds the source list and opens it.
func (a *app) addSource(c config.Source) {
	cfg := a.opts.Config
	cfg.Sources = append(cfg.Sources, c)
	if err := a.saveConfig(); err != nil {
		a.notify(T("Error: %s", err.Error()))
		return
	}
	a.reloadSources()
	a.stack = a.stack[:1]
	for _, s := range a.opts.Sources {
		if s.Name() == c.Name {
			a.push(openSource(a, s))
			break
		}
	}
}

// removeSource deletes the i-th configured source.
func (a *app) removeSource(name string) {
	cfg := a.opts.Config
	for i, c := range cfg.Sources {
		if c.Name == name {
			cfg.Sources = append(cfg.Sources[:i], cfg.Sources[i+1:]...)
			break
		}
	}
	if err := a.saveConfig(); err != nil {
		a.notify(T("Error: %s", err.Error()))
	}
	a.reloadSources()
}

func (a *app) saveConfig() error {
	if a.opts.SaveConfig == nil {
		return nil
	}
	return a.opts.SaveConfig()
}

func (a *app) reloadSources() {
	if a.opts.BuildSources == nil {
		return
	}
	var notices []string
	a.opts.Sources, notices = a.opts.BuildSources()
	if len(notices) > 0 {
		a.notify(strings.Join(notices, "; "))
	}
	if root, ok := a.stack[0].(*sourcesScreen); ok {
		root.l.SetN(len(a.opts.Sources) + 3)
	}
}

// uniqueName appends a counter when a source with this name exists.
func (a *app) uniqueName(name string) string {
	taken := map[string]bool{}
	for _, c := range a.opts.Config.Sources {
		taken[c.Name] = true
	}
	n, i := name, 2
	for taken[n] {
		n = fmt.Sprintf("%s %d", name, i)
		i++
	}
	return n
}

// ---- Menu helper -----------------------------------------------------------

// menuItem is one row of a simple selection screen.
type menuItem struct {
	label, right string
	action       func()
}

// menuScreen shows a static list of actions.
type menuScreen struct {
	a     *app
	title string
	items []menuItem
	l     list
}

func newMenuScreen(a *app, title string, items []menuItem) *menuScreen {
	return &menuScreen{a: a, title: title, items: items, l: list{N: len(items)}}
}

func (m *menuScreen) Title() string { return m.title }
func (m *menuScreen) Update() bool  { return false }
func (m *menuScreen) Hints() []Hint { return []Hint{{"A", T("Select")}, {"B", T("Back")}} }

func (m *menuScreen) Handle(act Action) {
	switch act {
	case Up:
		m.l.Move(-1)
	case Down:
		m.l.Move(1)
	case A:
		if m.l.N > 0 {
			m.items[m.l.Sel].action()
		}
	case B:
		m.a.pop()
	}
}

func (m *menuScreen) Draw(g *Gfx, area sdl.Rect) {
	area.Y += g.S(8)
	area.H -= g.S(8)
	m.l.Draw(g, area, g.S(40), func(i int, r sdl.Rect, sel bool) {
		rowText(g, r, m.items[i].label, m.items[i].right, sel)
	})
}

// confirm asks a yes/no question; A runs yes.
func confirm(a *app, question string, yes func()) {
	a.push(newMenuScreen(a, question, []menuItem{
		{label: T("Yes"), action: func() { a.pop(); yes() }},
		{label: T("No"), action: func() { a.pop() }},
	}))
}

func newAddSourceScreen(a *app) Screen {
	return newMenuScreen(a, T("Add source"), []menuItem{
		{label: "RomM", right: T("game library server"), action: func() { a.push(newRommFindScreen(a)) }},
		{label: T("Network folder (SMB)"), right: T("NAS, Windows, macOS"), action: func() { a.push(newSMBFindScreen(a)) }},
		{label: "archive.org", right: T("search collections"), action: func() { newArchiveSearch(a) }},
		{label: "itch.io", right: T("free homebrew"), action: func() {
			for _, c := range a.opts.Config.Sources {
				if c.Type == "itch" {
					a.notify(T("Already added"))
					return
				}
			}
			a.addSource(config.Source{Name: "itch.io", Type: "itch"})
		}},
		{label: T("Web folder (HTTP)"), right: T("directory listing"), action: func() {
			a.push(newInputScreen(a, T("Folder address"), "http://", false, func(u string) {
				if !validURL(u) {
					a.notify(T("Invalid address"))
					return
				}
				a.addSource(config.Source{Name: a.uniqueName(hostOf(u)), Type: "http", URL: u})
			}))
		}},
	})
}

func validURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func hostOf(s string) string {
	if u, err := url.Parse(s); err == nil && u.Host != "" {
		return u.Host
	}
	return s
}

// ---- Network scan screens --------------------------------------------------

// scanScreen lists hosts found by a network scan plus a manual-entry row.
type scanScreen struct {
	a      *app
	title  string
	scan   func(context.Context) []discover.Host
	pick   func(discover.Host)
	manual func()
	t      *task[[]discover.Host]
	l      list
}

func newScanScreen(a *app, title string, scan func(context.Context) []discover.Host, pick func(discover.Host), manual func()) *scanScreen {
	s := &scanScreen{a: a, title: title, scan: scan, pick: pick, manual: manual}
	s.start()
	return s
}

func (s *scanScreen) start() {
	s.t = startTask(func() ([]discover.Host, error) { return s.scan(context.Background()), nil })
	s.l = list{N: 1}
}

func (s *scanScreen) Title() string { return s.title }

func (s *scanScreen) Update() bool {
	if s.t.Poll() {
		s.l.SetN(len(s.t.val) + 1)
		return true
	}
	return false
}

func (s *scanScreen) Hints() []Hint {
	return []Hint{{"A", T("Select")}, {"X", T("Search again")}, {"B", T("Back")}}
}

func (s *scanScreen) Handle(act Action) {
	switch act {
	case Up:
		s.l.Move(-1)
	case Down:
		s.l.Move(1)
	case A:
		if s.l.Sel < len(s.t.val) {
			s.pick(s.t.val[s.l.Sel])
		} else {
			s.manual()
		}
	case X:
		if s.t.done {
			s.start()
		}
	case B:
		s.a.pop()
	}
}

func (s *scanScreen) Draw(g *Gfx, area sdl.Rect) {
	infoH := g.S(28)
	info := T("Searching %s…", discover.Describe())
	if s.t.done {
		info = T("Found: %d", len(s.t.val))
	}
	g.Text(info, g.S(16), area.Y+(infoH-g.LineHeight(SizeSmall))/2, SizeSmall, false, colAccent)
	area.Y += infoH
	area.H -= infoH
	s.l.Draw(g, area, g.S(40), func(i int, r sdl.Rect, sel bool) {
		if i < len(s.t.val) {
			h := s.t.val[i]
			rowText(g, r, h.Label(), h.Version, sel)
			return
		}
		rowText(g, r, "›  "+T("Enter address manually"), "", sel)
	})
}

func newRommFindScreen(a *app) Screen {
	add := func(u string) {
		a.addSource(config.Source{Name: a.uniqueName("RomM " + hostOf(u)), Type: "romm", URL: u})
	}
	return newScanScreen(a, T("Find RomM"), discover.RomM,
		func(h discover.Host) { add("http://" + h.Addr) },
		func() {
			a.push(newInputScreen(a, T("RomM address"), "http://", false, func(u string) {
				if !validURL(u) {
					a.notify(T("Invalid address"))
					return
				}
				add(strings.TrimRight(u, "/"))
			}))
		})
}

func newSMBFindScreen(a *app) Screen {
	return newScanScreen(a, T("Find network folders"), discover.SMB,
		func(h discover.Host) { a.push(newSMBSharesScreen(a, h.Addr, "", "")) },
		func() {
			a.push(newInputScreen(a, T("Server address"), "", false, func(host string) {
				if host = strings.TrimSpace(host); host != "" {
					a.push(newSMBSharesScreen(a, host, "", ""))
				}
			}))
		})
}

// ---- SMB browsing ----------------------------------------------------------

// smbLogin asks for a user name and password, then calls done with them.
func smbLogin(a *app, host string, done func(user, pass string)) {
	a.push(newInputScreen(a, T("User on %s", host), "", false, func(user string) {
		a.push(newInputScreen(a, T("Password for %s", user), "", true, func(pass string) {
			done(user, pass)
		}))
	}))
}

// smbListScreen shows a list loaded from an SMB server and asks for a login
// when the server refuses guest access.
type smbListScreen struct {
	a     *app
	title string
	host  string
	user  string
	pass  string
	load  func(user, pass string) ([]string, error)
	open  func(name, user, pass string)
	use   func(user, pass string) // optional first row: pick this folder
	useOf func([]string) string   // label for the use row

	t *task[[]string]
	l list
}

func (s *smbListScreen) start() {
	user, pass := s.user, s.pass
	s.t = startTask(func() ([]string, error) { return s.load(user, pass) })
	s.l = list{}
}

func (s *smbListScreen) offset() int {
	if s.use != nil {
		return 1
	}
	return 0
}

func (s *smbListScreen) Title() string { return s.title }

func (s *smbListScreen) Update() bool {
	if !s.t.Poll() {
		return false
	}
	if errors.Is(s.t.err, source.ErrAuthRequired) {
		smbLogin(s.a, s.host, func(user, pass string) {
			s.user, s.pass = user, pass
			s.start()
		})
	}
	s.l.SetN(len(s.t.val) + s.offset())
	return true
}

func (s *smbListScreen) Hints() []Hint {
	return []Hint{{"A", T("Open")}, {"B", T("Back")}}
}

func (s *smbListScreen) Handle(act Action) {
	switch act {
	case Up:
		s.l.Move(-1)
	case Down:
		s.l.Move(1)
	case A:
		switch {
		case s.t.done && errors.Is(s.t.err, source.ErrAuthRequired):
			smbLogin(s.a, s.host, func(user, pass string) {
				s.user, s.pass = user, pass
				s.start()
			})
		case !s.t.done || s.t.err != nil:
		case s.use != nil && s.l.Sel == 0:
			s.use(s.user, s.pass)
		case s.l.N > 0:
			s.open(s.t.val[s.l.Sel-s.offset()], s.user, s.pass)
		}
	case B:
		s.a.pop()
	}
}

func (s *smbListScreen) Draw(g *Gfx, area sdl.Rect) {
	switch {
	case !s.t.done:
		drawCentered(g, area, colDim, T("Connecting to %s…", s.host))
		return
	case errors.Is(s.t.err, source.ErrAuthRequired):
		drawCentered(g, area, colDim, T("Login required"), T("Press A to sign in"))
		return
	case s.t.err != nil:
		drawCentered(g, area, colErr, T("Error: %s", s.t.err.Error()))
		return
	}
	area.Y += g.S(8)
	area.H -= g.S(8)
	s.l.Draw(g, area, g.S(40), func(i int, r sdl.Rect, sel bool) {
		if s.use != nil && i == 0 {
			rowText(g, r, "→  "+T("Use this folder"), s.useOf(s.t.val), sel)
			return
		}
		rowText(g, r, s.t.val[i-s.offset()], "", sel)
	})
}

func newSMBSharesScreen(a *app, host, user, pass string) *smbListScreen {
	s := &smbListScreen{a: a, title: host, host: host, user: user, pass: pass}
	s.load = func(user, pass string) ([]string, error) {
		return source.SMBShares(context.Background(), host, user, pass)
	}
	s.open = func(share, user, pass string) { a.push(newSMBFolderScreen(a, host, share, "", user, pass)) }
	s.start()
	return s
}

func newSMBFolderScreen(a *app, host, share, dir, user, pass string) *smbListScreen {
	s := &smbListScreen{a: a, title: "//" + path.Join(host, share, dir), host: host, user: user, pass: pass}
	s.load = func(user, pass string) ([]string, error) {
		return source.SMBDirs(context.Background(), host, share, dir, user, pass)
	}
	s.open = func(sub, user, pass string) {
		a.push(newSMBFolderScreen(a, host, share, path.Join(dir, sub), user, pass))
	}
	s.use = func(user, pass string) {
		name := path.Join(host, share, dir)
		a.addSource(config.Source{
			Name: a.uniqueName(name), Type: "smb",
			Host: host, Share: share, Path: dir, Username: user, Password: pass,
		})
	}
	// Tell the user whether this looks like a ROM root: count system folders.
	s.useOf = func(dirs []string) string {
		n := 0
		for _, d := range dirs {
			if _, ok := platform.MatchSystem(d); ok {
				n++
			}
		}
		return T("systems: %d", n)
	}
	s.start()
	return s
}
