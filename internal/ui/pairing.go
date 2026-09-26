package ui

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/veandco/go-sdl2/sdl"
	"rsc.io/qr"

	"github.com/meerhelm/jubilant-potato/internal/source"
)

// pairingScreen links the device to a server account: it shows a QR code
// and a short code to approve in the server's web UI, then polls until the
// user does.
type pairingScreen struct {
	a      *app
	src    source.Source
	pairer source.Pairer

	start    *task[*source.Pairing]
	pairing  *source.Pairing
	qr       *qr.Code
	poll     *task[struct{}]
	nextPoll time.Time
	err      error
	lastSec  int
}

func newPairingScreen(a *app, src source.Source, p source.Pairer) *pairingScreen {
	s := &pairingScreen{a: a, src: src, pairer: p}
	s.begin()
	return s
}

func (s *pairingScreen) begin() {
	s.pairing, s.qr, s.poll, s.err = nil, nil, nil, nil
	s.start = startTask(func() (*source.Pairing, error) {
		return s.pairer.StartPairing(context.Background())
	})
}

func (s *pairingScreen) Title() string { return T("Connect to %s", s.src.Name()) }

func (s *pairingScreen) Hints() []Hint {
	if s.err != nil {
		return []Hint{{"A", T("Retry")}, {"B", T("Back")}}
	}
	return []Hint{{"B", T("Cancel")}}
}

func (s *pairingScreen) Handle(act Action) {
	switch act {
	case A:
		if s.err != nil {
			s.begin()
		}
	case B:
		s.a.pop()
	}
}

func (s *pairingScreen) Update() bool {
	changed := false
	if s.start.Poll() {
		changed = true
		if s.err = s.start.err; s.err == nil {
			s.pairing = s.start.val
			s.qr, s.err = qr.Encode(s.pairing.URL, qr.M)
			s.nextPoll = time.Now().Add(s.pairing.Interval)
		}
	}
	if s.pairing == nil || s.err != nil {
		return changed
	}

	if s.poll != nil && s.poll.Poll() {
		changed = true
		switch err := s.poll.err; {
		case err == nil:
			// Paired: continue straight to the source.
			s.a.pop()
			s.a.push(newSystemsScreen(s.a, s.src))
			s.a.notify(T("Connected"))
			return true
		case errors.Is(err, source.ErrPairingPending):
			s.nextPoll = time.Now().Add(s.pairing.Interval)
		default:
			s.err = err
		}
		s.poll = nil
	}
	if s.poll == nil && s.err == nil && time.Now().After(s.nextPoll) {
		if time.Now().After(s.pairing.Expires) {
			s.err = source.ErrPairingExpired
			return true
		}
		p := s.pairing
		s.poll = startTask(func() (struct{}, error) {
			return struct{}{}, s.pairer.PollPairing(context.Background(), p)
		})
	}
	// Redraw once a second for the countdown.
	if sec := int(time.Until(s.pairing.Expires).Seconds()); sec != s.lastSec {
		s.lastSec = sec
		changed = true
	}
	return changed
}

func (s *pairingScreen) Draw(g *Gfx, area sdl.Rect) {
	switch {
	case s.err != nil:
		msg := T("Error: %s", s.err.Error())
		switch {
		case errors.Is(s.err, source.ErrPairingExpired):
			msg = T("The code has expired")
		case errors.Is(s.err, source.ErrPairingDenied):
			msg = T("Pairing was declined")
		}
		drawCentered(g, area, colErr, msg, T("Press A to try again"))
		return
	case s.pairing == nil:
		drawCentered(g, area, colDim, T("Loading…"))
		return
	}

	pad := g.S(20)

	// QR code on a white quiet zone, as large as the area allows.
	side := min(area.H-2*pad, area.W/2-pad)
	modules := int32(s.qr.Size + 4)
	cell := side / modules
	side = cell * modules
	qx, qy := area.X+pad, area.Y+(area.H-side)/2
	g.Fill(qx, qy, side, side, Color{R: 0xff, G: 0xff, B: 0xff, A: 0xff})
	for y := 0; y < s.qr.Size; y++ {
		for x := 0; x < s.qr.Size; x++ {
			if s.qr.Black(x, y) {
				g.Fill(qx+int32(x+2)*cell, qy+int32(y+2)*cell, cell, cell, Color{A: 0xff})
			}
		}
	}

	// Instructions to the right of the code.
	tx := qx + side + pad
	tw := area.X + area.W - tx - pad
	y := qy
	line := func(text string, size int, bold bool, c Color, gap int) {
		g.Text(g.Fit(text, size, bold, tw), tx, y, size, bold, c)
		y += g.LineHeight(size) + g.S(gap)
	}
	line(T("Scan with your phone"), SizeNormal, true, colText, 4)
	line(T("or open in a browser:"), SizeSmall, false, colDim, 2)
	page := s.pairing.URL
	if u, err := url.Parse(page); err == nil {
		u.RawQuery = ""
		page = u.String()
	}
	line(page, SizeSmall, false, colAccent, 18)
	line(T("and enter the code:"), SizeSmall, false, colDim, 4)
	line(s.pairing.UserCode, SizeTitle+8, true, colAccent, 18)

	left := max(int(time.Until(s.pairing.Expires).Seconds()), 0)
	line(T("Waiting for approval…"), SizeSmall, false, colText, 2)
	line(T("Code expires in %s", fmt.Sprintf("%d:%02d", left/60, left%60)), SizeSmall, false, colDim, 0)
}
