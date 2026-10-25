package entity

import "math"

// Towns, for the wander to keep out of (F3 20261025-164404: wildlife
// "should generally not path near or into cities. They go close to the
// gates but not in"). The server has no map of its cities, so it finds them
// in its own NPCs, as tools/seed_faction_hits.py finds a town's people:
// townsfolk are the NPCs with a body of a players' race (charactermodel,
// the ones that stand and do not wander), and a town is a chain of at least
// townMinFolk of them each within townLink of the next, in one world. On
// eqoaclassic's NPCs that is 134 towns, Freeport's 228 people the largest;
// a camp of human bandits counts as one too (a settlement either way).
//
// A point is in a town within townMargin of one of its people: the town's
// own ground, its gates' guards and a margin round them, however its people
// stand (a long road's knights make a long, thin town, not a box over the
// fields beside it).

const (
	townLink    float32 = 60
	townMinFolk         = 10
	townMargin  float32 = 40
)

// townCell is the town grid's cell (larger than the margin, so a point's
// own cell and its eight neighbours hold every member within the margin).
const townCell float32 = 64

type townKey struct {
	world  uint8
	cx, cz int32
}

// townGrid is every town's people by world and cell.
type townGrid map[townKey][][2]float32

func townCellOf(world uint8, x, z float32) townKey {
	return townKey{world, int32(math.Floor(float64(x / townCell))), int32(math.Floor(float64(z / townCell)))}
}

// buildTownsLocked finds the towns among the NPCs (the file's doc). Must be
// called with the lock held.
func (w *World) buildTownsLocked() {
	type folk struct {
		world uint8
		x, z  float32
	}
	var people []folk
	for _, npc := range w.NPCs {
		if npc.ModelID != 0 && w.playerModels[uint32(npc.ModelID)] && npc.Owner == 0 {
			people = append(people, folk{npc.World, npc.SpawnX, npc.SpawnZ})
		}
	}
	// chains: union-find over a grid of townLink cells
	parent := make([]int, len(people))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	cell := func(f folk) townKey {
		return townKey{f.world, int32(math.Floor(float64(f.x / townLink))), int32(math.Floor(float64(f.z / townLink)))}
	}
	byCell := make(map[townKey][]int)
	for i, f := range people {
		byCell[cell(f)] = append(byCell[cell(f)], i)
	}
	for i, f := range people {
		c := cell(f)
		for dx := int32(-1); dx <= 1; dx++ {
			for dz := int32(-1); dz <= 1; dz++ {
				for _, j := range byCell[townKey{c.world, c.cx + dx, c.cz + dz}] {
					if j > i && distXZ(f.x, f.z, people[j].x, people[j].z) <= townLink {
						if a, b := find(i), find(j); a != b {
							parent[a] = b
						}
					}
				}
			}
		}
	}
	size := make(map[int]int)
	for i := range people {
		size[find(i)]++
	}
	g := make(townGrid)
	n := 0
	for i, f := range people {
		if size[find(i)] >= townMinFolk {
			k := townCellOf(f.world, f.x, f.z)
			g[k] = append(g[k], [2]float32{f.x, f.z})
			n++
		}
	}
	w.towns = g
	w.townFolk = n
}

// inTownLocked reports a point within townMargin of a town's people. Must
// be called with the lock held.
func (w *World) inTownLocked(world uint8, x, z float32) bool {
	if w.towns == nil {
		w.buildTownsLocked()
	}
	c := townCellOf(world, x, z)
	for dx := int32(-1); dx <= 1; dx++ {
		for dz := int32(-1); dz <= 1; dz++ {
			for _, p := range w.towns[townKey{world, c.cx + dx, c.cz + dz}] {
				if distXZ(x, z, p[0], p[1]) <= townMargin {
					return true
				}
			}
		}
	}
	return false
}

// legInTownLocked reports a walk from (x0, z0) to (x1, z1) that ends in a
// town or passes through one: one of a town's people within townMargin of
// the line walked. Must be called with the lock held.
func (w *World) legInTownLocked(world uint8, x0, z0, x1, z1 float32) bool {
	if w.towns == nil {
		w.buildTownsLocked()
	}
	lo := townCellOf(world, min(x0, x1)-townMargin, min(z0, z1)-townMargin)
	hi := townCellOf(world, max(x0, x1)+townMargin, max(z0, z1)+townMargin)
	for cx := lo.cx; cx <= hi.cx; cx++ {
		for cz := lo.cz; cz <= hi.cz; cz++ {
			for _, p := range w.towns[townKey{world, cx, cz}] {
				if segmentDist(x0, z0, x1, z1, p[0], p[1]) <= townMargin {
					return true
				}
			}
		}
	}
	return false
}

// segmentDist is the distance on the ground from (px, pz) to the line from
// (x0, z0) to (x1, z1).
func segmentDist(x0, z0, x1, z1, px, pz float32) float32 {
	dx, dz := x1-x0, z1-z0
	l2 := dx*dx + dz*dz
	t := float32(0)
	if l2 > 0 {
		t = max(0, min(1, ((px-x0)*dx+(pz-z0)*dz)/l2))
	}
	return distXZ(x0+dx*t, z0+dz*t, px, pz)
}
