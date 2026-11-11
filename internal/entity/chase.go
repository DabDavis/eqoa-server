package entity

import (
	"log"
	"math"
	"time"
)

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
//   - gives up past its leash (leashLocked from where the fight began, 300
//     for every NPC loaded: F3 20261027-170731, jib's "at least 3 times the
//     range" of the 100 it was, which PoD's Twins left after about 30 m) or
//     when its enemy is far off or gone to another world: its threats are
//     dropped and it runs back to where the fight
//     began, a creature that does not wander (a guard at its post) facing
//     as it was spawned. Hit on the way, it fights again.
//
// Sandstorm's chase is recorded (pcaps/ps2_main_capture.pcap, the Lv49
// guard Jasper Darvar after a bard who pulled him with a spell from 14 off,
// claude-notes/attack-range-20261025.md): he ran at 9.9 a second (107 units
// of path in 10.8 s, about a unit each tenth of a second; his channel said
// 10.3), so a player at the base run (8.83) does not get away; he came on at
// that pace until a step brought him within his reach of where the server
// last had the player, and stood there: 3.5 off a player who had stopped,
// the next round's swing 0.2 s later; a player who kept backing off he
// followed by turns. Until F3 pass 16 the pace was the player's own base run
// and the creature stood at 70% of its reach, 3.15 off: inside a big one's
// body, as nothing gave a big one its reach (reach.go).

// NPCRunSpeed is a creature's pace in a chase and on its way home.
const NPCRunSpeed float32 = 9.9

// runSpeed is the NPC's pace in a chase and on its way home: its template's
// run_speed, else NPCRunSpeed.
func (n *NPC) runSpeed() float32 {
	if n.tmpl != nil && n.tmpl.RunSpeed > 0 {
		return float32(n.tmpl.RunSpeed)
	}
	return NPCRunSpeed
}

// npcStopShort is how far inside its reach a chasing creature stands: half
// of Sandstorm's last step (a unit), whose stops fell between 3.5 and 4.5
// off a still player.
const npcStopShort float32 = 0.5

// chaseSlack is the shortest step a creature takes (none of a hair's
// width): closer than that to where it would stop, it stands.
const chaseSlack float32 = 0.05

// chaseGain is how fast a creature closes on where it would stop: its pace
// over its enemy's has this many units a second for each unit left. At 8 it
// runs at full pace to within a unit and a quarter of where it stops, as
// Sandstorm's came on at full pace to its last step (it was 4, an easing
// over the last 2.5 units that held it out of reach 0.2 s longer).
const chaseGain float32 = 8

// npcStop is where a chasing creature stands off its enemy: just inside its
// reach.
func npcStop(npc *NPC) float32 {
	return npcReach(npc) - npcStopShort
}

// npcGiveUp is how far off an enemy may be before the fight ends at once
// (a teleport, a fast runner long gone): past every client's view.
const npcGiveUp float32 = 300

// DefaultLeash is the leash of an NPC that has none set, and of every NPC
// loaded (ours: three times the 100 it was, F3 20261027-170731).
const DefaultLeash float32 = 300

// leashLocked is how far npc follows from where its fight began: a town
// guard's after a creature (guards.go), else a GM's npcaggro leash for its
// name (petition 1535), else its own, else DefaultLeash. Must be called with
// the lock held.
func (w *World) leashLocked(npc *NPC) float32 {
	if g := w.guardLeashLocked(npc); g > 0 {
		return g
	}
	if l := w.aggro.leashFor(npc.Name); l > 0 {
		return l
	}
	if npc.LeashRadius > 0 {
		return npc.LeashRadius
	}
	return DefaultLeash
}

// chaseState is a fight's movement: where it began (the leash's centre, and
// home after it), its last step, and the way home.
type chaseState struct {
	on           bool
	fromX, fromY float32
	fromZ        float32
	last         time.Time
	returning    bool
	route        chaseRoute // its way round, while it has one (chaseroute.go)
	// goal is the enemy its way was made for: a new enemy gets a way of its
	// own (forRoute), the chase's start kept for the leash
	goal EntityID
	// heldSince is when its steps toward its enemy began to be refused, the
	// ground in the way (chaseHeld); zero once one is not
	heldSince time.Time
	// homeHeldSince is the same on its way home (homeSystem)
	homeHeldSince time.Time
}

