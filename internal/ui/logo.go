package ui

import (
	"image"
	"image/color"
	"math"

	xdraw "golang.org/x/image/draw"
)

// potatoLogo renders the app mascot, a cheerful potato, as a size×size
// image. It is drawn procedurally at 4× and downscaled for smooth edges.
func potatoLogo(size int) *image.RGBA {
	const ss = 4
	n := size * ss
	big := image.NewRGBA(image.Rect(0, 0, n, n))

	var (
		outline = color.RGBA{0x7a, 0x4b, 0x1a, 0xff}
		skin    = color.RGBA{0xe0, 0xab, 0x4c, 0xff}
		light   = color.RGBA{0xf0, 0xc8, 0x72, 0xff}
		spot    = color.RGBA{0xb3, 0x7a, 0x2c, 0xff}
		ink     = color.RGBA{0x2a, 0x1a, 0x0c, 0xff}
		cheek   = color.RGBA{0xf2, 0x86, 0x6b, 0xff}
		shine   = color.RGBA{0xff, 0xff, 0xff, 0xff}
	)

	// Body: two overlapping tilted ellipses make a lumpy potato.
	body := func(x, y, grow float64) bool {
		return ellipse(x, y, 0.50, 0.54, 0.40+grow, 0.31+grow, -0.30) ||
			ellipse(x, y, 0.40, 0.46, 0.25+grow, 0.23+grow, 0.2)
	}
	eye := func(x, y, cx, cy float64) bool { return ellipse(x, y, cx, cy, 0.035, 0.05, 0) }

	for py := 0; py < n; py++ {
		for px := 0; px < n; px++ {
			x, y := (float64(px)+0.5)/float64(n), (float64(py)+0.5)/float64(n)
			var c color.RGBA
			switch {
			case !body(x, y, 0.025):
				continue
			case !body(x, y, 0):
				c = outline
			// Face.
			case eye(x, y, 0.42, 0.49) || eye(x, y, 0.58, 0.46):
				c = ink
				if ellipse(x, y, 0.428, 0.475, 0.012, 0.015, 0) || ellipse(x, y, 0.588, 0.445, 0.012, 0.015, 0) {
					c = shine
				}
			case smile(x, y):
				c = ink
			case ellipse(x, y, 0.35, 0.58, 0.05, 0.03, 0) || ellipse(x, y, 0.67, 0.54, 0.05, 0.03, 0):
				c = cheek
			// Potato "eyes" and a highlight for volume.
			case ellipse(x, y, 0.26, 0.40, 0.025, 0.018, 0.5) ||
				ellipse(x, y, 0.72, 0.70, 0.03, 0.02, -0.4) ||
				ellipse(x, y, 0.47, 0.76, 0.022, 0.016, 0.1) ||
				ellipse(x, y, 0.80, 0.45, 0.02, 0.015, 0.3):
				c = spot
			case ellipse(x, y, 0.33, 0.36, 0.09, 0.05, -0.5):
				c = light
			default:
				c = skin
			}
			big.SetRGBA(px, py, c)
		}
	}

	out := image.NewRGBA(image.Rect(0, 0, size, size))
	xdraw.CatmullRom.Scale(out, out.Bounds(), big, big.Bounds(), xdraw.Src, nil)
	return out
}

// ellipse reports whether (x, y) lies inside an ellipse centred at (cx, cy)
// with radii rx, ry, rotated by rot radians.
func ellipse(x, y, cx, cy, rx, ry, rot float64) bool {
	dx, dy := x-cx, y-cy
	s, c := math.Sincos(rot)
	u, v := dx*c+dy*s, -dx*s+dy*c
	return (u*u)/(rx*rx)+(v*v)/(ry*ry) <= 1
}

// smile is a thick arc under the eyes.
func smile(x, y float64) bool {
	const cx, cy, r, w = 0.505, 0.50, 0.12, 0.022
	dx, dy := x-cx, y-cy
	d := math.Hypot(dx, dy)
	return dy > 0.045 && d > r-w && d < r+w && math.Abs(dx) < 0.1
}
