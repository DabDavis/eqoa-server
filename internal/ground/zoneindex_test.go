package ground

import (
	"math/rand"
	"os"
	"slices"
	"testing"
)

// The index gives, for any box, every zone whose bounds meet it, each once:
// against brute force over random zones (overlapping, touching, far apart)
// and random boxes, the points and edges among them.
func TestZoneIndexGivesEveryZoneABoxMeets(t *testing.T) {
	rnd := rand.New(rand.NewSource(3))
	type box struct{ x0, z0, x1, z1 float32 }
	zones := make([]box, 300)
	for i := range zones {
		x, z := rnd.Float32()*20000-10000, rnd.Float32()*20000-10000
		zones[i] = box{x, z, x + rnd.Float32()*2500, z + rnd.Float32()*2500}
	}
	empty := map[int]bool{7: true, 99: true} // zones with nothing in them are left out
	ix := newZoneIndex(len(zones), func(i int) (float32, float32, float32, float32, bool) {
		b := zones[i]
		return b.x0, b.z0, b.x1, b.z1, !empty[i]
	})
	for q := 0; q < 20000; q++ {
		x, z := rnd.Float32()*24000-12000, rnd.Float32()*24000-12000
		w, d := float32(0), float32(0)
		switch q % 4 {
		case 1:
			w, d = rnd.Float32()*3, rnd.Float32()*3 // a step or a leg's piece
		case 2:
			w, d = rnd.Float32()*600, rnd.Float32()*600
		case 3: // a corner of a zone, exactly
			b := zones[rnd.Intn(len(zones))]
			x, z = b.x1, b.z0
		}
		got := ix.near(x, z, x+w, z+d)
		seen := map[int32]bool{}
		for _, i := range got {
			if seen[i] {
				t.Fatalf("query %d: zone %d given twice", q, i)
			}
			seen[i] = true
		}
		for i, b := range zones {
			meets := !(x+w < b.x0 || x > b.x1 || z+d < b.z0 || z > b.z1)
			if meets && !empty[i] && !seen[int32(i)] {
				t.Fatalf("query %d (%v,%v)-(%v,%v): zone %d %+v meets it and was not given", q, x, z, x+w, z+d, i, b)
			}
		}
	}
	if got := newZoneIndex(0, nil).near(1, 2, 3, 4); got != nil {
		t.Errorf("an empty world's index gave %v", got)
	}
}

// everyZone is an index that gives every zone of n: the walk over every zone
// the index replaced, as a test's reference.
func everyZone(n int) *zoneIndex {
	all := make([]int32, n)
	for i := range all {
		all[i] = int32(i)
	}
	return &zoneIndex{nx: 1, nz: 1, cells: [][]int32{all}}
}

// On the real ground files (EQOA_GROUND_DIR, a worlds/ground folder), every
// query finds with the index what the walk over every zone found: floors,
// walls, what is placed, roofs and surfaces at random places in every world.
func TestRealZoneIndexFindsWhatTheWalkFound(t *testing.T) {
	dir := os.Getenv("EQOA_GROUND_DIR")
	if dir == "" {
		t.Skip("EQOA_GROUND_DIR names a server's worlds/ground folder")
	}
	g, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	rnd := rand.New(rand.NewSource(11))
	// both indexes on the same Ground, so the zones load once
	type pair struct{ terrain, walls [2]*zoneIndex }
	idx := map[uint8]*pair{}
	g.mu.Lock()
	for n, w := range g.worlds {
		p := &pair{}
		p.terrain = [2]*zoneIndex{w.terrainIndexLocked(), everyZone(len(w.entries))}
		if ww := g.walls[n]; ww != nil {
			p.walls = [2]*zoneIndex{ww.wallsIndexLocked(), everyZone(len(ww.zones))}
		}
		idx[n] = p
	}
	g.mu.Unlock()
	use := func(n uint8, k int) {
		g.mu.Lock()
		g.worlds[n].index = idx[n].terrain[k]
		if ww := g.walls[n]; ww != nil {
			ww.index = idx[n].walls[k]
		}
		g.mu.Unlock()
	}
	queries := 0
	for n, w := range g.worlds {
		for q := 0; q < 3000; q++ {
			e := w.entries[rnd.Intn(len(w.entries))]
			if e.verts == 0 {
				continue
			}
			x := e.minX + rnd.Float32()*(e.maxX-e.minX)
			z := e.minZ + rnd.Float32()*(e.maxZ-e.minZ)
			// near the ground there, as the server's queries are: its highest
			// floor, a little over or under it
			y := e.baseY
			if top, ok := g.Floor(n, x, 1e6, z); ok {
				y = top + rnd.Float32()*4 - 1
			}
			dx, dz, dy := rnd.Float32()*6-3, rnd.Float32()*6-3, rnd.Float32()*4-2
			var floor [2]float32
			var floorOK, wall, placed, roof [2]bool
			var surf [2][]Surface
			for k := 0; k < 2; k++ {
				use(n, k)
				floor[k], floorOK[k] = g.Floor(n, x, y, z)
				wall[k] = g.Wall(n, x, y+1, z, x+dx, y+1+dy, z+dz)
				placed[k] = g.Placed(n, x, y, z, x+dx, y+dy, z+dz)
				roof[k] = g.Roof(n, x, y, y+200, z)
				surf[k] = g.Surfaces(n, x, z, y-40, y+40)
				slices.SortFunc(surf[k], func(a, b Surface) int {
					switch {
					case a.Y < b.Y:
						return -1
					case a.Y > b.Y:
						return 1
					}
					return 0
				})
			}
			if floor[0] != floor[1] || floorOK[0] != floorOK[1] || wall[0] != wall[1] || placed[0] != placed[1] ||
				roof[0] != roof[1] || !slices.Equal(surf[0], surf[1]) {
				t.Fatalf("world %d at (%v, %v, %v): index floor %v %v wall %v placed %v roof %v surfaces %v; every zone %v %v %v %v %v %v",
					n, x, y, z, floor[0], floorOK[0], wall[0], placed[0], roof[0], surf[0],
					floor[1], floorOK[1], wall[1], placed[1], roof[1], surf[1])
			}
			queries++
		}
	}
	t.Logf("%d places in %d worlds asked both ways, the same answers", queries, len(g.worlds))
}
