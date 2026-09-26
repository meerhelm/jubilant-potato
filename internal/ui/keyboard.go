package ui

import (
	"strings"

	"github.com/veandco/go-sdl2/sdl"
)

// Special keys on the last row of the on-screen keyboard.
const (
	keyShift = "Aa"
	keySpace = "Space"
	keyDel   = "Delete"
	keyClear = "Clear"
	keyDone  = "Done"
)

var keyRows = [][]string{
	{"1", "2", "3", "4", "5", "6", "7", "8", "9", "0"},
	{"q", "w", "e", "r", "t", "y", "u", "i", "o", "p"},
	{"a", "s", "d", "f", "g", "h", "j", "k", "l", ":"},
	{"z", "x", "c", "v", "b", "n", "m", ".", "/", "-"},
	{"_", "@", "'", "&", "?", "=", "%", "+", "!", "#"},
	{keyShift, keySpace, keyDel, keyClear, keyDone},
}

// keyboardScreen edits a line of text. onChange fires after every edit so
// callers can update results live; onDone fires when the user confirms.
type keyboardScreen struct {
	a        *app
	title    string
	text     []rune
	row, col int
	upper    bool
	secret   bool // show dots instead of the text (passwords)
	onChange func(string)
	onDone   func(string)
}

func newKeyboardScreen(a *app, title, text string, onChange func(string)) *keyboardScreen {
	return &keyboardScreen{a: a, title: title, text: []rune(text), row: 1, onChange: onChange}
}

// newInputScreen asks for a value and calls onDone with it; B cancels.
func newInputScreen(a *app, title, text string, secret bool, onDone func(string)) *keyboardScreen {
	k := newKeyboardScreen(a, title, text, nil)
	k.secret, k.onDone = secret, onDone
	return k
}

func (k *keyboardScreen) Title() string { return k.title }
func (k *keyboardScreen) Update() bool  { return false }

func (k *keyboardScreen) Hints() []Hint {
	return []Hint{{"A", T("Type")}, {"B", T("Delete")}, {"X", T("Space")}, {"Y", keyShift}, {"START", T("Done")}}
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
		if len(k.text) == 0 && k.onDone != nil {
			k.a.pop() // B on an empty field cancels the prompt
			return
		}
		k.press(keyDel)
	case X:
		k.press(keySpace)
	case Y:
		k.upper = !k.upper
	case Start:
		k.press(keyDone)
	case Select:
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

func (k *keyboardScreen) label(key string) string {
	if key == keyShift {
		return key
	}
	if len(key) > 1 {
		return T(key)
	}
	if k.upper {
		return strings.ToUpper(key)
	}
	return key
}

func (k *keyboardScreen) press(key string) {
	switch key {
	case keyDone:
		k.a.pop()
		if k.onDone != nil {
			k.onDone(string(k.text))
		}
		return
	case keyShift:
		k.upper = !k.upper
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
		k.text = append(k.text, []rune(k.label(key))...)
	}
	if k.onChange != nil {
		k.onChange(string(k.text))
	}
}

func (k *keyboardScreen) Draw(g *Gfx, area sdl.Rect) {
	pad := g.S(16)

	// Text field.
	fieldH := g.S(40)
	fy := area.Y + g.S(10)
	g.Fill(pad, fy, area.W-2*pad, fieldH, colBar)
	g.Fill(pad, fy+fieldH-g.S(2), area.W-2*pad, g.S(2), colAccent)
	shown := string(k.text)
	if k.secret {
		shown = strings.Repeat("•", len(k.text))
	}
	shown += "_"
	for g.Measure(shown, SizeTitle, false) > area.W-4*pad && len(shown) > 1 {
		shown = string([]rune(shown)[1:])
	}
	g.Text(shown, 2*pad, fy+(fieldH-g.LineHeight(SizeTitle))/2, SizeTitle, false, colText)

	// Key grid.
	gap := g.S(5)
	top := fy + fieldH + g.S(12)
	keyH := min((area.Y+area.H-top-g.S(6))/int32(len(keyRows))-gap, g.S(40))
	gridW := area.W - 2*pad
	for ri, row := range keyRows {
		keyW := (gridW - gap*int32(len(row)-1)) / int32(len(row))
		y := top + int32(ri)*(keyH+gap)
		for ci, key := range row {
			x := pad + int32(ci)*(keyW+gap)
			bg, fg := colBar, colText
			if key == keyShift && k.upper {
				fg = colAccent
			}
			if ri == k.row && ci == k.col {
				bg, fg = colAccent, colBg
			}
			g.Fill(x, y, keyW, keyH, bg)
			label := g.Fit(k.label(key), SizeNormal, true, keyW-g.S(6))
			lw := g.Measure(label, SizeNormal, true)
			g.Text(label, x+(keyW-lw)/2, y+(keyH-g.LineHeight(SizeNormal))/2, SizeNormal, true, fg)
		}
	}
}
