package entity

import (
	"math"
	"sort"
	"testing"
	"time"
)

// TestRealSlimeFish is the Plane of Disease's creatures of the water on the
// real ground (EQOA_GROUND_DIR, world 5's files) under the real creatures
// (EQOA_REAL_DB), with the slime's surface rule (water.go) and without it
// (the control): where they spawn (on the surface, up a bank, over the
// slime), where their wander legs end, and where a chase after a player up
// the nearest bank leaves them. Skipped without both.
func TestRealSlimeFish(t *testing.T) {
	defer func() { slimeSurfaces = true }() // a skip leaves by Goexit
	for _, on := range []bool{false, true} {
		slimeSurfaces = on
		realSlimeFish(t, on)
	}
}

func realSlimeFish(t *testing.T, on bool) {
	w, _, done := landWaterWorld(t)
	defer done()
	w.mu.Lock()
	defer w.mu.Unlock()
	var fish []*NPC
	for _, npc := range w.NPCs {
		if npc.World == 5 && npc.Owner == 0 && w.keepsToLocked(npc) == keepsToWater &&
			w.slimeOnlyLocked(5, npc.SpawnX, npc.SpawnY+bodyAbove, npc.SpawnZ) {
			fish = append(fish, npc)
		}
	}
	sort.Slice(fish, func(i, j int) bool { return fish[i].ServerID < fish[j].ServerID })
	// the surface under each: the lowest floor in the slime round it
	surface := make(map[*NPC]float32)
	for _, f := range fish {
		y, _ := w.floorLocked(5, f.SpawnX, f.SpawnY, f.SpawnZ)
		surface[f] = w.slimeSurfaceLocked(5, f.SpawnX, y, f.SpawnZ)
	}
	off := func(f *NPC, x, y, z float32) bool { // off the slime's surface: up a bank, over it, out of it
		fl, ok := w.floorLocked(5, x, y, z)
		return !ok || y-surface[f] > 1 || fl-surface[f] > 1 || !w.slimeOnlyLocked(5, x, fl+bodyAbove, z)
	}
	var onSurface, up, over int
	for _, f := range fish {
		w.spawnOnGroundLocked(f)
		fl, _ := w.floorLocked(5, f.X, f.Y, f.Z)
		switch {
		case f.Y-fl > 1:
			over++
		case fl-surface[f] > 1:
			up++
		default:
			onSurface++
		}
	}
	// wander legs
	home := func(f *NPC) bool { return absf(f.SpawnY-surface[f]) <= slimeLevel } // spawned on the surface
	var legsAll, legsOff int
	for _, f := range fish {
		if !w.wanders(f) || !home(f) {
			continue
		}
		for i := 0; i < 20; i++ {
			x, z := w.roamPointLocked(f)
			if x == f.X && z == f.Z {
				continue
			}
			legsAll++
			fl, _ := w.floorLocked(5, x, f.Y, z)
			if off(f, x, fl, z) {
				legsOff++
			}
		}
	}
	// chases after a player up the nearest bank in the slime's leaf
	var chases, chaseOff int
	now := time.Now()
	for _, f := range fish {
		if !home(f) {
			continue // up a bank or over the slime: the data's to fix
		}
		bx, by, bz, found := float32(0), float32(0), float32(0), false
		for r := float32(4); r <= 60 && !found; r += 4 {
			for a := 0; a < 16; a++ {
				s, c := math.Sincos(float64(a) * math.Pi / 8)
				x, z := f.X+r*float32(c), f.Z+r*float32(s)
				fl, ok := w.floorLocked(5, x, f.Y+30, z)
				if ok && fl-surface[f] > 4 && w.slimeOnlyLocked(5, x, fl+bodyAbove, z) {
					bx, by, bz, found = x, fl, z, true
					break
				}
			}
		}
		if !found {
			continue
		}
		x0, y0, z0 := f.X, f.Y, f.Z
		f.chase.on, f.chase.fromX, f.chase.fromY, f.chase.fromZ = true, f.X, f.Y, f.Z
		t0 := now
		for i := 0; i < 100; i++ {
			t0 = t0.Add(100 * time.Millisecond)
			f.chase.last = t0.Add(-100 * time.Millisecond)
			w.chaseLocked(f, bx, by, bz, 0, 0, t0)
		}
		chases++
		if off(f, f.X, f.Y, f.Z) {
			chaseOff++
			fl, _ := w.floorLocked(5, f.X, f.Y, f.Z)
			t.Logf("  off: %d %s from (%.1f, %.1f, %.1f) to (%.1f, %.1f, %.1f) floor %.2f surface %.2f slime %v; its enemy at (%.1f, %.1f, %.1f)",
				f.ServerID, f.Name, x0, y0, z0, f.X, f.Y, f.Z, fl, surface[f], w.slimeOnlyLocked(5, f.X, fl+bodyAbove, f.Z), bx, by, bz)
		}
		f.X, f.Y, f.Z = x0, y0, z0
		f.chase = chaseState{}
		f.stopMoving()
	}
	t.Logf("slime surface rule %v: %d creatures of the water in world 5's slime: %d on its surface, %d up a bank, %d over it; "+
		"of those on it, wander legs %d, %d ending off its surface; chases up a bank %d, %d ending off its surface",
		on, len(fish), onSurface, up, over, legsAll, legsOff, chases, chaseOff)
	if on && (legsOff != 0 || chaseOff != 0 || over != 0) {
		t.Errorf("with the rule: %d legs and %d chases off the slime's surface, %d over it; want none", legsOff, chaseOff, over)
	}
}
