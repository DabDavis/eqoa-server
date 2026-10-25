package entity

import "time"

// A creature that fights runs at its enemy (F3 20261025-165533: "Mobs
// currently do not chase you when they agro. They also do not attack
// consistently, only ever seeing the occasional hit if my character is in
// melee range"). Until then the AI dropped an enemy the moment it stood more
// than 10 from the NPC, which never moved: a pull from range ended the fight
// at once, and a player stepping about in a fight lost the creature's
// attention and its rounds with it, which came back only with the player's
// next hit; nor did a creature turn to face whom it fought.
//
// Now an NPC fighting a player:
//   - runs at it (NPCRunSpeed) until it is a little inside its reach
//     (npcCloseIn), and keeps there as its enemy moves, so a step back is
//     still within it and one that backs off is followed in one walk; its
//     velocity on its channel is the pace it moves at each tick, with the
//     walk or the run to match, as CLIENT's ExtrapolateMove and
//     SetLocomotion want it (a creature following a backing player walks
//     after it rather than sliding); a round's hits that fall due while it
//     is out of reach are lost (the next round begins in reach);
//   - in reach, fights its rounds (rounds.go), turned to its enemy every
//     tick, and once it has closed in, stands;
//   - gives up past its leash (LeashRadius from where the fight began, 100
//     for every NPC loaded) or when its enemy is far off or gone to another
//     world: its threats are dropped and it runs back to where the fight
//     began, a creature that does not wander (a guard at its post) facing
//     as it was spawned. Hit on the way, it fights again.
//
// No capture records a creature chasing (Sandstorm's wander at 5 a second,
// pcaps/ps2_sandstorm_1min10secs.pcap): the pace is the player's own base
// run, as a summoned pet's is (pet.go's petRunSpeed), so a creature keeps up
// with a player at that speed and falls behind one sped up.

// NPCRunSpeed is a creature's pace in a chase and on its way home.
const NPCRunSpeed float32 = 8.833333

// npcCloseIn is how far into its reach a chasing creature comes before it
// stops: a fraction of the reach.
const npcCloseIn float32 = 0.7

// chaseSlack is the shortest step a creature takes (none of a hair's
// width): closer than that to where it would stop, it stands.
const chaseSlack float32 = 0.05

// chaseGain is how fast a creature closes on where it would stop: its pace
// over its enemy's has this many units a second for each unit left.
const chaseGain float32 = 4

// npcGiveUp is how far off an enemy may be before the fight ends at once
// (a teleport, a fast runner long gone): past every client's view.
const npcGiveUp float32 = 300

// DefaultLeash is the leash of an NPC that has none set.
const DefaultLeash float32 = 100

// chaseState is a fight's movement: where it began (the leash's centre, and
// home after it), its last step, and the way home.
type chaseState struct {
	on           bool
	fromX, fromY float32
	fromZ        float32
	last         time.Time
	returning    bool
}

// npcReach is how near an NPC must be to hit: CLIENT's melee reach, the
// player's own against it (attack.go's meleeReach, and a big creature's
// allowance), so the two stand within each other's reach.
func npcReach(npc *NPC) float32 {
	return meleeReach + npcReachExtra(npc)
}

// chaseStepDT is the time since the fight's last step (a stalled tick does
// not jump it), the step's clock set to now.
func (n *NPC) chaseStepDT(now time.Time) float32 {
	dt := float32(0)
	if !n.chase.last.IsZero() {
		dt = float32(now.Sub(n.chase.last).Seconds())
	}
	n.chase.last = now
	if dt > 0.5 {
		dt = 0.5
	}
	if dt < 0 {
		dt = 0
	}
	return dt
}

