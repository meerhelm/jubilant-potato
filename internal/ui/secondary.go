package ui

import (
	"fmt"

	"github.com/veandco/go-sdl2/sdl"

	"github.com/meerhelm/jubilant-potato/internal/download"
)

// secondary fills a second display (the RG DS bottom screen) with the logo
// and download progress, so the firmware menu doesn't show through.
type secondary struct {
	win  *sdl.Window
	r    *sdl.Renderer
	g    *Gfx
	logo *sdl.Texture

	version string
	shown   string // status line currently on screen
	drawn   bool
}

// openSecondary opens a fullscreen window on display 1, or returns nil when
// there is only one display.
func openSecondary(version string) *secondary {
	if n, err := sdl.GetNumVideoDisplays(); err != nil || n < 2 {
		return nil
	}
	// "Secondary" in the title also makes ROCKNIX's sway put it on screen 2.
	win, err := sdl.CreateWindow("Jubilant Potato Secondary",
		sdl.WINDOWPOS_CENTERED_MASK|1, sdl.WINDOWPOS_CENTERED_MASK|1, 0, 0, sdl.WINDOW_FULLSCREEN_DESKTOP)
	if err != nil {
		return nil
	}
	r, err := sdl.CreateRenderer(win, -1, sdl.RENDERER_ACCELERATED)
	if err != nil {
		if r, err = sdl.CreateRenderer(win, -1, sdl.RENDERER_SOFTWARE); err != nil {
			win.Destroy()
			return nil
		}
	}
	g, err := newGfx(win, r)
	if err != nil {
		r.Destroy()
		win.Destroy()
		return nil
	}
	return &secondary{win: win, r: r, g: g, version: version}
}

// draw repaints when the status changes; it retries until the compositor
// has given the window a size.
func (s *secondary) draw(m *download.Manager) {
	status := downloadStatus(m)
	if s.drawn && status == s.shown {
		return
	}
	g := s.g
	if !g.Clear() {
		return
	}
	if s.logo == nil {
		if tex, err := g.Image(potatoLogo(int(g.S(220)))); err == nil {
			s.logo = tex
		}
	}

	size := g.S(220)
	y := (g.H - size - g.S(90)) / 2
	if s.logo != nil {
		g.r.Copy(s.logo, nil, &sdl.Rect{X: (g.W - size) / 2, Y: y, W: size, H: size})
	}
	y += size + g.S(8)
	center := func(text string, size int, bold bool, c Color) {
		g.Text(text, (g.W-g.Measure(text, size, bold))/2, y, size, bold, c)
		y += g.LineHeight(size) + g.S(4)
	}
	center("Jubilant Potato", SizeTitle+10, true, colText)
	center(T("ROM downloader"), SizeNormal, false, colDim)

	pad := g.S(14)
	if status != "" {
		g.Fill(0, g.H-g.S(40), g.W, g.S(40), colBar)
		g.Text(g.Fit(status, SizeSmall, false, g.W-2*pad), pad, g.H-g.S(40)+(g.S(40)-g.LineHeight(SizeSmall))/2, SizeSmall, false, colAccent)
	} else {
		g.TextRight(s.version, g.W-pad, g.H-g.LineHeight(SizeSmall)-pad, SizeSmall, false, colDim)
	}
	g.Present()
	s.shown, s.drawn = status, true
}

// downloadStatus describes the running download, e.g. "↓ Game — 42%  (+2)".
func downloadStatus(m *download.Manager) string {
	var cur *download.Info
	queued := 0
	jobs := m.Jobs()
	for i := range jobs {
		switch j := &jobs[i]; j.State {
		case download.Downloading, download.Extracting:
			if cur == nil {
				cur = j
			}
		case download.Queued:
			queued++
		}
	}
	if cur == nil {
		return ""
	}
	s := "↓ " + cur.Name
	switch {
	case cur.State == download.Extracting:
		s += " — " + T("extracting")
	case cur.Total > 0:
		s += fmt.Sprintf(" — %d%%", cur.Done*100/cur.Total)
	}
	if queued > 0 {
		s += fmt.Sprintf("  (+%d)", queued)
	}
	return s
}

func (s *secondary) close() {
	if s.logo != nil {
		s.logo.Destroy()
	}
	s.g.destroy()
	s.r.Destroy()
	s.win.Destroy()
}
