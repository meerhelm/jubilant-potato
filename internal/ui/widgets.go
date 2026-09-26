package ui

import "github.com/veandco/go-sdl2/sdl"

// list is a scrolling selection over N rows.
type list struct {
	Sel, Top, N int
	visible     int // rows that fit, updated on draw
}

// Move moves the selection; single steps wrap around, pages clamp.
func (l *list) Move(d int) {
	if l.N == 0 {
		return
	}
	next := l.Sel + d
	switch {
	case (d == 1 || d == -1) && next < 0:
		next = l.N - 1
	case (d == 1 || d == -1) && next >= l.N:
		next = 0
	case next < 0:
		next = 0
	case next >= l.N:
		next = l.N - 1
	}
	l.Sel = next
}

// Page returns the number of rows in one screen page.
func (l *list) Page() int { return max(l.visible, 1) }

// SetN changes the row count and keeps the selection in range.
func (l *list) SetN(n int) {
	l.N = n
	if l.Sel >= n {
		l.Sel = max(n-1, 0)
	}
}

// Draw renders visible rows inside area, calling row for each one.
func (l *list) Draw(g *Gfx, area sdl.Rect, rowH int32, row func(i int, r sdl.Rect, selected bool)) {
	l.visible = max(int(area.H/rowH), 1)
	if l.Sel < l.Top {
		l.Top = l.Sel
	}
	if l.Sel >= l.Top+l.visible {
		l.Top = l.Sel - l.visible + 1
	}
	l.Top = max(min(l.Top, l.N-l.visible), 0)

	for i := l.Top; i < l.N && i < l.Top+l.visible; i++ {
		r := sdl.Rect{X: area.X, Y: area.Y + int32(i-l.Top)*rowH, W: area.W, H: rowH}
		if i == l.Sel {
			g.Fill(r.X, r.Y, r.W, r.H, colSel)
			g.Fill(r.X, r.Y, g.S(4), r.H, colAccent)
		}
		row(i, r, i == l.Sel)
	}

	// Scrollbar.
	if l.N > l.visible {
		trackH := area.H
		thumbH := max(trackH*int32(l.visible)/int32(l.N), g.S(16))
		thumbY := area.Y + (trackH-thumbH)*int32(l.Top)/int32(l.N-l.visible)
		g.Fill(area.X+area.W-g.S(4), thumbY, g.S(4), thumbH, colDim)
	}
}

// task runs f in the background; the UI polls it every frame.
type task[T any] struct {
	ch   chan struct{}
	done bool
	val  T
	err  error
}

func startTask[T any](f func() (T, error)) *task[T] {
	t := &task[T]{ch: make(chan struct{})}
	go func() {
		t.val, t.err = f()
		close(t.ch)
	}()
	return t
}

// Poll returns true exactly once, when the task has just finished.
func (t *task[T]) Poll() bool {
	if t.done {
		return false
	}
	select {
	case <-t.ch:
		t.done = true
		return true
	default:
		return false
	}
}

// drawCentered draws dimmed lines of text centered in area.
func drawCentered(g *Gfx, area sdl.Rect, c Color, lines ...string) {
	lh := g.LineHeight(SizeNormal) + g.S(6)
	y := area.Y + (area.H-lh*int32(len(lines)))/2
	for _, s := range lines {
		s = g.Fit(s, SizeNormal, false, area.W-g.S(24))
		g.Text(s, area.X+(area.W-g.Measure(s, SizeNormal, false))/2, y, SizeNormal, false, c)
		y += lh
	}
}

// rowText draws a single text row inside r with standard padding.
func rowText(g *Gfx, r sdl.Rect, s string, right string, selected bool) {
	pad := g.S(16)
	y := r.Y + (r.H-g.LineHeight(SizeNormal))/2
	rw := int32(0)
	if right != "" {
		rw = g.Measure(right, SizeSmall, false) + g.S(12)
		g.TextRight(right, r.X+r.W-pad, r.Y+(r.H-g.LineHeight(SizeSmall))/2, SizeSmall, false, colDim)
	}
	c := colText
	if !selected {
		c = Color{R: 0xc8, G: 0xc9, B: 0xd2, A: 0xff}
	}
	g.Text(g.Fit(s, SizeNormal, selected, r.W-2*pad-rw), r.X+pad, y, SizeNormal, selected, c)
}
