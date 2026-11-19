package entity

import "math"

// backstab.go: a backstab needs its caster behind its target (the user,
// 2026-11-01: "yes need behind"). The workbook marks the rogue's backstab arts
// (its "backstab" effects: Rush, Assault, Assail, Mug, Ambush, the Backstab
// line and its masteries' "+", Poisoned Blade +, Silent Blade); until then
// they hit from anywhere. Behind is the creature's rear half: the caster more
// than a quarter turn from its facing, as seen from the creature (ours), the
// rear widened 3% (backstabArc, petition 2196). A
// creature faces what it swings at or casts on (faceLocked) and what it
// chases (chase.go), so a rogue behind a creature another holds may
// backstab; and one the creature fights may too, by outrunning its turn
// (npcTurnRate, chase.go).

// castNotBehind is a backstab's refusal from in front.
const castNotBehind = "You must be behind your target to do that."

// faceLocked turns a creature to face an entity by its id, a player or a
// creature; nothing when it stands where it stands or is not found, or for one
// in a chase's fight, which turns at its own rate each tick (chase.go).
func (w *World) faceLocked(npc *NPC, target EntityID) {
	if npc.chase.on {
		return
	}
	var x, z float32
	switch {
	case w.Players[target] != nil:
		x, z = w.Players[target].X, w.Players[target].Z
	case w.NPCs[target] != nil:
		x, z = w.NPCs[target].X, w.NPCs[target].Z
	default:
		return
	}
	if x != npc.X || z != npc.Z {
		npc.Facing = headingTo(x-npc.X, z-npc.Z)
	}
}

// backstabArc is how much wider than a half turn a creature's rear is for a
// backstab (petition 2196: "Assassinate or Backstab spells need a
// slightly larger 'behind enemy' radius 3 percent more to start"): its rear's
// half-angle a quarter turn's (90 degrees) times this, 92.7 degrees, so a
// caster more than 87.3 degrees from its facing is behind it.
const backstabArc = 1.03

// behindFrom is the least angle off a creature's facing, seen from it, that
// is behind it (backstabArc).
const behindFrom = math.Pi - math.Pi/2*backstabArc

// behind reports whether a place is in a creature's rear (backstabArc's
// widened half).
func behind(npc *NPC, x, z float32) bool {
	d := math.Mod(float64(headingTo(x-npc.X, z-npc.Z)-npc.Facing), 2*math.Pi)
	if d > math.Pi {
		d -= 2 * math.Pi
	} else if d < -math.Pi {
		d += 2 * math.Pi
	}
	return math.Abs(d) > behindFrom
}

// backstabRefusalLocked refuses a backstab at a creature its caster is not
// behind. Must be called with the lock held.
func (w *World) backstabRefusalLocked(p *Player, targets []EntityID) string {
	for _, id := range targets {
		if npc := w.NPCs[id]; npc != nil && !behind(npc, p.X, p.Z) {
			return castNotBehind
		}
	}
	return ""
}
