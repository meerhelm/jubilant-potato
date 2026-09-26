// Package ui is the SDL2 gamepad-driven interface.
package ui

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/veandco/go-sdl2/sdl"

	"github.com/meerhelm/jubilant-potato/internal/catalog"
	"github.com/meerhelm/jubilant-potato/internal/config"
	"github.com/meerhelm/jubilant-potato/internal/download"
	"github.com/meerhelm/jubilant-potato/internal/platform"
	"github.com/meerhelm/jubilant-potato/internal/source"
	"github.com/meerhelm/jubilant-potato/internal/update"
)

// Options configures Run.
type Options struct {
	Config   *config.Config
	Platform platform.Platform
	Sources  []source.Source
	Manager  *download.Manager
	// HTTPClient is shared by catalogs and downloads.
	HTTPClient *http.Client
	Window     string // "WxH" for a desktop window; empty = fullscreen
	Version    string
	Notices    []string // problems to show on start (bad config entries, ...)

	// SaveConfig persists Config after the UI changes it.
	SaveConfig func() error
	// BuildSources recreates Sources from Config.Sources after edits and
	// returns problems with individual entries.
	BuildSources func() ([]source.Source, []string)
}

// Hint is a button legend shown in the footer.
type Hint struct{ Btn, Label string }

// Screen is one page of the UI stack.
type Screen interface {
	Title() string
	Handle(a Action)
	Update() bool // polls background work, returns true when a redraw is needed
	Draw(g *Gfx, area sdl.Rect)
	Hints() []Hint
}

type app struct {
	opts  Options
	gfx   *Gfx
	input *Input
	stack []Screen
	quit  bool

	toast      string
	toastUntil time.Time

	free     string
	freeScan time.Time

	prefs catalog.Prefs

	second *secondary // bottom screen on dual-screen devices, may be nil

	updateCheck    *task[*update.Release] // nil until a check starts
	updateNotified bool
	restart        bool // relaunch the (updated) binary after quitting
	script         *script
	shot           string // save the next frame to this path
}

// Run opens the window and blocks until the user quits. It reports whether
// the app should restart itself (after an update). It must be called from
// the main OS thread.
func Run(opts Options) (restart bool, err error) {
	setLanguage(opts.Config.Language)

	if err := sdl.Init(sdl.INIT_VIDEO | sdl.INIT_JOYSTICK | sdl.INIT_GAMECONTROLLER); err != nil {
		return false, err
	}
	defer sdl.Quit()
	sdl.ShowCursor(sdl.DISABLE)
	addMappings(opts.Config)

	// Open the second screen first so the main window ends up focused.
	var second *secondary
	if opts.Window == "" {
		if second = openSecondary(opts.Version); second != nil {
			defer second.close()
		}
	}

	var w, h int32 = 0, 0
	flags := uint32(sdl.WINDOW_FULLSCREEN_DESKTOP)
	if opts.Window != "" {
		ws, hs, _ := strings.Cut(opts.Window, "x")
		wi, err1 := strconv.Atoi(ws)
		hi, err2 := strconv.Atoi(hs)
		if err1 != nil || err2 != nil {
			return false, fmt.Errorf("invalid window size %q, want WxH", opts.Window)
		}
		w, h, flags = int32(wi), int32(hi), sdl.WINDOW_SHOWN|sdl.WINDOW_ALLOW_HIGHDPI
	}
	win, err := sdl.CreateWindow("Jubilant Potato", sdl.WINDOWPOS_CENTERED, sdl.WINDOWPOS_CENTERED, w, h, flags)
	if err != nil {
		return false, err
	}
	defer win.Destroy()

	r, err := sdl.CreateRenderer(win, -1, sdl.RENDERER_ACCELERATED|sdl.RENDERER_PRESENTVSYNC)
	if err != nil {
		if r, err = sdl.CreateRenderer(win, -1, sdl.RENDERER_SOFTWARE); err != nil {
			return false, err
		}
	}
	defer r.Destroy()

	g, err := newGfx(win, r)
	if err != nil {
		return false, err
	}
	defer g.destroy()

	a := &app{opts: opts, gfx: g, input: newInput(opts.Config.SwapAB, joystickButtons(opts)), script: loadScript(), second: second}
	defer a.input.closeAll()
	a.prefs = catalog.DefaultPrefs(lang)
	if len(opts.Config.PreferRegions) > 0 {
		a.prefs.Regions = opts.Config.PreferRegions
	}
	if len(opts.Config.PreferLanguages) > 0 {
		a.prefs.Languages = opts.Config.PreferLanguages
	}
	a.push(newSourcesScreen(a))
	if len(opts.Notices) > 0 {
		a.notify(strings.Join(opts.Notices, "; "))
	}
	if update.IsRelease(opts.Version) && !opts.Config.DisableUpdateCheck {
		a.checkUpdates()
	}
	a.loop()
	return a.restart, nil
}

