package ui

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"io"
	"net/http"
	"strings"

	"github.com/veandco/go-sdl2/sdl"

	"github.com/meerhelm/jubilant-potato/internal/catalog"
	"github.com/meerhelm/jubilant-potato/internal/source"
)

// maxImages caps how many pictures of a game are fetched.
const maxImages = 8

// gameInfoScreen shows a game's description and pictures before it is
// downloaded, for sources that can describe games (PICO-8 BBS).
type gameInfoScreen struct {
	a     *app
	games *gamesScreen
	group catalog.Group
	t     *task[*source.Details]

	imgs []*task[*image.RGBA]
	texs []*sdl.Texture // created from imgs on the render thread
	cur  int            // shown image

	scroll    int // first visible description line
	maxScroll int
	wrapW     int32
	lines     []string // description wrapped to wrapW
}

func newGameInfoScreen(a *app, games *gamesScreen, group catalog.Group) *gameInfoScreen {
	d := games.src.(source.Describer)
	g := group.Best().Game
	return &gameInfoScreen{a: a, games: games, group: group,
		t: startTask(func() (*source.Details, error) { return d.Details(context.Background(), g) })}
}

func (s *gameInfoScreen) Title() string { return s.group.Title }

func (s *gameInfoScreen) Hints() []Hint {
	h := []Hint{{"A", T("Download")}}
	if len(s.imgs) > 1 {
		h = append(h, Hint{"←→", T("Images")})
	}
	if s.maxScroll > 0 {
		h = append(h, Hint{"↑↓", T("Scroll")})
	}
	if len(s.group.Variants) > 1 {
		h = append(h, Hint{"X", T("Versions")})
	}
	return append(h, Hint{"B", T("Back")})
}

func (s *gameInfoScreen) Update() bool {
	changed := s.games.Update()
	if s.t.Poll() {
		changed = true
		if s.t.err == nil {
			client := s.a.opts.HTTPClient
			for _, u := range s.t.val.Images[:min(len(s.t.val.Images), maxImages)] {
				s.imgs = append(s.imgs, startTask(func() (*image.RGBA, error) { return fetchImage(client, u) }))
			}
			s.texs = make([]*sdl.Texture, len(s.imgs))
		}
	}
	for _, t := range s.imgs {
		if t.Poll() {
			changed = true
		}
	}
	return changed
}

func (s *gameInfoScreen) Handle(act Action) {
	switch act {
	case Left:
		s.showImage(-1)
	case Right:
		s.showImage(1)
	case Up:
		s.scroll = max(s.scroll-3, 0)
	case Down:
		s.scroll = min(s.scroll+3, s.maxScroll)
	case A:
		s.games.download(s.group.Variants)
	case X:
		if len(s.group.Variants) > 1 {
			s.a.push(newVariantsScreen(s.a, s.games, s.group))
		}
	case Select:
		s.a.push(newDownloadsScreen(s.a))
	case B:
		s.a.pop()
	}
}

// showImage steps through the pictures, skipping ones that failed to load.
func (s *gameInfoScreen) showImage(d int) {
	n := len(s.imgs)
	for i := 1; i < n; i++ {
		j := ((s.cur+d*i)%n + n) % n
		if t := s.imgs[j]; !t.done || t.err == nil {
			s.cur = j
			return
		}
	}
}

// Close frees the image textures when the screen is popped.
func (s *gameInfoScreen) Close() {
	for _, t := range s.texs {
		if t != nil {
			t.Destroy()
		}
	}
}