// chaseHeld is how long a chase's steps may be refused, the ground in their way
// and nothing to go round by (a swimmer under a bank too high to climb, its
// enemy on the land over it), before it gives up as past its leash: its enemy
// is out of its reach where it is, and stays so (petition 1500).
const chaseHeld = 8 * time.Second

// forRoute keeps the chase's way for the enemy id, dropping one made for
// another (petition 1280: a creature turned from a player to a pet ran on
// along its way to the player).
func (c *chaseState) forRoute(id EntityID) {
	if c.goal != id {
		c.route = chaseRoute{}
		c.goal = id
	}
}

// npcReach is how near an NPC must be to hit: CLIENT's melee reach, the
// player's own against it (attack.go's meleeReach, and a big creature's
// allowance, reach.go), so the two stand within each other's reach. Its
// hits land within it of where the server has the player, as Sandstorm's
// did (its own view of the player, a channel behind the player's 0x40, put
// every one of Jasper Darvar's 68 hits within 4.5 (4.44 the furthest),
// though he hit a player kiting him 5.7 to 7.7 off by the player's own
// reckoning).
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

// npcTurnRate is how fast a creature turns to the enemy in its reach, in radians
// a second, and npcTurnReach the reach that rate is for (petition 1177, Jibrish:
// "Mobs need to turn when in combat SLIGHTLY slower ... Rogue need to be able to
// run around them in circles and get off backstabs ... Needs a very subtle delay
// just enough for a rogue flipping around them on the sides"). Ours: a rogue at the
// walk's run of 8.83, circling at 3, goes round at 169 degrees a second, so it gains
// 34 degrees a second on the creature's 135 and is behind in under 3 s; at 11.48 in
// about 1. A larger creature's reach is circled wider, so it turns the slower for it.
const (
	npcTurnRate          = 135 * math.Pi / 180
	npcTurnReach float32 = 4.5
)

// turnedInFight is a creature's facing after dt of its fight with an enemy d off,
// wanting to face want: at once past its reach (a chase runs the way it faces), else
// by no more than its rate.
func turnedInFight(npc *NPC, want, d, reach, dt float32) float32 {
	if d > reach {
		return want
	}
	most := npcTurnRate * dt
	if reach > npcTurnReach {
		most *= npcTurnReach / reach
	}
	return turnToward(npc.Facing, want, most)
}

// turnToward is a heading turned the short way toward want, by no more than most
// radians.
func turnToward(from, want, most float32) float32 {
	d := math.Mod(float64(want-from), 2*math.Pi)
	if d > math.Pi {
		d -= 2 * math.Pi
	} else if d < -math.Pi {
		d += 2 * math.Pi
	}
	if math.Abs(d) <= float64(most) {
		return want
	}
	if d < 0 {
		most = -most
	}
	h := math.Mod(float64(from+most), 2*math.Pi)
	if h < 0 {
		h += 2 * math.Pi
	}
	return float32(h)
}

