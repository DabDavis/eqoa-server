package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

// mark is a creature drawn on a map.
type mark struct {
	x, z float64
	y    float32
	c    color.RGBA
}

// view is what a map shows: the dungeon's floor (inside) cropped to its
// extent, px pixels a unit.
type view struct {
	g      *graph
	keep   map[int32]bool
	in     []bool
	px     float64
	x0, z0 float64 // the crop's corner
	w, h   int
}

func (v *view) inside(k int) bool { return v.in[k] }

func newView(g *graph, keep map[int32]bool, in []bool, px float64) *view {
	v := &view{g: g, keep: keep, in: in, px: px}
	x0, z0, x1, z1 := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for k, n := range g.nodes {
		if v.inside(k) {
			x0, z0, x1, z1 = math.Min(x0, n.x), math.Min(z0, n.z), math.Max(x1, n.x), math.Max(z1, n.z)
		}
	}
	if math.IsInf(x0, 1) {
		x0, z0, x1, z1 = g.x0, g.z0, g.x0+float64(g.nx)*g.cell, g.z0+float64(g.nz)*g.cell
	}
	const margin = 12
	v.x0, v.z0 = x0-margin, z0-margin
	v.w, v.h = int((x1-x0+2*margin)*px), int((z1-z0+2*margin)*px)
	return v
}

// renderBand draws the floor from blo to bhi high seen from above: the
// dungeon's (inside) by height, low blue to high yellow, lighter where a
// column holds more than one of its floors; the rest of the reached floor
// (the open hillside) dark grey; the marks within a unit of the band as dots.
// North (smaller z) is up; a white scale bar is 20 units.
func (v *view) renderBand(path string, marks []mark, blo, bhi float32) error {
	g := v.g
	img := image.NewRGBA(image.Rect(0, 0, v.w, v.h))
	for i := range img.Pix {
		img.Pix[i] = 255
		if i%4 != 3 {
			img.Pix[i] = 20
		}
	}
	lo, hi := float32(math.Inf(1)), float32(math.Inf(-1))
	for k, n := range g.nodes {
		if v.inside(k) {
			lo, hi = min(lo, n.y), max(hi, n.y)
		}
	}
	cpx := int(math.Max(1, math.Ceil(g.cell*v.px)))
	for i := 0; i < g.nx; i++ {
		for j := 0; j < g.nz; j++ {
			c := g.col(i, j)
			var col color.RGBA
			levels, other := 0, false
			lowest := float32(math.Inf(1))
			for k := g.first[c]; k < g.first[c+1]; k++ {
				n := g.nodes[k]
				if n.y < blo || n.y >= bhi {
					continue
				}
				if v.inside(int(k)) {
					levels++
					lowest = min(lowest, n.y)
				} else if v.keep[n.comp] {
					other = true
				}
			}
			switch {
			case levels > 0:
				t := float64((lowest - lo) / max(hi-lo, 1))
				col = color.RGBA{uint8(40 + 215*t), uint8(60 + 180*t), uint8(200 * (1 - t)), 255}
				if levels > 1 {
					col.R, col.G, col.B = col.R/2+110, col.G/2+110, col.B/2+110
				}
			case other:
				col = color.RGBA{58, 58, 58, 255}
			default:
				continue
			}
			x0 := int((g.x0 + float64(i)*g.cell - v.x0) * v.px)
			y0 := int((g.z0 + float64(j)*g.cell - v.z0) * v.px)
			for a := 0; a < cpx; a++ {
				for b := 0; b < cpx; b++ {
					img.SetRGBA(x0+a, y0+b, col)
				}
			}
		}
	}
	for _, m := range marks {
		if m.y < blo-1 || m.y >= bhi+1 {
			continue
		}
		cx, cz := int((m.x-v.x0)*v.px), int((m.z-v.z0)*v.px)
		for a := -4; a <= 4; a++ {
			for b := -4; b <= 4; b++ {
				if a*a+b*b > 17 {
					continue
				}
				c := m.c
				if a*a+b*b > 9 {
					c = color.RGBA{0, 0, 0, 255}
				}
				img.SetRGBA(cx+a, cz+b, c)
			}
		}
	}
	white := color.RGBA{255, 255, 255, 255}
	for a := 0; a < int(20*v.px); a++ {
		for b := 0; b < 3; b++ {
			img.SetRGBA(8+a, v.h-12+b, white)
		}
	}
	drawLabel(img, 8, 8, bandLabel(blo, bhi), white)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func bandLabel(lo, hi float32) string {
	return "Y " + itoa(int(lo)) + "-" + itoa(int(hi))
}

func itoa(n int) string {
	if n < 0 {
		return "-" + itoa(-n)
	}
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}

// a 3x5 font for the band labels
var glyphs = map[rune][5]string{
	'0': {"111", "101", "101", "101", "111"}, '1': {"010", "110", "010", "010", "111"},
	'2': {"111", "001", "111", "100", "111"}, '3': {"111", "001", "111", "001", "111"},
	'4': {"101", "101", "111", "001", "001"}, '5': {"111", "100", "111", "001", "111"},
	'6': {"111", "100", "111", "101", "111"}, '7': {"111", "001", "010", "010", "010"},
	'8': {"111", "101", "111", "101", "111"}, '9': {"111", "101", "111", "001", "111"},
	'-': {"000", "000", "111", "000", "000"}, 'Y': {"101", "101", "010", "010", "010"},
	' ': {"000", "000", "000", "000", "000"},
}

func drawLabel(img *image.RGBA, x, y int, s string, c color.RGBA) {
	const scale = 4
	for _, r := range s {
		g := glyphs[r]
		for row := 0; row < 5; row++ {
			for col := 0; col < 3; col++ {
				if g[row][col] == '1' {
					for a := 0; a < scale; a++ {
						for b := 0; b < scale; b++ {
							img.SetRGBA(x+col*scale+a, y+row*scale+b, c)
						}
					}
				}
			}
		}
		x += 4 * scale
	}
}
