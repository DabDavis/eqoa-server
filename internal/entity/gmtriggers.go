package entity

import (
	"math"
	"sort"
	"strings"
	"time"
)

// What a GM's trigger needs of the world (handlers/gmtriggers.go): copies of
// an NPC placed round a point on the ground, hostile ones attacking any
// player on sight, and where a player stands.

// triggerFloorReach is how far from the trigger's height the ground's floor
// may be for a spawn to stand on it; past it (a cave under the place, a
// cliff's top above) the spawn keeps the place's height.
const triggerFloorReach float32 = 20

// SpawnEventNPC places a copy of the NPC named name (SpawnNPCCopy's pick) at
// (x, z) in world, on the ground's floor under cy when the server has the
// ground files and one lies within triggerFloorReach, else at cy, facing
// (cx, cz): a GM trigger's spawn. A hostile one may be attacked (the NPC bit
// 0x80 cleared) and attacks any player on sight (EventAggro). It returns the
// copy's id and the name as the NPC spells it.
func (w *World) SpawnEventNPC(name string, world uint8, cx, cy, cz, x, z float32, hostile bool) (EntityID, string, bool) {
	y := cy
	if fy, ok := w.GroundFloor(world, x, cy+3, z); ok && fy-cy <= triggerFloorReach && cy-fy <= triggerFloorReach {
		y = fy
	}
	facing := float32(0)
	if dx, dz := cx-x, cz-z; dx != 0 || dz != 0 {
		facing = headingTo(dx, dz)
	}
	id, spelled, ok := w.SpawnNPCCopy(name, x, y, z, facing, world)
	if !ok {
		return 0, "", false
	}
	if hostile {
		w.mu.Lock()
		if c := w.NPCs[id]; c != nil {
			c.NPCType &^= 0x80
			c.EventAggro = true
		}
		w.mu.Unlock()
	}
	return id, spelled, true
}

// PlayerSpot is where a player stands and whether it is dead; ok false for
// one that is no longer the world's.
func (w *World) PlayerSpot(p *Player) (world uint8, x, y, z float32, dead, ok bool) {
	if p == nil {
		return 0, 0, 0, 0, false, false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	cur, found := w.Players[p.ServerID]
	if !found || cur != p {
		return 0, 0, 0, 0, false, false
	}
	return p.World, p.X, p.Y, p.Z, p.IsDead(), true
}

// NPCSpot is where an NPC stands, its corpse too: false for one gone.
func (w *World) NPCSpot(id EntityID) (world uint8, x, y, z float32, ok bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	npc := w.NPCs[id]
	if npc == nil {
		return 0, 0, 0, 0, false
	}
	return npc.World, npc.X, npc.Y, npc.Z, true
}

// npcsNearLocked is the living NPCs named name (any case) within reach of (x, z) in
// world: what a trigger's walk, home and hide take, the spawn-only originals not
// among them. Must be called with the lock held.
func (w *World) npcsNearLocked(name string, world uint8, x, z, reach float32) []*NPC {
	var out []*NPC
	for _, n := range w.NPCs {
		if n.Dead || n.SpawnOnly || n.World != world || !strings.EqualFold(strings.TrimSpace(n.Name), name) {
			continue
		}
		if distXZ(n.X, n.Z, x, z) <= reach {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ServerID < out[j].ServerID })
	return out
}

// NearestNPC is the living NPC named name (any case) nearest (x, z) in world within
// reach, not hidden: what a trigger's protect step guards. False: none.
func (w *World) NearestNPC(name string, world uint8, x, z, reach float32) (EntityID, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	now := time.Now()
	var best *NPC
	var bestD float32
	for _, n := range w.npcsNearLocked(strings.TrimSpace(name), world, x, z, reach) {
		if n.hiddenAt(now) {
			continue
		}
		if d := distXZ(n.X, n.Z, x, z); best == nil || d < bestD {
			best, bestD = n, d
		}
	}
	if best == nil {
		return 0, false
	}
	return best.ServerID, true
}

// PlayerNear reports a living player within reach of (x, z) in world.
func (w *World) PlayerNear(world uint8, x, z, reach float32) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	for _, p := range w.Players {
		if p.World == world && !p.IsDead() && distXZ(p.X, p.Z, x, z) <= reach {
			return true
		}
	}
	return false
}

