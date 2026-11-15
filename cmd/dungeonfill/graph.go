package main

import (
	"math"

	"github.com/eqoa/server/internal/ground"
)

// The floors of a dungeon: every column of a grid (cell apart) over the
// dungeon's boxes holds the up-facing surfaces of the game's collision there
// (ground.Surfaces: the terrain's and what is placed on it) where a body
// fits over it (no surface from clearLo to clearHi above it: never inside a
// rock or a solid wall) and it is dry (no water, slime, lava or mud at
// envUp over it). Two floors of neighbouring columns join when the rise
// between them is a walk's (riseStep + the run: a slope to about 45° and a
// step) and no wall stands between them at bodyUp over each (ground.Wall):
// unlike the open world's land (wildspread's highest floor) a citadel's
// levels stand over one another, so every floor is kept.
const (
	scanLo    = -400.0
	scanHi    = 600.0
	clearLo   = 0.4
	clearHi   = 2.2
	riseStep  = 0.8
	bodyUp    = 1.0
	envUp     = 1.2
	roofReach = 60.0
	envBad    = ground.EnvWater | ground.EnvSlime | ground.EnvLava | ground.EnvMud
)

type box struct{ x0, z0, x1, z1 float64 }

func (b box) in(x, z float64) bool { return x >= b.x0 && x <= b.x1 && z >= b.z0 && z <= b.z1 }

// node is a floor of a column.
type node struct {
	x, z    float64
	y       float32
	terrain bool  // the terrain's own (a creature's walk stands on these: ground.Floor)
	roofed  bool  // a surface over it within roofReach (a tunnel's, a hall's): inside
	col     int32 // the column
	comp    int32 // its piece of joined floor
}

type graph struct {
	x0, z0, cell float64
	nx, nz       int
	first        []int32 // column c's nodes are first[c]..first[c+1]
	nodes        []node
	adj          [][]int32
}

func (g *graph) col(i, j int) int { return i*g.nz + j }

func (g *graph) colOf(x, z float64) (int, int, bool) {
	i := int(math.Floor((x - g.x0) / g.cell))
	j := int(math.Floor((z - g.z0) / g.cell))
	return i, j, i >= 0 && j >= 0 && i < g.nx && j < g.nz
}

