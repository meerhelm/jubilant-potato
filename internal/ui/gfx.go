package ui

import (
	"image"
	"unsafe"

	"github.com/veandco/go-sdl2/sdl"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Font sizes in pixels at the 480p reference height.
const (
	SizeSmall  = 16
	SizeNormal = 20
	SizeTitle  = 24
)

type Color = sdl.Color

var (
	colBg      = Color{R: 0x16, G: 0x17, B: 0x1d, A: 0xff}
	colBar     = Color{R: 0x22, G: 0x24, B: 0x2d, A: 0xff}
	colSel     = Color{R: 0x3a, G: 0x3f, B: 0x55, A: 0xff}
	colText    = Color{R: 0xe8, G: 0xe8, B: 0xee, A: 0xff}
	colDim     = Color{R: 0x8a, G: 0x8d, B: 0x9c, A: 0xff}
	colAccent  = Color{R: 0xf2, G: 0xb1, B: 0x3b, A: 0xff} // potato gold
	colOK      = Color{R: 0x6c, G: 0xc9, B: 0x6e, A: 0xff}
	colErr     = Color{R: 0xe5, G: 0x5b, B: 0x5b, A: 0xff}
	colBtnFace = Color{R: 0x44, G: 0x47, B: 0x55, A: 0xff}
)

type textKey struct {
	s    string
	size int
	bold bool
}

type textTex struct {
	tex      *sdl.Texture
	w, h     int32
	lastUsed uint64
}

// Gfx wraps the SDL renderer with cached text drawing.
type Gfx struct {
	win *sdl.Window
	r   *sdl.Renderer
	W   int32
	H   int32

	scale   float64
	regular *opentype.Font
	bold    *opentype.Font
	faces   map[textKey]font.Face // keyed by size/bold only
	cache   map[textKey]*textTex
	frame   uint64
}

func newGfx(win *sdl.Window, r *sdl.Renderer) (*Gfx, error) {
	reg, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return nil, err
	}
	bold, err := opentype.Parse(gobold.TTF)
	if err != nil {
		return nil, err
	}
	g := &Gfx{
		win: win, r: r, regular: reg, bold: bold,
		faces: map[textKey]font.Face{},
		cache: map[textKey]*textTex{},
	}
	g.syncSize()
	return g, nil
}

// syncSize picks up the current output size. Under Wayland the window is
// 0x0 until the compositor configures it, and it may change later, so this
// runs every frame; it reports whether there is anything to draw on.
func (g *Gfx) syncSize() bool {
	w, h, err := g.r.GetOutputSize()
	if err != nil || w <= 0 || h <= 0 {
		return false
	}
	if w == g.W && h == g.H {
		return true
	}
	g.W, g.H = w, h
	g.scale = float64(h) / 480
	g.destroy() // glyphs are rasterized for the old scale
	g.faces = map[textKey]font.Face{}
	g.cache = map[textKey]*textTex{}
	return true
}

// S scales a length given at the 480p reference height.
func (g *Gfx) S(v int) int32 { return int32(float64(v)*g.scale + 0.5) }

func (g *Gfx) face(size int, bold bool) font.Face {
	k := textKey{size: size, bold: bold}
	if f, ok := g.faces[k]; ok {
		return f
	}
	fnt := g.regular
	if bold {
		fnt = g.bold
	}
	f, err := opentype.NewFace(fnt, &opentype.FaceOptions{
		Size: float64(g.S(size)), DPI: 72, Hinting: font.HintingFull,
	})
	if err != nil {
		panic(err) // only fails on invalid options
	}
	g.faces[k] = f
	return f
}

// LineHeight returns the pixel height of a text line.
func (g *Gfx) LineHeight(size int) int32 {
	m := g.face(size, false).Metrics()
	return int32((m.Ascent + m.Descent).Ceil())
}

// Measure returns the pixel width of s.
func (g *Gfx) Measure(s string, size int, bold bool) int32 {
	return int32(font.MeasureString(g.face(size, bold), s).Ceil())
}

