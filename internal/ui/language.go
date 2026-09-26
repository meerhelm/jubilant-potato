package ui

import "github.com/veandco/go-sdl2/sdl"

// languageScreen lets the user pick the UI language or follow the firmware.
type languageScreen struct {
	a *app
	l list
}

func newLanguageScreen(a *app) *languageScreen {
	s := &languageScreen{a: a, l: list{N: len(Languages) + 1}}
	for i, l := range Languages {
		if l.Code == a.opts.Config.Language {
			s.l.Sel = i + 1
		}
	}
	return s
}

func (s *languageScreen) Title() string { return T("Language") }
func (s *languageScreen) Update() bool  { return false }

func (s *languageScreen) Hints() []Hint {
	return []Hint{{"A", T("Select")}, {"B", T("Back")}}
}

func (s *languageScreen) Handle(act Action) {
	switch act {
	case Up:
		s.l.Move(-1)
	case Down:
		s.l.Move(1)
	case A:
		code := ""
		if s.l.Sel > 0 {
			code = Languages[s.l.Sel-1].Code
		}
		s.a.applyLanguage(code)
		s.a.pop()
	case B:
		s.a.pop()
	}
}

func (s *languageScreen) Draw(g *Gfx, area sdl.Rect) {
	area.Y += g.S(8)
	area.H -= g.S(8)
	current := s.a.opts.Config.Language
	s.l.Draw(g, area, g.S(38), func(i int, r sdl.Rect, sel bool) {
		if i == 0 {
			// Show what "automatic" resolves to on this device.
			auto := languageName(resolveLanguage("", s.a.opts.Platform.Language))
			mark := ""
			if current == "" {
				mark = "•  "
			}
			rowText(g, r, mark+T("Automatic"), auto, sel)
			return
		}
		l := Languages[i-1]
		mark := ""
		if l.Code == current {
			mark = "•  "
		}
		rowText(g, r, mark+l.Name, "", sel)
	})
}
