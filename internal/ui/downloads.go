package ui

import (
	"fmt"
	"time"

	"github.com/veandco/go-sdl2/sdl"

	"github.com/meerhelm/jubilant-potato/internal/download"
)

type downloadsScreen struct {
	a        *app
	jobs     []download.Info
	l        list
	lastPoll time.Time
}

func newDownloadsScreen(a *app) *downloadsScreen {
	s := &downloadsScreen{a: a}
	s.refresh()
	return s
}

func (s *downloadsScreen) refresh() {
	s.jobs = s.a.opts.Manager.Jobs()
	s.l.SetN(len(s.jobs))
	s.lastPoll = time.Now()
}

func (s *downloadsScreen) Title() string { return T("Downloads") }

// Update refreshes progress four times a second.
func (s *downloadsScreen) Update() bool {
	if time.Since(s.lastPoll) < 250*time.Millisecond {
		return false
	}
	s.refresh()
	return true
}

func (s *downloadsScreen) Hints() []Hint {
	h := []Hint{{"B", T("Back")}}
	if j, ok := s.selected(); ok {
		switch j.State {
		case download.Queued, download.Downloading:
			h = append(h, Hint{"X", T("Cancel")})
		case download.Failed, download.Canceled:
			h = append(h, Hint{"A", T("Retry")})
		}
	}
	return append(h, Hint{"Y", T("Clear done")})
}

func (s *downloadsScreen) selected() (download.Info, bool) {
	if s.l.N == 0 {
		return download.Info{}, false
	}
	return s.jobs[s.l.Sel], true
}

func (s *downloadsScreen) Handle(act Action) {
	m := s.a.opts.Manager
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
		if j, ok := s.selected(); ok {
			m.Retry(j.ID)
		}
	case X:
		if j, ok := s.selected(); ok {
			m.Cancel(j.ID)
		}
	case Y:
		m.ClearFinished()
	case B:
		s.a.pop()
		return
	}
	s.refresh()
}

var stateNames = map[download.State]string{
	download.Queued:      "queued",
	download.Downloading: "downloading",
	download.Extracting:  "extracting",
	download.Done:        "done",
	download.Failed:      "failed",
	download.Canceled:    "canceled",
}

func (s *downloadsScreen) Draw(g *Gfx, area sdl.Rect) {
	if len(s.jobs) == 0 {
		drawCentered(g, area, colDim, T("No downloads yet"))
		return
	}
	area.Y += g.S(6)
	area.H -= g.S(6)
	pad := g.S(16)
	s.l.Draw(g, area, g.S(58), func(i int, r sdl.Rect, sel bool) {
		j := s.jobs[i]
		g.Text(g.Fit(j.Name, SizeNormal, sel, r.W-2*pad), r.X+pad, r.Y+g.S(6), SizeNormal, sel, colText)

		status, col := T(stateNames[j.State]), colDim
		switch j.State {
		case download.Done:
			col = colOK
		case download.Failed:
			col = colErr
			status += ": " + j.Err.Error()
		case download.Downloading:
			status += "  " + formatBytes(j.Done)
			if j.Total > 0 {
				status += " / " + formatBytes(j.Total) + fmt.Sprintf("  %d%%", j.Done*100/j.Total)
			}
		}

		y := r.Y + g.S(32)
		barW := r.W * 2 / 5
		textX := r.X + pad
		if j.State == download.Downloading || j.State == download.Extracting {
			bh := g.S(8)
			by := y + (g.LineHeight(SizeSmall)-bh)/2
			g.Fill(textX, by, barW, bh, colBar)
			fill := barW
			if j.State == download.Downloading {
				fill = 0
				if j.Total > 0 {
					fill = int32(int64(barW) * j.Done / j.Total)
				}
			}
			g.Fill(textX, by, fill, bh, colAccent)
			textX += barW + g.S(12)
		}
		g.Text(g.Fit(status, SizeSmall, false, r.X+r.W-pad-textX), textX, y, SizeSmall, false, col)
	})
}