// Fit truncates s with an ellipsis so it fits in maxW pixels.
func (g *Gfx) Fit(s string, size int, bold bool, maxW int32) string {
	if g.Measure(s, size, bold) <= maxW {
		return s
	}
	r := []rune(s)
	lo, hi := 0, len(r)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if g.Measure(string(r[:mid])+"…", size, bold) <= maxW {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return string(r[:lo]) + "…"
}

// Text draws s with its top-left corner at x,y and returns its width.
func (g *Gfx) Text(s string, x, y int32, size int, bold bool, c Color) int32 {
	if s == "" {
		return 0
	}
	t := g.textTexture(s, size, bold)
	if t == nil {
		return 0
	}
	t.tex.SetColorMod(c.R, c.G, c.B)
	t.tex.SetAlphaMod(c.A)
	g.r.Copy(t.tex, nil, &sdl.Rect{X: x, Y: y, W: t.w, H: t.h})
	return t.w
}

// TextRight draws s right-aligned to x.
func (g *Gfx) TextRight(s string, x, y int32, size int, bold bool, c Color) {
	g.Text(s, x-g.Measure(s, size, bold), y, size, bold, c)
}

func (g *Gfx) textTexture(s string, size int, bold bool) *textTex {
	k := textKey{s: s, size: size, bold: bold}
	if t, ok := g.cache[k]; ok {
		t.lastUsed = g.frame
		return t
	}

	f := g.face(size, bold)
	m := f.Metrics()
	w := font.MeasureString(f, s).Ceil()
	h := (m.Ascent + m.Descent).Ceil()
	if w <= 0 || h <= 0 {
		return nil
	}
	mask := image.NewAlpha(image.Rect(0, 0, w, h))
	d := font.Drawer{Dst: mask, Src: image.Opaque, Face: f, Dot: fixed.Point26_6{Y: m.Ascent}}
	d.DrawString(s)

	// White pixels with glyph coverage as alpha; color comes from ColorMod.
	pix := make([]byte, w*h*4)
	for i, a := range mask.Pix {
		pix[i*4], pix[i*4+1], pix[i*4+2], pix[i*4+3] = 0xff, 0xff, 0xff, a
	}
	tex, err := g.r.CreateTexture(sdl.PIXELFORMAT_ABGR8888, sdl.TEXTUREACCESS_STATIC, int32(w), int32(h))
	if err != nil {
		return nil
	}
	tex.Update(nil, unsafe.Pointer(&pix[0]), w*4)
	tex.SetBlendMode(sdl.BLENDMODE_BLEND)
	t := &textTex{tex: tex, w: int32(w), h: int32(h), lastUsed: g.frame}
	g.cache[k] = t
	return t
}

// Image uploads an RGBA image as a texture. The caller owns the texture.
func (g *Gfx) Image(img *image.RGBA) (*sdl.Texture, error) {
	b := img.Bounds()
	tex, err := g.r.CreateTexture(sdl.PIXELFORMAT_ABGR8888, sdl.TEXTUREACCESS_STATIC, int32(b.Dx()), int32(b.Dy()))
	if err != nil {
		return nil, err
	}
	tex.Update(nil, unsafe.Pointer(&img.Pix[0]), img.Stride)
	tex.SetBlendMode(sdl.BLENDMODE_BLEND)
	return tex, nil
}

// Fill draws a solid rectangle.
func (g *Gfx) Fill(x, y, w, h int32, c Color) {
	g.r.SetDrawColor(c.R, c.G, c.B, c.A)
	g.r.FillRect(&sdl.Rect{X: x, Y: y, W: w, H: h})
}

// Clear starts a new frame. It returns false while the window has no size yet.
func (g *Gfx) Clear() bool {
	if !g.syncSize() {
		return false
	}
	g.frame++
	g.r.SetDrawBlendMode(sdl.BLENDMODE_BLEND)
	g.r.SetDrawColor(colBg.R, colBg.G, colBg.B, colBg.A)
	g.r.Clear()
	return true
}

// Present shows the frame and evicts text textures unused for a while.
func (g *Gfx) Present() {
	g.r.Present()
	if len(g.cache) > 400 {
		for k, t := range g.cache {
			if g.frame-t.lastUsed > 2 {
				t.tex.Destroy()
				delete(g.cache, k)
			}
		}
	}
}

func (g *Gfx) destroy() {
	for _, t := range g.cache {
		t.tex.Destroy()
	}
	for _, f := range g.faces {
		f.Close()
	}
}