func (a *app) loop() {
	dirty := true
	var lastDraw time.Time
	for !a.quit {
		for ev := sdl.PollEvent(); ev != nil; ev = sdl.PollEvent() {
			switch ev.(type) {
			case *sdl.QuitEvent:
				a.quit = true
			case *sdl.WindowEvent:
				dirty = true
			}
			for _, act := range a.input.Handle(ev) {
				a.dispatch(act)
				dirty = true
			}
		}
		now := time.Now()
		for _, act := range a.input.Repeats(now) {
			a.dispatch(act)
			dirty = true
		}
		if a.script.run(a, now) {
			dirty = true
		}
		if len(a.stack) == 0 || a.quit {
			return
		}
		if a.top().Update() {
			dirty = true
		}
		if a.updateCheck != nil && !a.updateNotified && a.updateCheck.Poll() {
			if rel, ok := a.updateAvailable(); ok {
				a.notify(T("Update available: %s", rel.Version))
				dirty = true
			}
			a.updateNotified = true
		}
		if a.toast != "" && now.After(a.toastUntil) {
			a.toast = ""
			dirty = true
		}
		// Redraw on change, and periodically for progress/free space.
		if dirty || now.Sub(lastDraw) > 500*time.Millisecond {
			if a.draw() {
				dirty, lastDraw = false, now
			}
			if a.second != nil {
				a.second.draw(a.opts.Manager)
			}
		}
		sdl.Delay(16)
	}
}

func (a *app) dispatch(act Action) {
	if act == Menu {
		a.quit = true
		return
	}
	a.top().Handle(act)
}

func (a *app) top() Screen { return a.stack[len(a.stack)-1] }

func (a *app) push(s Screen) { a.stack = append(a.stack, s) }

func (a *app) pop() {
	a.stack = a.stack[:len(a.stack)-1]
	if len(a.stack) == 0 {
		a.quit = true
	}
}

func (a *app) notify(msg string) {
	a.toast = msg
	a.toastUntil = time.Now().Add(3 * time.Second)
}

// draw renders a frame and reports whether it could.
func (a *app) draw() bool {
	g := a.gfx
	if !g.Clear() {
		return false
	}

	headerH, footerH := g.S(40), g.S(34)
	pad := g.S(12)
	s := a.top()

	// Header: title on the left, free space and download count on the right.
	g.Fill(0, 0, g.W, headerH, colBar)
	g.Fill(0, headerH-g.S(2), g.W, g.S(2), colAccent)
	right := a.freeSpace()
	if n := a.opts.Manager.Active(); n > 0 {
		right = fmt.Sprintf("↓ %d   %s", n, right)
	}
	rw := g.Measure(right, SizeSmall, false)
	g.TextRight(right, g.W-pad, (headerH-g.LineHeight(SizeSmall))/2, SizeSmall, false, colDim)
	title := g.Fit(s.Title(), SizeTitle, true, g.W-rw-3*pad)
	g.Text(title, pad, (headerH-g.LineHeight(SizeTitle))/2, SizeTitle, true, colText)

	s.Draw(g, sdl.Rect{X: 0, Y: headerH, W: g.W, H: g.H - headerH - footerH})

	// Footer: button legends.
	g.Fill(0, g.H-footerH, g.W, footerH, colBar)
	x := pad
	for _, h := range s.Hints() {
		x += drawHint(g, x, g.H-footerH, footerH, h) + g.S(14)
	}

	if a.toast != "" {
		tw := g.Measure(a.toast, SizeNormal, false)
		maxW := g.W - 4*pad
		if tw > maxW {
			tw = maxW
		}
		bh := g.LineHeight(SizeNormal) + g.S(16)
		bx, by := (g.W-tw)/2-g.S(12), g.H-footerH-bh-g.S(10)
		g.Fill(bx, by, tw+g.S(24), bh, colSel)
		g.Fill(bx, by, g.S(4), bh, colAccent)
		g.Text(g.Fit(a.toast, SizeNormal, false, maxW), bx+g.S(12), by+g.S(8), SizeNormal, false, colText)
	}
	if a.shot != "" {
		if err := g.saveShot(a.shot); err != nil {
			log.Printf("screenshot: %v", err)
		}
		a.shot = ""
	}
	g.Present()
	return true
}

func drawHint(g *Gfx, x, y, h int32, hint Hint) int32 {
	d := g.S(22)
	bw := d
	if lw := g.Measure(hint.Btn, SizeSmall, true) + g.S(10); lw > bw {
		bw = lw
	}
	by := y + (h-d)/2
	g.Fill(x, by, bw, d, colBtnFace)
	g.Text(hint.Btn, x+(bw-g.Measure(hint.Btn, SizeSmall, true))/2, by+(d-g.LineHeight(SizeSmall))/2, SizeSmall, true, colText)
	lx := x + bw + g.S(6)
	lw := g.Text(hint.Label, lx, y+(h-g.LineHeight(SizeSmall))/2, SizeSmall, false, colDim)
	return lx + lw - x
}

func (a *app) freeSpace() string {
	if time.Since(a.freeScan) > 5*time.Second {
		a.freeScan = time.Now()
		if n, err := platform.FreeBytes(a.opts.Platform.RomRoot); err == nil {
			a.free = T("Free: %s", formatBytes(int64(n)))
		}
	}
	return a.free
}

func formatBytes(n int64) string {
	switch {
	case n >= 1<<40:
		return fmt.Sprintf("%.1f TB", float64(n)/(1<<40))
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%d KB", n>>10)
	}
	return fmt.Sprintf("%d B", n)
}

// joystickButtons is the raw-button fallback map: the user's, else the
// firmware default.
func joystickButtons(opts Options) map[string]string {
	if len(opts.Config.JoystickButtons) > 0 {
		return opts.Config.JoystickButtons
	}
	return opts.Platform.DefaultJoystickButtons()
}
