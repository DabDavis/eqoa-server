package entity

import (
	"math"
	"sort"
	"testing"
	"time"
)

// Petitions 2189/2190 on the real ground and creatures (EQOA_GROUND_DIR,
// EQOA_REAL_DB): every creature of the water round the Isle of Dread (world 0,
// x 23000-27000, z 1000-5000) chases an enemy standing 8 past the nearest dry
// shore within 200 of its spawn. None ends a tick out of the water, and each
// breaks off at the edge. Before the fix the rays, piranhas and Megaladon that
// swim off the sea's bed walked up it onto the beach (217-244 ticks of 300
// out of the water). Skipped without both.
func TestRealIsleOfDreadWaterCreaturesStayInTheWater(t *testing.T) {
	w, _, done := landWaterWorld(t)
	defer done()
	w.mu.Lock()
	defer w.mu.Unlock()
	var list []*NPC
	for _, n := range w.NPCs {
		if n.World == 0 && n.Owner == 0 && n.SpawnX > 23000 && n.SpawnX < 27000 && n.SpawnZ > 1000 && n.SpawnZ < 5000 &&
			w.keepsToLocked(n) == keepsToWater && w.spawnInWaterLocked(n) {
			list = append(list, n)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ServerID < list[j].ServerID })
	if len(list) == 0 {
		t.Fatal("no creature of the water round the Isle of Dread")
	}
	chased, offBed := 0, 0
	for _, n := range list {
		var lx, ly, lz float32
		found := false
		for r := float32(4); r <= 200 && !found; r += 4 {
			for a := 0; a < 32; a++ {
				s, c := math.Sincos(float64(a) * math.Pi / 16)
				px, pz := n.SpawnX+r*float32(c), n.SpawnZ+r*float32(s)
				if py, ok := w.ground.Floor(0, px, 2000, pz); ok && !w.wetAtLocked(0, px, py, pz) {
					lx, ly, lz, found = px, py, pz, true
					break
				}
			}
		}
		if !found {
			continue
		}
		d := distXZ(n.SpawnX, n.SpawnZ, lx, lz)
		ex, ez := lx+(lx-n.SpawnX)/d*8, lz+(lz-n.SpawnZ)/d*8
		ey, ok := w.ground.Floor(0, ex, ly+5, ez)
		if !ok || w.wetAtLocked(0, ex, ey, ez) {
			continue
		}
		chased++
		if !w.spawnOnGroundLocked(n) {
			offBed++
		}
		now := time.Unix(1_800_000_000, 0)
		n.X, n.Y, n.Z = n.SpawnX, n.SpawnY, n.SpawnZ
		n.chase = chaseState{}
		out, gaveUp := 0, false
		for i := 0; i < 400 && !gaveUp; i++ {
			now = now.Add(100 * time.Millisecond)
			w.TickCount++
			_, gaveUp = w.chaseLocked(n, ex, ey, ez, 0, 0, now)
			if !w.npcInWaterLocked(n) {
				out++
			}
		}
		if out > 0 || !gaveUp {
			t.Errorf("%s %d (spawn %.0f, %.1f, %.0f) to (%.0f, %.1f, %.0f): %d ticks out of the water, gave up %v, at (%.1f, %.1f, %.1f)",
				n.Name, n.ServerID, n.SpawnX, n.SpawnY, n.SpawnZ, ex, ey, ez, out, gaveUp, n.X, n.Y, n.Z)
		}
		n.X, n.Y, n.Z = n.SpawnX, n.SpawnY, n.SpawnZ
		n.chase = chaseState{}
	}
	t.Logf("%d creatures of the water round the island, %d chased to a shore (%d swimming off the bed)", len(list), chased, offBed)
}

// Live petition 5582 on the real ground and creatures (EQOA_GROUND_DIR, EQOA_REAL_DB): Croc Island by Blackwater
// (world 0, x 25550-25950, z 10850-11250), whose sea's water leaf runs on under the island's sand. Every creature of
// the water there swimming off the bed (the mako sharks, at the surface's 50.1) wanders 300 legs, each from the last's end, and
// chases an enemy on the island's sand (25750, 54.1, 11060): no leg ends in the ground, and the chase never takes it
// out of the water. Before the fix legs ended under the sand, where the ground lifted the sharks onto it (seven in
// the petition's view, at 52.8-54.1). Skipped without both.
func TestRealCrocIslandSharksStayInTheWater(t *testing.T) {
	w, _, done := landWaterWorld(t)
	defer done()
	var list []*NPC
	w.mu.Lock()
	for _, n := range w.NPCs {
		if n.World == 0 && n.Owner == 0 && n.SpawnX > 25550 && n.SpawnX < 25950 && n.SpawnZ > 10850 && n.SpawnZ < 11250 &&
			w.keepsToLocked(n) == keepsToWater && w.spawnInWaterLocked(n) && !w.spawnOnGroundLocked(n) {
			list = append(list, n)
		}
	}
	w.mu.Unlock()
	sort.Slice(list, func(i, j int) bool { return list[i].ServerID < list[j].ServerID })
	if len(list) < 20 {
		t.Fatalf("%d swimmers of the water round Croc Island, want the sharks (some thirty)", len(list))
	}
	under, total := 0, 0
	for _, n := range list {
		n.X, n.Y, n.Z = n.SpawnX, n.SpawnY, n.SpawnZ
		for i := 0; i < 300; i++ { // each leg from the last one's end, as it wanders
			ls := legs(w, n, 1)
			if len(ls) == 0 {
				continue
			}
			total++
			n.X, n.Z = ls[0][0], ls[0][1]
			w.mu.Lock()
			if w.inGroundLocked(0, n.X, n.Y, n.Z) || !w.waterAtLocked(0, n.X, n.Y+bodyAbove, n.Z) {
				under++
				n.X, n.Y, n.Z = n.SpawnX, n.SpawnY, n.SpawnZ
			}
			w.mu.Unlock()
		}
		n.X, n.Y, n.Z = n.SpawnX, n.SpawnY, n.SpawnZ
	}
	if under > 0 || total < 100*len(list)/2 {
		t.Errorf("%d of %d legs of %d swimmers end in the ground or out of the water", under, total, len(list))
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	ex, ey, ez := float32(25750), float32(54.1), float32(11060)
	for _, n := range list {
		now := time.Unix(1_800_000_000, 0)
		n.X, n.Y, n.Z = n.SpawnX, n.SpawnY, n.SpawnZ
		n.chase = chaseState{}
		out := 0
		for i := 0; i < 400; i++ {
			now = now.Add(100 * time.Millisecond)
			w.TickCount++
			if _, give := w.chaseLocked(n, ex, ey, ez, 0, 0, now); give {
				break
			}
			if !w.npcInWaterLocked(n) {
				out++
			}
		}
		if out > 0 {
			t.Errorf("%s %d (spawn %.0f, %.1f, %.0f): %d ticks out of the water chasing onto the sand, at (%.1f, %.1f, %.1f)",
				n.Name, n.ServerID, n.SpawnX, n.SpawnY, n.SpawnZ, out, n.X, n.Y, n.Z)
		}
		n.X, n.Y, n.Z = n.SpawnX, n.SpawnY, n.SpawnZ
		n.chase = chaseState{}
	}
	t.Logf("%d swimmers round Croc Island, %d legs", len(list), total)
}