// build samples the floors of world over the boxes' bounds (columns outside
// every box left empty) and joins them.
func build(gr *ground.Ground, env *ground.Env, world uint8, boxes []box, cell float64) *graph {
	x0, z0, x1, z1 := boxes[0].x0, boxes[0].z0, boxes[0].x1, boxes[0].z1
	for _, b := range boxes[1:] {
		x0, z0 = math.Min(x0, b.x0), math.Min(z0, b.z0)
		x1, z1 = math.Max(x1, b.x1), math.Max(z1, b.z1)
	}
	g := &graph{x0: x0, z0: z0, cell: cell, nx: int(math.Ceil((x1-x0)/cell)) + 1, nz: int(math.Ceil((z1-z0)/cell)) + 1}
	g.first = make([]int32, g.nx*g.nz+1)
	for i := 0; i < g.nx; i++ {
		for j := 0; j < g.nz; j++ {
			c := g.col(i, j)
			g.first[c] = int32(len(g.nodes))
			x, z := x0+(float64(i)+0.5)*cell, z0+(float64(j)+0.5)*cell
			inside := false
			for _, b := range boxes {
				inside = inside || b.in(x, z)
			}
			if !inside {
				continue
			}
			ss := gr.Surfaces(world, float32(x), float32(z), scanLo, scanHi)
			for _, s := range ss {
				if !s.Up {
					continue
				}
				open := true
				for _, o := range ss {
					if d := o.Y - s.Y; d > clearLo && d < clearHi {
						open = false
						break
					}
				}
				if !open {
					continue
				}
				if env != nil {
					if f, ok := env.Flags(world, float32(x), s.Y+envUp, float32(z)); ok && f&envBad != 0 {
						continue
					}
				}
				dup := false
				for k := g.first[c]; k < int32(len(g.nodes)); k++ {
					if math.Abs(float64(g.nodes[k].y-s.Y)) < 0.3 {
						dup = true
						if !s.Placed {
							g.nodes[k].terrain = true
						}
					}
				}
				if dup {
					continue
				}
				roofed := false
				for _, o := range ss {
					if d := o.Y - s.Y; d >= clearHi && d <= roofReach {
						roofed = true
						break
					}
				}
				terrain := false
				if !s.Placed {
					if f, ok := gr.Floor(world, float32(x), s.Y+0.5, float32(z)); ok && math.Abs(float64(f-s.Y)) < 0.3 {
						terrain = true
					}
				}
				g.nodes = append(g.nodes, node{x: x, z: z, y: s.Y, terrain: terrain, roofed: roofed, col: int32(c), comp: -1})
			}
		}
	}
	g.first[g.nx*g.nz] = int32(len(g.nodes))
	g.adj = make([][]int32, len(g.nodes))
	for i := 0; i < g.nx; i++ {
		for j := 0; j < g.nz; j++ {
			a := g.col(i, j)
			for _, d := range [][2]int{{1, 0}, {0, 1}, {1, 1}, {1, -1}} {
				ii, jj := i+d[0], j+d[1]
				if ii < 0 || jj < 0 || ii >= g.nx || jj >= g.nz {
					continue
				}
				b := g.col(ii, jj)
				run := cell * math.Hypot(float64(d[0]), float64(d[1]))
				for p := g.first[a]; p < g.first[a+1]; p++ {
					for q := g.first[b]; q < g.first[b+1]; q++ {
						np, nq := &g.nodes[p], &g.nodes[q]
						if math.Abs(float64(np.y-nq.y)) > riseStep+run {
							continue
						}
						if gr.Wall(world, float32(np.x), np.y+bodyUp, float32(np.z), float32(nq.x), nq.y+bodyUp, float32(nq.z)) {
							continue
						}
						g.adj[p] = append(g.adj[p], q)
						g.adj[q] = append(g.adj[q], p)
					}
				}
			}
		}
	}
	// pieces
	var n int32
	for s := range g.nodes {
		if g.nodes[s].comp >= 0 {
			continue
		}
		stack := []int32{int32(s)}
		g.nodes[s].comp = n
		for len(stack) > 0 {
			v := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for _, w := range g.adj[v] {
				if g.nodes[w].comp < 0 {
					g.nodes[w].comp = n
					stack = append(stack, w)
				}
			}
		}
		n++
	}
	return g
}

// nearest is the node nearest (x, y, z) within reach (its column's or a
// neighbour's floor, the height counting double), -1 for none.
func (g *graph) nearest(x, y, z, reach float64) int32 {
	i, j, ok := g.colOf(x, z)
	if !ok {
		return -1
	}
	best, bd := int32(-1), reach
	r := int(math.Ceil(reach / g.cell))
	for ii := i - r; ii <= i+r; ii++ {
		for jj := j - r; jj <= j+r; jj++ {
			if ii < 0 || jj < 0 || ii >= g.nx || jj >= g.nz {
				continue
			}
			c := g.col(ii, jj)
			for k := g.first[c]; k < g.first[c+1]; k++ {
				n := &g.nodes[k]
				d := math.Sqrt((n.x-x)*(n.x-x) + (n.z-z)*(n.z-z) + 4*float64(n.y-float32(y))*float64(n.y-float32(y)))
				if d < bd {
					best, bd = k, d
				}
			}
		}
	}
	return best
}

// insideOf marks the dungeon's floor: the kept pieces' floor under insideY,
// and over it only where roofed (a hall's, not the hillside's), each piece
// of that joined floor (joins between such floors alone) at least minPiece
// floors big: a tree's canopy roofs a few columns of the open hill.
func (g *graph) insideOf(keep map[int32]bool, insideY float32, minPiece int) []bool {
	in := make([]bool, len(g.nodes))
	for k, n := range g.nodes {
		in[k] = keep[n.comp] && (n.y < insideY || n.roofed)
	}
	seen := make([]bool, len(g.nodes))
	for s := range g.nodes {
		if !in[s] || seen[s] {
			continue
		}
		piece := []int32{int32(s)}
		seen[s] = true
		for i := 0; i < len(piece); i++ {
			for _, w := range g.adj[piece[i]] {
				if in[w] && !seen[w] {
					seen[w] = true
					piece = append(piece, w)
				}
			}
		}
		if len(piece) < minPiece {
			for _, k := range piece {
				in[k] = false
			}
		}
	}
	return in
}
