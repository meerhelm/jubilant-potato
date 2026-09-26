package ui

import (
	"github.com/veandco/go-sdl2/sdl"
	"rsc.io/qr"
)

// DonateURL is where the "Support the project" screen points.
const DonateURL = "https://buymeacoffee.com/f2mzlbyhvc"

// supportScreen shows a QR code for the donation page.
type supportScreen struct {
	a    *app
	code *qr.Code
}

func newSupportScreen(a *app) *supportScreen {
	code, _ := qr.Encode(DonateURL, qr.M)
	return &supportScreen{a: a, code: code}
}

func (s *supportScreen) Title() string { return T("Support the project") }
func (s *supportScreen) Update() bool  { return false }
func (s *supportScreen) Hints() []Hint { return []Hint{{"B", T("Back")}} }

func (s *supportScreen) Handle(act Action) {
	if act == B || act == A {
		s.a.pop()
	}
}

func (s *supportScreen) Draw(g *Gfx, area sdl.Rect) {
	pad := g.S(20)
	qx := area.X + pad
	qy, side := drawQR(g, s.code, qx, area.Y+pad, min(area.H-2*pad, area.W/2-pad), area.H-2*pad)

	tx := qx + side + pad
	tw := area.X + area.W - tx - pad
	y := qy + g.S(6)
	line := func(text string, size int, bold bool, c Color, gap int) {
		g.Text(g.Fit(text, size, bold, tw), tx, y, size, bold, c)
		y += g.LineHeight(size) + g.S(gap)
	}
	line(T("Enjoying the potato?"), SizeTitle, true, colText, 12)
	line(T("Jubilant Potato is free and open source."), SizeSmall, false, colDim, 2)
	line(T("A coffee buys test devices"), SizeSmall, false, colDim, 2)
	line(T("and support for more handhelds."), SizeSmall, false, colDim, 18)
	line(T("Scan to buy me a coffee:"), SizeSmall, false, colText, 4)
	line("buymeacoffee.com/f2mzlbyhvc", SizeSmall, false, colAccent, 18)
	line(T("Thank you!"), SizeNormal, true, colAccent, 0)
}
