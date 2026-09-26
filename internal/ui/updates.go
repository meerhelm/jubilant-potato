package ui

import (
	"context"
	"os"
	"strings"
	"sync/atomic"

	"github.com/veandco/go-sdl2/sdl"

	"github.com/meerhelm/jubilant-potato/internal/source"
	"github.com/meerhelm/jubilant-potato/internal/update"
)

// checkUpdates starts a background release check.
func (a *app) checkUpdates() {
	client := a.opts.HTTPClient
	a.updateCheck = startTask(func() (*update.Release, error) {
		return update.Latest(context.Background(), client, source.UserAgent)
	})
}

// updateAvailable returns the newer release, if the check found one.
func (a *app) updateAvailable() (*update.Release, bool) {
	t := a.updateCheck
	if t == nil || !t.done || t.err != nil || !update.Newer(a.opts.Version, t.val.Version) {
		return nil, false
	}
	return t.val, true
}

type updateState int

const (
	updChecking updateState = iota
	updUpToDate
	updAvailable
	updInstalling
	updDone
	updFailed
)

// updateScreen checks for, downloads and installs a new version.
type updateScreen struct {
	a       *app
	state   updateState
	err     error
	install *task[struct{}]
	done    atomic.Int64
	total   atomic.Int64
}

func newUpdateScreen(a *app) *updateScreen {
	if a.updateCheck == nil || a.updateCheck.done && a.updateCheck.err != nil {
		a.checkUpdates()
	}
	return &updateScreen{a: a}
}

func (s *updateScreen) Title() string { return T("Updates") }

func (s *updateScreen) Update() bool {
	switch s.state {
	case updChecking:
		t := s.a.updateCheck
		t.Poll()
		if !t.done {
			return false
		}
		switch _, ok := s.a.updateAvailable(); {
		case t.err != nil:
			s.state, s.err = updFailed, t.err
		case ok:
			s.state = updAvailable
		default:
			s.state = updUpToDate
		}
		return true
	case updInstalling:
		if s.install.Poll() {
			if s.install.err != nil {
				s.state, s.err = updFailed, s.install.err
			} else {
				s.state = updDone
			}
		}
		return true // progress
	}
	return false
}

func (s *updateScreen) Hints() []Hint {
	switch s.state {
	case updAvailable:
		return []Hint{{"A", T("Install")}, {"B", T("Back")}}
	case updDone:
		return []Hint{{"A", T("Restart")}, {"B", T("Later")}}
	case updInstalling:
		return nil
	}
	return []Hint{{"B", T("Back")}}
}

func (s *updateScreen) Handle(act Action) {
	switch {
	case act == A && s.state == updAvailable:
		s.startInstall()
	case act == A && s.state == updDone:
		s.a.restart, s.a.quit = true, true
	case act == B && s.state != updInstalling:
		s.a.pop()
	}
}

func (s *updateScreen) startInstall() {
	rel, _ := s.a.updateAvailable()
	asset, ok := rel.AssetFor(s.a.opts.Platform.Firmware)
	if !ok {
		s.state, s.err = updFailed, errNoPackage
		return
	}
	exe, err := os.Executable()
	if err != nil {
		s.state, s.err = updFailed, err
		return
	}
	s.total.Store(asset.Size)
	s.state = updInstalling
	client := s.a.opts.HTTPClient
	s.install = startTask(func() (struct{}, error) {
		return struct{}{}, update.Install(context.Background(), client, asset, exe, func(done, total int64) {
			s.done.Store(done)
			s.total.Store(total)
		})
	})
}

type updateError string

func (e updateError) Error() string { return string(e) }

const errNoPackage = updateError("no update package for this firmware; download it from the website")

func (s *updateScreen) Draw(g *Gfx, area sdl.Rect) {
	pad := g.S(24)
	y := area.Y + g.S(22)
	text := func(t string, size int, bold bool, c Color, gap int) {
		g.Text(g.Fit(t, size, bold, area.W-2*pad), area.X+pad, y, size, bold, c)
		y += g.LineHeight(size) + g.S(gap)
	}
	text(T("Installed: %s", s.a.opts.Version), SizeSmall, false, colDim, 14)

	switch s.state {
	case updChecking:
		text(T("Checking for updates…"), SizeNormal, false, colDim, 0)
	case updUpToDate:
		text(T("You have the latest version"), SizeTitle, true, colOK, 0)
	case updFailed:
		text(T("Update failed"), SizeTitle, true, colErr, 8)
		msg := s.err.Error()
		if s.err == errNoPackage {
			msg = T(msg)
		}
		text(msg, SizeSmall, false, colDim, 0)
	case updAvailable, updInstalling, updDone:
		rel, _ := s.a.updateAvailable()
		text(T("New version %s", rel.Version), SizeTitle, true, colAccent, 14)
		switch s.state {
		case updAvailable:
			for _, l := range noteLines(rel.Notes, 7) {
				text(l, SizeSmall, false, colText, 4)
			}
		case updInstalling:
			done, total := s.done.Load(), s.total.Load()
			barW, barH := area.W-2*pad, g.S(12)
			g.Fill(area.X+pad, y, barW, barH, colBar)
			if total > 0 {
				g.Fill(area.X+pad, y, int32(int64(barW)*min(done, total)/total), barH, colAccent)
			}
			y += barH + g.S(10)
			text(T("Downloading %s of %s", formatBytes(done), formatBytes(total)), SizeSmall, false, colDim, 0)
		case updDone:
			text(T("Installed. Restart to use the new version."), SizeNormal, false, colOK, 0)
		}
	}
}

// noteLines turns Markdown release notes into a few plain lines.
func noteLines(md string, n int) []string {
	var out []string
	for _, l := range strings.Split(md, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "|") || strings.HasPrefix(l, "#") {
			continue
		}
		l = strings.NewReplacer("**", "", "`", "", "- ", "• ").Replace(l)
		out = append(out, l)
		if len(out) == n {
			break
		}
	}
	return out
}
