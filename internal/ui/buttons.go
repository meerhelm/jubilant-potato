package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/veandco/go-sdl2/sdl"
)

// buttonStep is one control the wizard asks for, keyed by its SDL
// GameController name.
type buttonStep struct {
	key, label string
	optional   bool // skipped automatically when not pressed in time
}

var buttonSteps = []buttonStep{
	{key: "a", label: "A"},
	{key: "b", label: "B"},
	{key: "x", label: "X"},
	{key: "y", label: "Y"},
	{key: "leftshoulder", label: "L1"},
	{key: "rightshoulder", label: "R1"},
	{key: "lefttrigger", label: "L2", optional: true},
	{key: "righttrigger", label: "R2", optional: true},
	{key: "back", label: "Select"},
	{key: "start", label: "Start"},
	{key: "guide", label: "Menu", optional: true},
}

const (
	optionalTimeout = 6 * time.Second
	// Some pads report one physical button as two codes at once; ignore
	// presses right after a capture.
	captureDebounce = 400 * time.Millisecond
)

// buttonsScreen maps a gamepad by asking for each button in turn. It reads
// raw joystick events, so it works even when the current mapping is wrong,
// then saves an SDL mapping for the pad and applies it immediately.
type buttonsScreen struct {
	a     *app
	step  int
	got   map[string]string // SDL key -> "b3", "a2", "h0.1"
	used  map[string]bool
	joy   sdl.JoystickID
	haveJ bool

	stepStart time.Time
	lastHit   time.Time
	done      bool
	err       error
	lastSec   int
}

func newButtonsScreen(a *app) *buttonsScreen {
	s := &buttonsScreen{a: a, got: map[string]string{}, used: map[string]bool{}, stepStart: time.Now()}
	a.input.capture = s.capture
	return s
}

func (s *buttonsScreen) Title() string { return T("Button setup") }

func (s *buttonsScreen) Hints() []Hint {
	if s.done {
		return []Hint{{T("any button"), T("Done")}}
	}
	return nil
}

// Handle is unused: all input goes to capture while the wizard is open.
func (s *buttonsScreen) Handle(Action) {}

func (s *buttonsScreen) capture(ev sdl.Event) {
	now := time.Now()
	if k, ok := ev.(*sdl.KeyboardEvent); ok && !s.done && k.Type == sdl.KEYDOWN &&
		(k.Keysym.Sym == sdl.K_ESCAPE || k.Keysym.Sym == sdl.K_BACKSPACE) {
		s.finish(false)
		return
	}
	if s.done {
		if isPress(ev) && now.Sub(s.lastHit) > captureDebounce {
			s.a.input.capture = nil
			s.a.pop()
		}
		return
	}
	if now.Sub(s.lastHit) < captureDebounce {
		return
	}
	var which sdl.JoystickID
	var bind string
	switch e := ev.(type) {
	case *sdl.JoyButtonEvent:
		if e.State != sdl.PRESSED {
			return
		}
		which, bind = e.Which, fmt.Sprintf("b%d", e.Button)
	case *sdl.JoyAxisEvent:
		// Analog triggers rest at one end; only a strong push counts.
		if e.Value < 24000 && e.Value > -24000 {
			return
		}
		which, bind = e.Which, fmt.Sprintf("a%d", e.Axis)
	default:
		return
	}
	if s.haveJ && which != s.joy {
		return // ignore other pads
	}
	if s.used[bind] {
		return
	}
	s.joy, s.haveJ = which, true
	s.got[buttonSteps[s.step].key] = bind
	s.used[bind] = true
	s.lastHit = now
	s.next()
}

func isPress(ev sdl.Event) bool {
	switch e := ev.(type) {
	case *sdl.JoyButtonEvent:
		return e.State == sdl.PRESSED
	case *sdl.KeyboardEvent:
		return e.Type == sdl.KEYDOWN
	}
	return false
}

func (s *buttonsScreen) next() {
	s.step++
	s.stepStart = time.Now()
	if s.step == len(buttonSteps) {
		s.finish(true)
	}
}

func (s *buttonsScreen) Update() bool {
	if s.done {
		return false
	}
	st := buttonSteps[s.step]
	if st.optional && time.Since(s.stepStart) > optionalTimeout {
		s.next()
		return true
	}
	if sec := int(time.Since(s.stepStart).Seconds()); sec != s.lastSec {
		s.lastSec = sec
		return st.optional
	}
	return false
}

