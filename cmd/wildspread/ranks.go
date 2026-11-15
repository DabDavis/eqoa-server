package main

import (
	"math"
	"sort"
)

// Ranks (-ranks; 2026-11-15, Kelinar: the owner, "kelinar needs its mobs
// spread out like we have in the past", and a player's petition, "a bunch of
// NPC lined up in Kelinar"). Return Home's spawn table (jib's export,
// tools/import_prh_content.py, 2026-10-21) put some places' creatures down
// as a parade: each kind a straight row of three or four, 4-6 units apart,
// every row facing one way, the kinds' rows side by side in one block
// (Kelinar's six blocks: gnoll reavers, gnoll shamans and their Companion
// Spirits, withered, centaur warders, werebears, lionweres, hill giants,
// spinebacks, griffennes). The server stands every creature that is not an
// animal and walks a roamer round its spawn, so the rows stand as they were
// written. A rank is a kind's creatures chained within rankLink of each
// other, rankMin or more, in a line (rankWidth across it at most, rankLength
// along it at least); a block is a rank and every creature chained to it
// within blockLink. A block's creatures are spread as a clump is (the
// wildlife's Lloyd spread) but within rankTerritory of its middle: the camp
// stays where it was, its creatures stand apart. A block creature still stays
// for every reason of the place (stays's second half: another tool's row, a
// path, a sign, a quest's, a faction's place, off the open ground, in the
// water, a town's, a zone left as it is).
const (
	rankLink      = 15.0  // a rank's creatures from each other, at most (Kelinar's withered stand 8-14 apart)
	rankMin       = 3     // creatures in a rank, at least
	rankWidth     = 1.5   // a rank's spread across its line, at most (RMS)
	rankLength    = 6.0   // a rank's length along its line, at least
	blockLink     = 20.0  // a block's creatures from each other, at most
	rankTerritory = 200.0 // how far from its block's middle a block's creature is spread
)

// findRanks is the rows (by id) standing in a block: every rank, and every
// creature chained to one within blockLink.
func findRanks(rows []*npcRow) map[int64]bool {
	var cs []*npcRow
	for _, r := range rows {
		if creature(r) && r.key != "" {
			cs = append(cs, r)
		}
	}
	sort.Slice(cs, func(a, b int) bool { return cs[a].id < cs[b].id })
	chains := func(link float64, same bool) [][]int {
		parent := make([]int, len(cs))
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
		ix := newIndex(link)
		for i, r := range cs {
			ix.add(r.x, r.z, i)
		}
		for i, r := range cs {
			ix.near(r.x, r.z, link, func(q pt, _ float64) {
				if q.v > i && (!same || cs[q.v].key == r.key) {
					if a, b := find(i), find(q.v); a != b {
						parent[a] = b
					}
				}
			})
		}
		groups := map[int][]int{}
		for i := range cs {
			groups[find(i)] = append(groups[find(i)], i)
		}
		out := make([][]int, 0, len(groups))
		for _, g := range groups {
			out = append(out, g)
		}
		return out
	}
	inRank := make([]bool, len(cs))
	for _, g := range chains(rankLink, true) {
		if len(g) < rankMin {
			continue
		}
		pts := make([][2]float64, len(g))
		for k, i := range g {
			pts[k] = [2]float64{cs[i].x, cs[i].z}
		}
		if length, width := lineShape(pts); width <= rankWidth && length >= rankLength {
			for _, i := range g {
				inRank[i] = true
			}
		}
	}
	out := map[int64]bool{}
	for _, g := range chains(blockLink, false) {
		ranked := false
		for _, i := range g {
			ranked = ranked || inRank[i]
		}
		if ranked {
			for _, i := range g {
				out[cs[i].id] = true
			}
		}
	}
	return out
}

// lineShape is how long points are along their main axis (the length a
// uniform row of them would have: 2*sqrt(3*variance)) and how wide across it
// (the RMS distance from that axis).
func lineShape(pts [][2]float64) (length, width float64) {
	n := float64(len(pts))
	var mx, mz float64
	for _, q := range pts {
		mx += q[0]
		mz += q[1]
	}
	mx /= n
	mz /= n
	var sxx, szz, sxz float64
	for _, q := range pts {
		dx, dz := q[0]-mx, q[1]-mz
		sxx += dx * dx
		szz += dz * dz
		sxz += dx * dz
	}
	tr, det := sxx+szz, sxx*szz-sxz*sxz
	disc := math.Sqrt(math.Max(tr*tr/4-det, 0))
	l1, l2 := tr/2+disc, math.Max(tr/2-disc, 0)
	return 2 * math.Sqrt(3*l1/n), math.Sqrt(l2 / n)
}