// PlayerNearAt reports a living player within reach of (x, z) in world, standing no
// more than height above or below y: a trigger's awaitnear (the Plane of Sky's isles
// lie over one another, so the flat reach alone would take a player on the isle above).
func (w *World) PlayerNearAt(world uint8, x, y, z, reach, height float32) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	for _, p := range w.Players {
		if p.World == world && !p.IsDead() && distXZ(p.X, p.Z, x, z) <= reach && p.Y-y <= height && y-p.Y <= height {
			return true
		}
	}
	return false
}

// SpawnName is the name a copy of name would have: a row key's ("#119543") row's own
// name, else name as it is. What a trigger's spawn counts its max by.
func (w *World) SpawnName(name string, world uint8) string {
	if _, ok := ParseRowKey(name); !ok {
		return name
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	if t := w.npcByNameForLocked(name, world); t != nil {
		return t.Name
	}
	return name
}

// CountNear is how many living NPCs of that name stand within reach of (x, z) in world.
func (w *World) CountNear(name string, world uint8, x, z, reach float32) int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.npcsNearLocked(name, world, x, z, reach))
}

// MakeAttackable clears a copy's NPC bit, and no more: it does not attack on sight.
func (w *World) MakeAttackable(id EntityID) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if c := w.NPCs[id]; c != nil {
		c.NPCType &^= 0x80
	}
}

// RouseNPC has a run's copy attack any player on sight (EventAggro). False for one gone.
func (w *World) RouseNPC(id EntityID) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	c := w.NPCs[id]
	if c == nil || c.Dead {
		return false
	}
	c.NPCType &^= 0x80
	c.EventAggro = true
	return true
}

// SetNPCOn sets the NPC id on the nearest living NPC named target within reach
// of it, in its world (a trigger's sic: the user, 2026-11-08, "spawn nagafen
// attacking mr moogie"): a threat on it, as a hit would leave, so it is its
// enemy (aiSystem's fight with an NPC, guards.go); the target fights back at
// its first hit, as any creature hit does. Never itself, a pet, a dummy, a
// hidden one. With casts it fights that NPC by its whole class template, its
// casts and specials as well as its melee (the user, 2026-11-09: Tunare, "a
// strong druid god", fights Mr. Moogie with a druid's spells; castsAt,
// classtemplate.go); the target's own fighting back is its melee alone, as
// before. It reports the target's name, or false when none is near.
func (w *World) SetNPCOn(id EntityID, target string, reach float32, casts bool) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	c := w.NPCs[id]
	if c == nil || c.Dead {
		return "", false
	}
	now := time.Now()
	var best *NPC
	var bestD float32
	for _, n := range w.npcsNearLocked(strings.TrimSpace(target), c.World, c.X, c.Z, reach) {
		if n == c || n.Owner != 0 || n.Dummy != NotDummy || n.hiddenAt(now) || n.challenge != nil {
			continue // a Metal Moogie is his challenger's alone (metalmoogie.go)
		}
		if d := distXZ(c.X, c.Z, n.X, n.Z); best == nil || d < bestD {
			best, bestD = n, d
		}
	}
	if best == nil {
		return "", false
	}
	w.addThreatLocked(c, best.ServerID, 1)
	c.CurrentEnemy = best.ServerID
	c.castsAt = 0
	if casts {
		c.castsAt = best.ServerID
	}
	return best.Name, true
}

