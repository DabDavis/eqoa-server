package entity

import "math"

// Where a GM's #tele to an XP camp lands (handlers/camps.go): beside the
// camp, out of its creatures' notice, on the ground, rather than on the
// anchor creature's own spot in the middle of the camp, where every creature
// of it attacks on sight at once (a level 60 GM teleported into the Deathfist
// Citadel died in three seconds).
const (
	campAround = 120 // the camp: the creatures this near its anchor
	campClear  = 40  // no creature this near the landing (DefaultAggroRadius is 30)
	campRise   = 12  // the landing's floor within this of the anchor's
	campOpen   = 8   // no floor this far above the landing's: under the sky
	campHigh   = 60  // an anchor at most this above the terrain under it
	// with no spot campClear from every creature, one with this much room at
	// least (a single creature's notice, not the whole camp's)
	campCrowded = 15
)

// campSteps are the distances from the anchor a landing is looked for at,
// out past a citadel's walls (the Deathfist Citadel's creatures stand up to
// 115 from its anchor).
var campSteps = [...]float32{45, 60, 75, 90, 110, 130, 150}

// CampLanding is where a #tele to a camp lands: its anchor, the NPC of that
// name (sameNPCName) in the world whose spawn is nearest (x, z) within reach, and, when the
// anchor stands on the world's ground, a spot beside the camp: 45 to 150 off
// the anchor, no creature (an NPC without the NPC bit, alive, not a dummy)
// within campClear of it, on the terrain within campRise of the terrain at
// the anchor (under it, for a camp on a keep's floor or a tree's platforms,
// campHigh at most) with no floor campOpen above it (the open air, never a tunnel or a
// castle's hall the ground's floors cannot tell from the outside), out of the
// water, nearest first, the side away from the middle of the camp's creatures
// first; in a camp with no such spot, the open spot with the most room
// (campCrowded at least). A world with no ground, an anchor with no terrain
// under it, or a camp with no open spot (a cave's) lands on the anchor's
// spawn. ok is false when no NPC of that name is there.
func (w *World) CampLanding(name string, world uint8, x, z, reach float32) (lx, ly, lz float32, ok bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	var a *NPC
	best := reach * reach
	for _, npc := range w.NPCs {
		if !sameNPCName(npc.Name, name) || npc.World != world || npc.Temporary {
			continue
		}
		dx, dz := npc.SpawnX-x, npc.SpawnZ-z
		if d := dx*dx + dz*dz; d < best || (d == best && a != nil && npc.ServerID < a.ServerID) {
			best, a = d, npc
		}
	}
	if a == nil {
		return 0, 0, 0, false
	}
	lx, ly, lz, ok = a.SpawnX, a.SpawnY, a.SpawnZ, true
	if w.ground == nil {
		return
	}
	// the terrain the camp stands on, or under it: a camp on a keep's floor,
	// a tree's platforms or a dock lands on the terrain beside it
	floor, on := w.ground.Floor(world, a.SpawnX, a.SpawnY, a.SpawnZ)
	if !on || floor-a.SpawnY > spawnedBelow || a.SpawnY-floor > campHigh {
		return // no terrain under it within reach: the anchor's own spot
	}
	creature := func(npc *NPC) bool {
		return npc.World == world && !npc.Dead && npc.NPCType&0x80 == 0 && npc.Dummy == NotDummy
	}
	// the creatures that could stand near a landing (by where they stand or
	// spawned: a roamer comes back), and the camp's middle
	reachOut := campSteps[len(campSteps)-1] + campClear
	var local []*NPC
	var cx, cz float32
	n := 0
	for _, npc := range w.NPCs {
		if !creature(npc) {
			continue
		}
		if distXZ(npc.SpawnX, npc.SpawnZ, a.SpawnX, a.SpawnZ) <= reachOut || distXZ(npc.X, npc.Z, a.SpawnX, a.SpawnZ) <= reachOut {
			local = append(local, npc)
		}
		if distXZ(npc.SpawnX, npc.SpawnZ, a.SpawnX, a.SpawnZ) <= campAround {
			cx, cz, n = cx+npc.SpawnX, cz+npc.SpawnZ, n+1
		}
	}
	away := 0.0
	if n > 0 && distXZ(cx/float32(n), cz/float32(n), a.SpawnX, a.SpawnZ) > 1 {
		away = math.Atan2(float64(a.SpawnZ-cz/float32(n)), float64(a.SpawnX-cx/float32(n)))
	}
	// room is how far the nearest creature stands from a spot (by where it
	// stands or spawned), at most campClear
	room := func(px, pz float32) float32 {
		r := float32(campClear)
		for _, npc := range local {
			r = min(r, distXZ(npc.SpawnX, npc.SpawnZ, px, pz), distXZ(npc.X, npc.Z, px, pz))
		}
		return r
	}
	// with no spot clear of them all, the open spot with the most room
	// (campCrowded at least) is still better than the camp's middle
	var bx, by, bz, broom float32
	for _, d := range campSteps {
		for k := 0; k < 16; k++ {
			// 0, +1, -1, +2, -2 ... sixteenths of a turn from the side away
			step := float64((k+1)/2) * math.Pi / 8
			if k%2 == 0 {
				step = -step
			}
			h := away + step
			px := a.SpawnX + d*float32(math.Cos(h))
			pz := a.SpawnZ + d*float32(math.Sin(h))
			fy, ok := w.ground.Floor(world, px, floor+campRise, pz)
			if !ok || float32(math.Abs(float64(fy-floor))) > campRise {
				continue
			}
			if top, ok := w.ground.Floor(world, px, fy+1000, pz); ok && top > fy+campOpen {
				continue // under a roof, an overhang, another floor
			}
			if w.waterAtLocked(world, px, fy+bodyAbove, pz) {
				continue
			}
			r := room(px, pz)
			if r >= campClear {
				return px, fy, pz, true
			}
			if r > broom {
				bx, by, bz, broom = px, fy, pz, r
			}
		}
	}
	if broom >= campCrowded {
		return bx, by, bz, true
	}
	return
}