func (s *gameInfoScreen) Draw(g *Gfx, area sdl.Rect) {
	pad := g.S(16)
	area.Y += pad / 2
	area.H -= pad / 2

	// Left: the current picture in a square box, with its number below.
	dotsH := g.S(24)
	box := min(area.H-dotsH-pad, area.W*9/20)
	bx, by := area.X+pad, area.Y
	g.Fill(bx, by, box, box, colBar)
	s.drawImage(g, sdl.Rect{X: bx, Y: by, W: box, H: box})
	if n := len(s.imgs); n > 1 {
		label := fmt.Sprintf("‹  %d / %d  ›", s.cur+1, n)
		g.Text(label, bx+(box-g.Measure(label, SizeSmall, false))/2, by+box+(dotsH-g.LineHeight(SizeSmall))/2+g.S(4), SizeSmall, false, colDim)
	}

	// Right: author, status, tags and the scrolling description.
	x := bx + box + pad
	w := area.X + area.W - x - pad
	y := area.Y
	switch installed, pending := s.games.status(s.group.Variants); {
	case installed:
		g.Text(T("Installed"), x, y, SizeSmall, true, colOK)
		y += g.LineHeight(SizeSmall) + g.S(4)
	case pending:
		g.Text(T("In queue"), x, y, SizeSmall, true, colAccent)
		y += g.LineHeight(SizeSmall) + g.S(4)
	}
	switch {
	case !s.t.done:
		g.Text(T("Loading description…"), x, y, SizeNormal, false, colDim)
		return
	case s.t.err != nil:
		for _, l := range wrapText(g, T("Error: %s", s.t.err.Error()), SizeSmall, w) {
			g.Text(l, x, y, SizeSmall, false, colErr)
			y += g.LineHeight(SizeSmall)
		}
		return
	}
	d := s.t.val
	if d.Author != "" {
		g.Text(g.Fit(T("by %s", d.Author), SizeNormal, true, w), x, y, SizeNormal, true, colText)
		y += g.LineHeight(SizeNormal) + g.S(4)
	}
	if len(d.Tags) > 0 {
		tags := "#" + strings.Join(d.Tags, "  #")
		g.Text(g.Fit(tags, SizeSmall, false, w), x, y, SizeSmall, false, colAccent)
		y += g.LineHeight(SizeSmall)
	}
	y += g.S(8)

	if s.wrapW != w {
		desc := d.Description
		if desc == "" {
			desc = T("No description")
		}
		s.lines, s.wrapW = wrapText(g, desc, SizeSmall, w), w
	}
	lh := g.LineHeight(SizeSmall) + g.S(2)
	bottom := area.Y + area.H - pad/2
	visible := max(int((bottom-y)/lh), 1)
	s.maxScroll = max(len(s.lines)-visible, 0)
	s.scroll = min(s.scroll, s.maxScroll)
	top := y
	for i := s.scroll; i < len(s.lines) && i < s.scroll+visible; i++ {
		g.Text(s.lines[i], x, y, SizeSmall, false, colText)
		y += lh
	}
	if s.maxScroll > 0 {
		trackH := int32(visible) * lh
		thumbH := max(trackH*int32(visible)/int32(len(s.lines)), g.S(16))
		thumbY := top + (trackH-thumbH)*int32(s.scroll)/int32(s.maxScroll)
		g.Fill(x+w+pad/2, thumbY, g.S(4), thumbH, colDim)
	}
}

// drawImage fits the current picture into r, at a whole multiple of its
// size when it is small so pixel art stays crisp.
func (s *gameInfoScreen) drawImage(g *Gfx, r sdl.Rect) {
	if s.cur >= len(s.imgs) {
		if s.t.done {
			drawCentered(g, r, colDim, "PICO-8")
		}
		return
	}
	t := s.imgs[s.cur]
	switch {
	case !t.done:
		drawCentered(g, r, colDim, "…")
		return
	case t.err != nil:
		drawCentered(g, r, colDim, "×")
		return
	}
	if s.texs[s.cur] == nil {
		tex, err := g.Image(t.val)
		if err != nil {
			return
		}
		s.texs[s.cur] = tex
	}
	b := t.val.Bounds()
	iw, ih := float64(b.Dx()), float64(b.Dy())
	scale := min(float64(r.W)/iw, float64(r.H)/ih)
	if scale >= 1 {
		scale = float64(int(scale))
	}
	w, h := int32(iw*scale), int32(ih*scale)
	g.r.Copy(s.texs[s.cur], nil, &sdl.Rect{X: r.X + (r.W-w)/2, Y: r.Y + (r.H-h)/2, W: w, H: h})
}

// fetchImage downloads and decodes a PNG, GIF (first frame) or JPEG.
func fetchImage(client *http.Client, url string) (*image.RGBA, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", source.UserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if cfg.Width*cfg.Height > 2048*2048 {
		return nil, fmt.Errorf("image too large: %dx%d", cfg.Width, cfg.Height)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	rgba := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(rgba, rgba.Bounds(), img, b.Min, draw.Src)
	return rgba, nil
}

// wrapText breaks s into lines no wider than maxW, keeping its line breaks.
func wrapText(g *Gfx, s string, size int, maxW int32) []string {
	var out []string
	for _, para := range strings.Split(s, "\n") {
		line := ""
		for _, word := range strings.Fields(para) {
			try := word
			if line != "" {
				try = line + " " + word
			}
			if g.Measure(try, size, false) <= maxW {
				line = try
				continue
			}
			if line != "" {
				out = append(out, line)
			}
			// Split words wider than the line.
			for g.Measure(word, size, false) > maxW {
				r := []rune(word)
				n := len(r) - 1
				for n > 1 && g.Measure(string(r[:n]), size, false) > maxW {
					n--
				}
				out = append(out, string(r[:n]))
				word = string(r[n:])
			}
			line = word
		}
		out = append(out, line)
	}
	return out
}
