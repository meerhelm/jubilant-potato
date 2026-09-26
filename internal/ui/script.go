package ui

import (
	"image"
	"image/png"
	"log"
	"os"
	"strings"
	"time"
	"unsafe"

	"github.com/veandco/go-sdl2/sdl"
)

// script replays actions from $POTATO_SCRIPT for testing and bug reports,
// e.g. POTATO_SCRIPT="a,down,a,wait,shot:games.png,quit".
// "wait" pauses one extra step; "shot:FILE" saves the next frame as PNG.
type script struct {
	steps []string
	next  time.Time
}

const scriptStep = 400 * time.Millisecond

func loadScript() *script {
	s := os.Getenv("POTATO_SCRIPT")
	if s == "" {
		return nil
	}
	return &script{steps: strings.Split(s, ","), next: time.Now().Add(scriptStep)}
}

func (s *script) run(a *app, now time.Time) bool {
	if s == nil || len(s.steps) == 0 || now.Before(s.next) {
		return false
	}
	step := strings.TrimSpace(s.steps[0])
	s.steps = s.steps[1:]
	s.next = now.Add(scriptStep)
	switch {
	case step == "wait":
	case step == "quit":
		a.quit = true
	case strings.HasPrefix(step, "shot:"):
		a.shot = strings.TrimPrefix(step, "shot:")
	default:
		if act, ok := actionNames[strings.ToLower(step)]; ok {
			a.dispatch(act)
		} else {
			log.Printf("script: unknown step %q", step)
		}
	}
	return true
}

// saveShot writes the current back buffer to path.
func (g *Gfx) saveShot(path string) error {
	img := image.NewRGBA(image.Rect(0, 0, int(g.W), int(g.H)))
	if err := g.r.ReadPixels(nil, sdl.PIXELFORMAT_ABGR8888, unsafe.Pointer(&img.Pix[0]), img.Stride); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
