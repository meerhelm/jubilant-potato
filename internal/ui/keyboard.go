package ui

import (
	"github.com/veandco/go-sdl2/sdl"
)

// Special keys on the last row of the on-screen keyboard.
const (
	keySpace = "Space"
	keyDel   = "Delete"
	keyClear = "Clear"
	keyDone  = "Done"
)

var keyRows = [][]string{
	{"1", "2", "3", "4", "5", "6", "7", "8", "9", "0"},
	{"Q", "W", "E", "R", "T", "Y", "U", "I", "O", "P"},
	{"A", "S", "D", "F", "G", "H", "J", "K", "L", "-"},
	{"Z", "X", "C", "V", "B", "N", "M", ".", "'", "&"},
	{keySpace, keyDel, keyClear, keyDone},
}

// keyboardScreen edits a line of text; onChange fires after every edit so
// callers can update results live.
type keyboardScreen struct {
	a        *app
	title    string
	text     []rune
	row, col int
	onChange func(string)
}

func newKeyboardScreen(a *app, title, text string, onChange func(string)) *keyboardScreen {
	return &keyboardScreen{a: a, title: title, text: []rune(text), row: 1, onChange: onChange}
}

func (k *keyboardScreen) Title() string { return k.title }
func (k *keyboardScreen) Update() bool  { return false }

func (k *keyboardScreen) Hints() []Hint {
	return []Hint{{"A", T("Type")}, {"B", T("Delete")}, {"X", T("Space")}, {"START", T("Done")}}
}

func (k *keyboardScreen) Handle(act Action) {
	switch act {
	case Up:
		k.moveRow(-1)
	case Down:
		k.moveRow(1)
	case Left:
		k.col = (k.col - 1 + len(keyRows[k.row])) % len(keyRows[k.row])
	case Right:
		k.col = (k.col + 1) % len(keyRows[k.row])
	case A:
		k.press(keyRows[k.row][k.col])
	case B:
		k.press(keyDel)
	case X:
		k.press(keySpace)
	case Start, Select:
		k.a.pop()
	}
}

// moveRow keeps the cursor at roughly the same horizontal position when
// moving between rows of different lengths.
func (k *keyboardScreen) moveRow(d int) {
	from := len(keyRows[k.row])
	k.row = (k.row + d + len(keyRows)) % len(keyRows)
	to := len(keyRows[k.row])
	k.col = min((k.col*to+to/2)/from, to-1)
}

func (k *keyboardScreen) press(key string) {
	switch key {
	case keyDone:
		k.a.pop()
		return
	case keySpace:
		k.text = append(k.text, ' ')
	case keyDel:
		if len(k.text) > 0 {
			k.text = k.text[:len(k.text)-1]
		}
	case keyClear:
		k.text = k.text[:0]
	default:
		k.text = append(k.text, []rune(key)...)
	}
	k.onChange(string(k.text))
}

func (k *keyboardScreen) Draw(g *Gfx, area sdl.Rect) {
	pad := g.S(16)

	// Text field.
	fieldH := g.S(40)
	fy := area.Y + g.S(12)
	g.Fill(pad, fy, area.W-2*pad, fieldH, colBar)
	g.Fill(pad, fy+fieldH-g.S(2), area.W-2*pad, g.S(2), colAccent)
	shown := string(k.text) + "_"
	for g.Measure(shown, SizeTitle, false) > area.W-4*pad && len(shown) > 1 {
		shown = string([]rune(shown)[1:])
	}
	g.Text(shown, 2*pad, fy+(fieldH-g.LineHeight(SizeTitle))/2, SizeTitle, false, colText)

	// Key grid.
	gap := g.S(6)
	top := fy + fieldH + g.S(16)
	keyH := min((area.Y+area.H-top-g.S(8))/int32(len(keyRows))-gap, g.S(44))
	gridW := area.W - 2*pad
	for ri, row := range keyRows {
		keyW := (gridW - gap*int32(len(row)-1)) / int32(len(row))
		y := top + int32(ri)*(keyH+gap)
		for ci, key := range row {
			x := pad + int32(ci)*(keyW+gap)
			bg, fg := colBar, colText
			if ri == k.row && ci == k.col {
				bg, fg = colAccent, colBg
			}
			g.Fill(x, y, keyW, keyH, bg)
			label := key
			if len(key) > 1 {
				label = T(key)
			}
			label = g.Fit(label, SizeNormal, true, keyW-g.S(6))
			lw := g.Measure(label, SizeNormal, true)
			g.Text(label, x+(keyW-lw)/2, y+(keyH-g.LineHeight(SizeNormal))/2, SizeNormal, true, fg)
		}
	}
}
