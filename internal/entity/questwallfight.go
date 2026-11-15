package entity

import "log"

// questwallfight.go — no fight through a quest wall (petition 2198,
// Trulytwo, Heirloom, Isle of Dread: "Dracoliche is still attackable with
// pets thru the walls in IoD").
//
// A quest wall (questwalls.go) is a box CLIENT stops its player's walk at,
// 5 a side times its size about its place (the invisible blocker's,
// claude-notes/invisible-blockers-20261029.md), and nothing on the server's
// side: a pet, a charmed creature, a merc or a merc's pet walked through it
// to what stood behind (the Dracoliche, in the alcove past the Cazic-Thule
// chamber's wall), and its owner's spells reached through it. Now a creature
// behind a wall its owner does not pass (wallPassedLocked: its quest's step,
// its item, the far side) is none of theirs:
//
//   - a pet's attack command, /merc attack and a charm's are refused
//     (petBehindWall);
//   - a summoned or charmed pet, a merc and a merc's pet choose no target
//     behind one, and let a fight go once one stands between (their target
//     choices: pet.go, mercfight.go, mercpet.go);
//   - a player's harmful spell and a ranged shot at a creature behind one are
//     refused (castNotSeen), as out of sight.
//
// A wall is between two places when the line between them, on the ground's
// plan, crosses its box and their heights meet its box's (a wall on another
// floor stands between nothing).

// petBehindWall is the refusal of an attack at a creature behind a wall.
const petBehindWall = "Your target is behind a wall."

// wallBetweenLocked is the first wall the owner does not pass standing
// between the two places in a world, false for none. Must be called with the
// lock held.
func (w *World) wallBetweenLocked(owner *Player, world uint8, ax, ay, az, bx, by, bz float32) (*NPC, bool) {
	if owner == nil || len(w.qwalls.walls) == 0 {
		return nil, false
	}
	for id := range w.qwalls.walls {
		n := w.NPCs[id]
		if n == nil || n.World != world || n.Dead || w.wallPassedLocked(n, owner) {
			continue
		}
		size := n.Size
		if size <= 0 {
			size = 1
		}
		h := 2.5 * size
		if min(ay, by)+sightWaist > n.Y+h || max(ay, by)+sightHead < n.Y-h {
			continue // another floor's
		}
		if segmentCrossesBox(ax, az, bx, bz, n.X-h, n.Z-h, n.X+h, n.Z+h) {
			return n, true
		}
	}
	return nil, false
}

// segmentCrossesBox is whether the segment from (ax, az) to (bx, bz) meets
// the box (x0, z0)..(x1, z1) (the slab test).
func segmentCrossesBox(ax, az, bx, bz, x0, z0, x1, z1 float32) bool {
	t0, t1 := float32(0), float32(1)
	for _, s := range [2][4]float32{{ax, bx - ax, x0, x1}, {az, bz - az, z0, z1}} {
		p, d, lo, hi := s[0], s[1], s[2], s[3]
		if d == 0 {
			if p < lo || p > hi {
				return false
			}
			continue
		}
		u, v := (lo-p)/d, (hi-p)/d
		if u > v {
			u, v = v, u
		}
		t0, t1 = max(t0, u), min(t1, v)
		if t0 > t1 {
			return false
		}
	}
	return true
}

// walledOffLocked is whether a wall its owner does not pass stands between
// the creature t and its owner, or t and the one fighting for him (a pet, a
// merc, a merc's pet; nil none). Must be called with the lock held.
func (w *World) walledOffLocked(owner *Player, fighter, t *NPC) bool {
	if owner == nil || t == nil || len(w.qwalls.walls) == 0 {
		return false
	}
	if owner.World == t.World {
		if _, ok := w.wallBetweenLocked(owner, t.World, owner.X, owner.Y, owner.Z, t.X, t.Y, t.Z); ok {
			return true
		}
	}
	if fighter != nil && fighter.World == t.World {
		if _, ok := w.wallBetweenLocked(owner, t.World, fighter.X, fighter.Y, fighter.Z, t.X, t.Y, t.Z); ok {
			return true
		}
	}
	return false
}

// playerWalledOffLocked is whether a wall the player does not pass stands
// between him and the creature, logged. Must be called with the lock held.
func (w *World) playerWalledOffLocked(p *Player, t *NPC, what string) bool {
	if p == nil || t == nil || p.World != t.World {
		return false
	}
	wall, ok := w.wallBetweenLocked(p, t.World, p.X, p.Y, p.Z, t.X, t.Y, t.Z)
	if ok {
		log.Printf("Wall: %s's %s at %s refused: wall %d between", p.Name, what, t.Name, wall.ServerID)
	}
	return ok
}