// chaseLocked is a fighting NPC's movement this tick toward its enemy at
// (x, z), moving at (vx, vz) a second as its last word says (a player's
// 0x40): it reports whether the enemy is in reach (fight on), and gives up
// (giveUp) when the leash or the distance says so. Must be called with the
// lock held.
func (w *World) chaseLocked(npc *NPC, x, z, vx, vz float32, now time.Time) (inReach, giveUp bool) {
	if !npc.chase.on {
		npc.chase = chaseState{on: true, fromX: npc.X, fromY: npc.Y, fromZ: npc.Z}
	}
	npc.chase.returning = false
	dt := npc.chaseStepDT(now)
	d := distXZ(npc.X, npc.Z, x, z)
	leash := npc.LeashRadius
	if leash <= 0 {
		leash = DefaultLeash
	}
	if d > npcGiveUp || distXZ(npc.X, npc.Z, npc.chase.fromX, npc.chase.fromZ) > leash {
		return false, true
	}
	reach, stop := npcReach(npc), npcReach(npc)*npcCloseIn
	if d > 0 {
		npc.Facing = headingTo(x-npc.X, z-npc.Z)
	}
	// its pace: the speed its enemy moves away at (a player's velocity, out
	// of its last 0x40) and a closing on what is left to stop (chaseGain a
	// second of it), at most its run. A player's place changes only when its
	// 0x40 comes, about ten a second against the tick's ten, so a step to its
	// place each tick went by turns too far and nowhere: CLIENT drew a walk
	// and a stand by turns. A step that stopped dead on arriving, or a wait
	// in reach until its enemy left it, drew a slide.
	var away float32
	if d > 0 {
		away = max((vx*(x-npc.X)+vz*(z-npc.Z))/d, 0)
	}
	speed := NPCRunSpeed
	if npc.snared(now) {
		speed /= 2
	}
	pace := min(away+chaseGain*(d-stop), speed)
	step := pace * dt
	if step <= chaseSlack || npc.rooted(now) || dt <= 0 {
		// closed in, its enemy still (or held where it stands by a root):
		// standing, turned to its enemy
		if npc.moving() {
			npc.stopMoving()
		}
		return d <= reach, false
	}
	ux, uz := (x-npc.X)/d, (z-npc.Z)/d
	oldX, oldZ := npc.X, npc.Z
	npc.X += ux * step
	npc.Z += uz * step
	if w.SpatialIndex != nil {
		w.SpatialIndex.Update(npc.ServerID, oldX, oldZ, npc.X, npc.Z)
	}
	npc.VelX, npc.VelY, npc.VelZ = ux*pace, 0, uz*pace
	npc.Loco = LocoWalk
	if pace >= NPCRunFrom {
		npc.Loco = LocoRun
	}
	return d-step <= reach, false
}

// giveUpLocked ends an NPC's fight past its leash: its threats dropped,
// its round with them, and the way home begun. Must be called with the
// lock held.
func (w *World) giveUpLocked(npc *NPC) {
	npc.Threats = nil
	npc.CurrentEnemy = 0
	npc.endRound()
	w.goHomeLocked(npc)
}

// goHomeLocked starts an NPC that was fighting back to where the fight
// began (nothing to do for one that never moved). Must be called with the
// lock held.
func (w *World) goHomeLocked(npc *NPC) {
	if !npc.chase.on {
		return
	}
	npc.chase.on = false
	if distXZ(npc.X, npc.Z, npc.chase.fromX, npc.chase.fromZ) < 0.5 {
		w.atHomeLocked(npc)
		return
	}
	npc.chase.returning = true
	if !w.returningHas(npc) {
		w.returning = append(w.returning, npc)
	}
}

// returningHas reports an NPC already on its way home.
func (w *World) returningHas(npc *NPC) bool {
	for _, r := range w.returning {
		if r == npc {
			return true
		}
	}
	return false
}

// atHomeLocked is an NPC back where its fight began: standing, a creature
// that does not wander facing as it was spawned.
func (w *World) atHomeLocked(npc *NPC) {
	npc.chase.returning = false
	npc.stopMoving()
	if !w.wanders(npc) && distXZ(npc.X, npc.Z, npc.SpawnX, npc.SpawnZ) < 0.5 {
		npc.Facing = npc.SpawnFacing
	}
}

// homeSystem runs the NPCs on their way home after a fight (chase.go's doc)
// at the chase's pace; one that fights again, dies or leaves the world is
// let go. Every tick; must be called with the lock held.
func (w *World) homeSystem(now time.Time) {
	if len(w.returning) == 0 {
		return
	}
	kept := w.returning[:0]
	for _, npc := range w.returning {
		if w.NPCs[npc.ServerID] != npc || npc.Dead || !npc.chase.returning || npc.CurrentEnemy != 0 {
			if npc.CurrentEnemy == 0 {
				npc.chase.returning = false
			}
			continue
		}
		dt := npc.chaseStepDT(now)
		if npc.rooted(now) {
			npc.stopMoving()
			kept = append(kept, npc)
			continue
		}
		speed := NPCRunSpeed
		if npc.snared(now) {
			speed /= 2
		}
		if w.stepNPCToward(npc, npc.chase.fromX, npc.chase.fromZ, speed, dt) {
			w.atHomeLocked(npc)
			continue
		}
		kept = append(kept, npc)
	}
	w.returning = kept
}