// chaseLocked is a fighting NPC's movement this tick toward its enemy at
// (x, y, z), moving at (vx, vz) a second as its last word says (a player's
// 0x40): it reports whether the enemy is in reach (fight on), and gives up
// (giveUp) when the leash or the distance says so. Its way is straight while
// the ground lets it run so, else round (chaseroute.go). Must be called with
// the lock held.
func (w *World) chaseLocked(npc *NPC, x, y, z, vx, vz float32, now time.Time) (inReach, giveUp bool) {
	if !npc.chase.on {
		npc.chase = chaseState{on: true, fromX: npc.X, fromY: npc.Y, fromZ: npc.Z}
	}
	npc.chase.returning = false
	if w.liftOutLocked(npc) {
		log.Printf("Chase: %s was in the ground at (%.1f, %.1f, %.1f): set on the floor over it", npc.Name, npc.X, npc.Y, npc.Z)
	}
	// its steps held by the ground since (kept only by a step refused again)
	held := npc.chase.heldSince
	npc.chase.heldSince = time.Time{}
	dt := npc.chaseStepDT(now)
	d := distXZ(npc.X, npc.Z, x, z)
	leash := w.leashLocked(npc)
	if d > npcGiveUp || distXZ(npc.X, npc.Z, npc.chase.fromX, npc.chase.fromZ) > leash {
		return false, true
	}
	reach, stop := npcReach(npc), npcStop(npc)
	// a creature that shoots fights from its bow's reach where it sees its enemy,
	// on another floor or not (npcshoot.go)
	shot, shooting := w.shootReachLocked(npc, d, x, y, z)
	if shooting {
		reach, stop = shot, shot-npcStopShort
	}
	// its enemy over or under it, on another floor within its reach across the
	// ground (a ledge): no fight from there, but its way round to it at its run,
	// where the ground has one (chaseroute.go)
	if !shooting && d <= reach+routeLook && absf(y-npc.Y) > routeLevel && !npc.rooted(now) && dt > 0 {
		run := npc.runSpeed() * npc.snareFactor(now)
		routing, hold := w.routeWayLocked(npc, &npc.chase.route, x, y, z, d, run*dt, true, now)
		if routing {
			w.followRouteLocked(npc, &npc.chase.route, run, dt)
			return false, false
		}
		if hold {
			npc.stopMoving()
			return false, false
		}
	}
	if d > 0 {
		npc.Facing = turnedInFight(npc, headingTo(x-npc.X, z-npc.Z), d, reach, dt)
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
	speed := npc.runSpeed()
	speed *= npc.snareFactor(now) // a snare's slow (workbook.go)
	pace := min(away+chaseGain*(d-stop), speed)
	step := pace * dt
	if step <= chaseSlack || npc.rooted(now) || dt <= 0 {
		// closed in, its enemy still (or held where it stands by a root):
		// standing, turned to its enemy
		if npc.moving() {
			npc.stopMoving()
			log.Printf("Chase: %s stands %.2f off its enemy (reach %.2f)", npc.Name, d, reach)
		}
		if !shooting && absf(y-npc.Y) > routeLevel && w.floorKnownLocked(npc.World, x, y, z) {
			// under or over its enemy, on another floor the ground knows, with
			// no way found to it: no fight through the floor (a clump of slime
			// on the floor 10 under its players, 2026-11-11), and held as a
			// blocked step is, so it gives up at chaseHeld. Where the ground
			// knows no floor under the enemy (something it stands on that the
			// baked ground lacks) the creature fights by the map, as before
			if held.IsZero() {
				held = now
			}
			npc.chase.heldSince = held
			return false, now.Sub(held) >= chaseHeld
		}
		return d <= reach, false
	}
	ux, uz := (x-npc.X)/d, (z-npc.Z)/d
	if w.staysInWaterLocked(npc) && !w.waterAheadLocked(npc, npc.X+ux*step, npc.Z+uz*step) {
		// a creature of the water stops at its edge, turned to its enemy
		// (water.go)
		if npc.moving() {
			npc.stopMoving()
		}
		return d <= reach, false
	}
	// the way: straight while the ground lets it run so, else round what stands
	// between, at its run (chaseroute.go)
	routing, hold := w.routeWayLocked(npc, &npc.chase.route, x, y, z, d, step, false, now)
	if routing {
		w.followRouteLocked(npc, &npc.chase.route, speed, dt)
		return false, false
	}
	if hold {
		if npc.moving() {
			npc.stopMoving()
		}
		return d <= reach && absf(y-npc.Y) <= routeLevel, false
	}
	oldX, oldY, oldZ, oldSwim := npc.X, npc.Y, npc.Z, npc.swimming
	guarded := w.routesFor(npc) && !now.Before(npc.chase.route.failedUntil)
	npc.X += ux * step
	npc.Z += uz * step
	noFloor := w.groundNPCLocked(npc)
	if (guarded && (npc.Y < oldY-legDrop || npc.Y > oldY+routeClimb)) || w.intoGroundLocked(npc, noFloor) {
		// off an edge the look ahead stepped over (or into the water below it),
		// or into the ground (a bank too high out of the water, a face too
		// steep: groundnpc.go): not taken, and the way searched at the next
		// step (chaseroute.go)
		npc.X, npc.Y, npc.Z, npc.swimming = oldX, oldY, oldZ, oldSwim
		npc.chase.route.edge = true
		npc.stopMoving()
		if held.IsZero() {
			held = now
		}
		npc.chase.heldSince = held
		in := d <= reach && absf(y-npc.Y) <= routeLevel
		return in, !in && now.Sub(held) >= chaseHeld
	}
	if w.SpatialIndex != nil {
		w.SpatialIndex.Update(npc.ServerID, oldX, oldZ, npc.X, npc.Z)
	}
	npc.VelX, npc.VelY, npc.VelZ = ux*pace, 0, uz*pace
	npc.Loco = LocoWalk
	if pace >= NPCRunFrom {
		npc.Loco = LocoRun
	}
	npc.Loco = locoIn(npc.Loco, npc.swimming, pace) // a chase into deep water swims (water.go)
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
	w.companionsHomeLocked(npc) // its pets home with it (companion.go)
}

// goHomeLocked starts an NPC that was fighting back to where the fight
// began (nothing to do for one that never moved). Must be called with the
// lock held.
func (w *World) goHomeLocked(npc *NPC) {
	if !npc.chase.on {
		return
	}
	npc.chase.on = false
	npc.chase.route = chaseRoute{} // the fight's way round: home is found anew
	npc.chase.heldSince, npc.chase.homeHeldSince = time.Time{}, time.Time{}
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

// atHomeLocked is an NPC back where its fight began: standing, at the height it
// stood at there, a creature that does not wander facing as it was spawned. A place
// can have two floors, and one that came home by the lower stayed on it (Lord
// Nagafen, off his island's edge in a chase, came back to stand under it in the lava:
// the user, 2026-11-07, "nagafen is standing on the lava not island after killing
// raid").
func (w *World) atHomeLocked(npc *NPC) {
	npc.chase.returning = false
	npc.stopMoving()
	if distXZ(npc.X, npc.Z, npc.chase.fromX, npc.chase.fromZ) < 0.5 {
		npc.Y = npc.chase.fromY
	}
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
		speed := npc.runSpeed()
		speed *= npc.snareFactor(now) // a snare's slow (workbook.go)
		// round what stands between, where the straight way home is not one it
		// can run (chaseroute.go: off a ledge it was led down, up the ramp)
		home := distXZ(npc.X, npc.Z, npc.chase.fromX, npc.chase.fromZ)
		routing, hold := w.routeWayLocked(npc, &npc.chase.route, npc.chase.fromX, npc.chase.fromY, npc.chase.fromZ, home, speed*dt,
			home <= routeLook && absf(npc.chase.fromY-npc.Y) > routeLevel, now)
		if routing || hold {
			if routing {
				w.followRouteLocked(npc, &npc.chase.route, speed, dt)
			} else {
				npc.stopMoving()
			}
			kept = append(kept, npc)
			continue
		}
		// the ground is asked from the height it left at or its own, the higher: below a
		// floor it left, it would walk home underneath it
		npc.Y = max(npc.Y, npc.chase.fromY)
		w.liftOutLocked(npc) // groundnpc.go
		bx, bz := npc.X, npc.Z
		if w.stepNPCToward(npc, npc.chase.fromX, npc.chase.fromZ, speed, dt) {
			w.atHomeLocked(npc)
			continue
		}
		if npc.X == bx && npc.Z == bz && dt > 0 {
			// its step refused, the ground in the way (groundnpc.go): held so
			// long, it is set where its fight began, as a creature that evades
			// home (petition 1500)
			if npc.chase.homeHeldSince.IsZero() {
				npc.chase.homeHeldSince = now
			} else if now.Sub(npc.chase.homeHeldSince) >= chaseHeld {
				log.Printf("Chase: %s held by the ground on its way home at (%.1f, %.1f, %.1f): set home", npc.Name, npc.X, npc.Y, npc.Z)
				oldX, oldZ := npc.X, npc.Z
				npc.X, npc.Y, npc.Z = npc.chase.fromX, npc.chase.fromY, npc.chase.fromZ
				npc.swimming = false
				if w.SpatialIndex != nil {
					w.SpatialIndex.Update(npc.ServerID, oldX, oldZ, npc.X, npc.Z)
				}
				w.atHomeLocked(npc)
				continue
			}
		} else {
			npc.chase.homeHeldSince = time.Time{}
		}
		kept = append(kept, npc)
	}
	w.returning = kept
}

// floorKnownLocked reports a floor of the ground under a body at (x, y, z),
// within floorKnownBelow under it: where it stands is a floor the server
// knows, so a creature on another floor is truly on another floor. False
// without a ground. Must be called with the lock held.
func (w *World) floorKnownLocked(world uint8, x, y, z float32) bool {
	if w.ground == nil {
		return false
	}
	f, ok := w.ground.Floor(world, x, y, z)
	return ok && y-f <= floorKnownBelow && f-y <= floorKnownBelow
}

// floorKnownBelow is how far over a known floor a standing body may be
// reported (a player's reports ride a little over the ground; a jump).
const floorKnownBelow float32 = 3