// finish saves and applies the mapping, or cancels when save is false.
func (s *buttonsScreen) finish(save bool) {
	s.done = true
	s.lastHit = time.Now()
	if !save || !s.haveJ {
		s.a.input.capture = nil
		s.a.pop()
		return
	}
	s.err = s.a.input.applyMapping(s.joy, s.got, s.a.opts.Config)
	if s.err == nil && s.a.opts.SaveConfig != nil {
		s.err = s.a.opts.SaveConfig()
	}
}

func (s *buttonsScreen) Draw(g *Gfx, area sdl.Rect) {
	pad := g.S(24)
	y := area.Y + g.S(24)
	if s.done {
		msg, c := T("Buttons saved"), colOK
		if s.err != nil {
			msg, c = T("Error: %s", s.err.Error()), colErr
		}
		g.Text(g.Fit(msg, SizeTitle, true, area.W-2*pad), pad, y, SizeTitle, true, c)
		y += g.LineHeight(SizeTitle) + g.S(16)
		// Summary in two columns.
		colW := (area.W - 2*pad) / 2
		for i, st := range buttonSteps {
			x := pad + int32(i%2)*colW
			yy := y + int32(i/2)*(g.LineHeight(SizeNormal)+g.S(6))
			bind := s.got[st.key]
			if bind == "" {
				bind = "—"
			}
			g.Text(st.label, x, yy, SizeNormal, true, colText)
			g.Text(bind, x+g.S(90), yy, SizeNormal, false, colDim)
		}
		return
	}

	st := buttonSteps[s.step]
	g.Text(T("Step %d of %d", s.step+1, len(buttonSteps)), pad, y, SizeSmall, false, colDim)
	y += g.LineHeight(SizeSmall) + g.S(30)
	prompt := T("Press %s", "")
	pw := g.Measure(prompt, SizeTitle, false)
	lw := g.Measure(st.label, SizeTitle+26, true)
	x := (area.W - pw - lw) / 2
	g.Text(prompt, x, y+g.S(14), SizeTitle, false, colText)
	g.Text(st.label, x+pw, y, SizeTitle+26, true, colAccent)
	y += g.LineHeight(SizeTitle+26) + g.S(30)

	var notes []string
	if st.optional {
		left := int((optionalTimeout - time.Since(s.stepStart)).Seconds()) + 1
		notes = append(notes, T("No such button? Wait %d s to skip", max(left, 0)))
	}
	notes = append(notes, T("Esc on a keyboard cancels"))
	for _, n := range notes {
		g.Text(n, (area.W-g.Measure(n, SizeSmall, false))/2, y, SizeSmall, false, colDim)
		y += g.LineHeight(SizeSmall) + g.S(6)
	}
}

// buildMapping turns captured bindings into an SDL GameController mapping,
// keeping d-pad and stick bindings from the pad's current mapping.
func buildMapping(guid, name, current string, got map[string]string) string {
	fields := []string{guid, strings.ReplaceAll(name, ",", " ")}
	for _, st := range buttonSteps {
		if b := got[st.key]; b != "" {
			fields = append(fields, st.key+":"+b)
		}
	}
	used := map[string]bool{}
	for _, b := range got {
		used[b] = true
	}
	keep := map[string]string{
		"dpup": "h0.1", "dpdown": "h0.4", "dpleft": "h0.8", "dpright": "h0.2",
	}
	for _, f := range strings.Split(current, ",") {
		k, v, ok := strings.Cut(f, ":")
		if !ok || used[v] {
			continue // a control the wizard just assigned elsewhere
		}
		switch k {
		case "dpup", "dpdown", "dpleft", "dpright", "leftx", "lefty", "rightx", "righty", "leftstick", "rightstick":
			keep[k] = v
		}
	}
	for _, k := range []string{"dpup", "dpdown", "dpleft", "dpright", "leftx", "lefty", "rightx", "righty", "leftstick", "rightstick"} {
		if v, ok := keep[k]; ok {
			fields = append(fields, k+":"+v)
		}
	}
	return strings.Join(append(fields, "platform:Linux"), ",") + ","
}