// WalkNPCsTo sets the NPCs of that name within reach of (x, z) running to a place
// (a guide's "the guards scurry over"), a step apart so they do not stand in each
// other: a path walked once, until SendNPCsHome. A fight takes them off it as it
// takes anyone off a path. It says how many went.
func (w *World) WalkNPCsTo(name string, world uint8, x, z, reach float32, toWorld uint8, tx, ty, tz float32) int {
	return w.WalkNPCsRoute(name, world, x, z, reach, toWorld, nil, tx, ty, tz, 0)
}

// WalkNPCsRoute is WalkNPCsTo by a route: the points in order, then (tx, ty, tz), at a
// pace of its own (0: the run's; under pathRunFrom it is walked: an escort, Ryini down
// her mountain).
func (w *World) WalkNPCsRoute(name string, world uint8, x, z, reach float32, toWorld uint8, route [][3]float32, tx, ty, tz, pace float32) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := time.Now()
	var npcs []*NPC
	for _, n := range w.npcsNearLocked(name, world, x, z, reach) {
		if !n.hiddenAt(now) {
			npcs = append(npcs, n)
		}
	}
	for i, n := range npcs {
		// it goes whatever it fought: "at some point it will run across the next bridge"
		n.Threats, n.CurrentEnemy, n.EventAggro = nil, 0, false
		a := float64(i) * 2 * math.Pi / float64(max(len(npcs), 1))
		px, pz := tx, tz
		if len(npcs) > 1 {
			px += float32(3 * math.Sin(a))
			pz += float32(3 * math.Cos(a))
		}
		pts := make([]PathPoint, 0, len(route)+1)
		for _, q := range route {
			pts = append(pts, PathPoint{X: q[0], Y: q[1], Z: q[2], Pause: 0})
		}
		pts = append(pts, PathPoint{X: px, Y: ty, Z: pz, Pause: -1})
		w.setPathLocked(n.ServerID, &NPCPath{World: toWorld, Mode: PathOnce, Run: pace == 0 || pace >= pathRunFrom, Pace: pace, Points: pts})
		w.wakeNPCLocked(n)
	}
	return len(npcs)
}

// SendNPCsHome takes the path WalkNPCsTo gave the NPCs of that name within reach of
// (x, z): each walks back to its spawn. Reach is from the place they left, so
// wherever they are now they are looked for across the world when it is wide
// enough. It says how many went.
func (w *World) SendNPCsHome(name string, world uint8, x, z, reach float32) int {
	w.mu.Lock()
	var ids []EntityID
	for _, n := range w.npcsNearLocked(name, world, x, z, reach) {
		if w.paths.byNPC[n.ServerID] != nil {
			ids = append(ids, n.ServerID)
		}
	}
	w.mu.Unlock()
	for _, id := range ids {
		w.ClearNPCPath(id)
	}
	return len(ids)
}

// HideNPCs keeps the NPCs of that name within reach of (x, z) out of every view until
// then (zero: back in view at once), as the panel's refresh hold does (hiddenAt): a
// guide's "he disappears". It says how many.
func (w *World) HideNPCs(name string, world uint8, x, z, reach float32, until time.Time) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	npcs := w.npcsNearLocked(name, world, x, z, reach)
	for _, n := range npcs {
		n.HiddenUntil = until
		if !until.IsZero() {
			w.respawnNPCLocked(n)
		}
	}
	return len(npcs)
}

// CopyStands reports a copy (a trigger's, a quest's or a GM's spawn: Temporary, not
// a spawn-only original) named any of names (any case) alive, or with corpses dead
// and its corpse still lying: what a quest's kill step (a loot step: its corpse too)
// could still be done on (handlers/questencounter.go).
func (w *World) CopyStands(names []string, corpses bool) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	now := time.Now()
	for _, n := range w.NPCs {
		if !n.Temporary || n.SpawnOnly || n.Dead && (!corpses || !n.corpseLies(now, w.corpseDecay)) {
			continue
		}
		for _, name := range names {
			if strings.TrimSpace(name) != "" && strings.EqualFold(strings.TrimSpace(n.Name), strings.TrimSpace(name)) {
				return true
			}
		}
	}
	return false
}
