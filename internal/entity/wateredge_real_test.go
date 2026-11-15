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
