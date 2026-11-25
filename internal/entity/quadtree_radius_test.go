package entity

import (
	"slices"
	"testing"
)

// A radius query inherits Query's traversal order and its half-open bounds.
// Channel assignment can observe this order when entities are equally far away.
func TestRadiusQueryKeepsOrderedMatches(t *testing.T) {
	tree := NewQuadTree(Bounds{X: -128, Y: -128, Width: 256, Height: 256})
	points := make(map[EntityID][2]float32)
	add := func(x, y float32) {
		id := EntityID(len(points) + 1)
		points[id] = [2]float32{x, y}
		tree.Insert(id, x, y)
	}
	for x := -112; x < 120; x += 8 {
		for y := -112; y < 120; y += 8 {
			add(float32(x), float32(y))
		}
	}
	add(0, 0)
	add(3, 4)
	add(-3, -4)
	add(-5, 0)
	add(5, 0)
	for _, q := range [][3]float32{{0, 0, 5}, {0, 0, 0}, {0, 0, -5}, {-120, 24, 39}, {42, 200, 4}, {12, -7, 300}} {
		x, y, r := q[0], q[1], q[2]
		var want []EntityID
		for _, id := range tree.Query(Bounds{X: x - r, Y: y - r, Width: 2 * r, Height: 2 * r}) {
			p := points[id]
			dx, dy := p[0]-x, p[1]-y
			if dx*dx+dy*dy <= r*r {
				want = append(want, id)
			}
		}
		got := tree.QueryRadius(x, y, r)
		if !slices.Equal(got, want) || (got == nil) != (want == nil) {
			t.Fatalf("query %v: got %v, want %v", q, got, want)
		}
	}
}

// Repeated queries from 68 nearby positions in one populated area.
func BenchmarkCrowdedRadius(b *testing.B) {
	tree := NewQuadTree(Bounds{Width: 32768, Height: 32768})
	for i := 0; i < 3000; i++ {
		tree.Insert(EntityID(i+1), 25000+float32(i%64)*8, 15500+float32(i/64)*8)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for player := 0; player < 68; player++ {
			if len(tree.QueryRadius(25309+float32(player%12), 15765+float32(player/12), 175)) == 0 {
				b.Fatal("empty crowd")
			}
		}
	}
}
