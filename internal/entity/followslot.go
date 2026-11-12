package entity

import (
	"math"
	"time"
)

// followslot.go — each of a player's followers keeps a place of its own round
// it (the owner, 2026-11-12: "the mercenary and healbot are following jib
// perfectly stacked on top of each other, lets have them spread out").
//
// Why they stacked: every follower (a class pet, a merc, a familiar, a merc's
// own pet) came to petFollow off its owner along the line from its owner to
// itself (followTrailLocked). Two following from behind come up that one line,
// so they stood in one place, and stayed there.
//
// Now each takes a slot of its own round whom it follows, by its kind
// (followSlot): the class pet at its owner's right, the merc behind its left,
// the familiar behind its right, a merc's pet behind its merc. A slot is
// followRadius off (more for a big body: followRadiusOf), so two neighbours'
// are 2.7 apart at the least, and the owner's own place 3.5 off each. The
// slots turn with the formation's heading (followFrame): its owner's facing
// as it last moved, so turning on the spot does not send its followers round
// it. A slot its follower cannot walk to from its owner (a wall, a ledge, no
// floor: movement.go's leg as a pet's way checks one) is tried again turned
// round its owner (followTries), then nearer; one too near another
// follower's slot likewise; with none, the follower goes as before
// (followTrailLocked: the line, petFollow off).
//
// How it goes there: standing within followSlack of its slot it stands (a
// step of its owner's does not stir it); else it walks there. With its owner
// moving it goes at its owner's measured pace and a little more for each unit
// it is behind (followGain), toward its slot a little ahead (followLead), so
// it keeps up without stopping and starting; its owner standing it walks
// (NPCWalkSpeed: the walk, not the run), running only from followWalkFar off;
// past petCatchUp half as fast again as its owner, as before. Arrived, it turns
// to the formation's heading (npcTurnRate). Every step is the one a pet's
// always was (petwalk.go: round what stands between), so the client sees it
// as any moving creature, at the usual rate.
//
// A merc idles too (the same brief: "let merc after idle for 5 secs walk
// around the player ever so often"): mercIdleLocked.

// followSlot is a follower's place round whom it follows.
type followSlot uint8

const (
	slotPet      followSlot = iota // a player's class pet (or charmed creature): its right
	slotMerc                       // a merc: behind its left
	slotFamiliar                   // a familiar: behind its right
	slotMercPet                    // a merc's own pet: behind its merc
)

// followSlotAngle is each slot's direction off the formation's heading,
// toward its owner's right for a positive one.
var followSlotAngle = [...]float32{
	slotPet:      math.Pi / 2,
	slotMerc:     -3 * math.Pi / 4,
	slotFamiliar: 3 * math.Pi / 4,
	slotMercPet:  math.Pi,
}

const (
	// followRadius is how far off whom it follows a body of size 1 stands
	followRadius float32 = 3.5
	// followSlack is how far off its slot a standing follower may be and
	// still stand
	followSlack float32 = 1.5
	// followArrive is how near its slot a moving one stops (its owner
	// standing)
	followArrive float32 = 0.3
	// followApart is the least distance between two followers' slots
	followApart float32 = 2.2
	// followMoveSlack is how far its owner goes before the formation is
	// moved (its place, its heading and its pace)
	followMoveSlack float32 = 0.3
	// followMovingFor is how long after its last move its owner counts as
	// moving
	followMovingFor = 750 * time.Millisecond
	// followLead is how far ahead (seconds of its owner's pace) a follower
	// aims while its owner moves; followGain the pace added for each unit
	// it is off its slot
	followLead float32 = 0.4
	followGain float32 = 0.6
	// followWalkFar is how far off its slot a follower walks back to it with
	// its owner standing; farther it runs
	followWalkFar float32 = 8
)

// followTries are the slots tried in turn, turned round its owner (radians)
// and drawn in (a share of followRadiusOf), the first the slot itself.
var followTries = [...]struct{ turn, share float32 }{
	{0, 1}, {0.4, 1}, {-0.4, 1}, {0.8, 1}, {-0.8, 1}, {0, 0.6}, {0.4, 0.6}, {-0.4, 0.6},
}

// followFrame is the formation of a player's followers: where it last stood,
// its heading as it last moved, and its pace.
type followFrame struct {
	set     bool
	world   uint8
	x, z    float32
	heading float32
	movedAt time.Time
	// vx, vz is its owner's velocity over its last move
	vx, vz float32
	// stamp counts the frame's changes (followSlotCache)
	stamp uint32
}

// followSlotCache is the place a follower found for its slot, for its
// anchor's frame of one stamp.
type followSlotCache struct {
	anchor *Player
	stamp  uint32
	slot   followSlot
	x, z   float32
	ok     bool
	// align: it arrived at its slot and turns to the formation's heading
	align bool
}

// followFrameLocked brings p's formation up to where p stands now and returns
// it. Must be called with the lock held.
func (w *World) followFrameLocked(p *Player, now time.Time) *followFrame {
	f := &p.follow
	if f.set && f.world == p.World && distXZ(f.x, f.z, p.X, p.Z) <= followMoveSlack {
		return f
	}
	if f.set && f.world == p.World {
		dx, dz := p.X-f.x, p.Z-f.z
		d := float32(math.Sqrt(float64(dx*dx + dz*dz)))
		el := float32(now.Sub(f.movedAt).Seconds())
		switch {
		case d > petLost: // a teleport
			f.vx, f.vz = 0, 0
		default:
			// its pace over the step, never past its speed (a step after
			// standing, a GM's move or a coach's: its speed at the most)
			s := p.SpeedNow() * 1.25
			if el > 0 && el < 1.5 {
				s = min(s, d/el)
			}
			f.vx, f.vz = dx/d*s, dz/d*s
		}
		f.movedAt = now
	} else {
		// first seen (or in another world): standing, from now
		f.vx, f.vz = 0, 0
		f.movedAt = now.Add(-followMovingFor)
	}
	f.set, f.world, f.x, f.z, f.heading = true, p.World, p.X, p.Z, p.Facing
	f.stamp++
	return f
}

// moving reports its owner moving at now.
func (f *followFrame) moving(now time.Time) bool {
	return f.set && now.Sub(f.movedAt) < followMovingFor
}

// pace is its owner's measured speed while it moves.
func (f *followFrame) pace(now time.Time) float32 {
	if !f.moving(now) {
		return 0
	}
	return float32(math.Sqrt(float64(f.vx*f.vx + f.vz*f.vz)))
}

// followRadiusOf is how far off whom it follows a follower's slot is: more
// for a body bigger than a man's.
func followRadiusOf(n *NPC) float32 {
	return followRadius + min(max(n.Size-1, 0)*1.5, 3)
}

// followSpotAt is the place at heading+angle, r off (x, z).
func followSpotAt(x, z, heading, angle, r float32) (float32, float32) {
	h := float64(heading + angle)
	return x + r*float32(math.Sin(h)), z + r*float32(math.Cos(h))
}

// followPlaceLocked is where a follower of the slot is put beside p (a
// summoning, a teleport): its slot off p's facing, else the first of
// followTries it can walk to from p (a slot in a wall is not taken), else p's
// own place. n's world must be p's. Must be called with the lock held.
func (w *World) followPlaceLocked(n *NPC, p *Player, s followSlot) (x, y, z float32) {
	for _, t := range followTries {
		x, z := followSpotAt(p.X, p.Z, p.Facing, followSlotAngle[s]+t.turn, followRadius*t.share)
		if w.followSpotWalkableLocked(n, p, x, z) {
			return x, p.Y, z
		}
	}
	return p.X, p.Y, p.Z
}

// followSiblingsLocked is the other followers round a: its class pet, its
// merc and its familiar but n. Must be called with the lock held.
func (w *World) followSiblingsLocked(a *Player, n *NPC) []*NPC {
	var out []*NPC
	if ps := w.pets[a.ServerID]; ps != nil && ps.npc != nil && ps.npc != n {
		out = append(out, ps.npc)
	}
	if ms := w.mercs[a.ServerID]; ms != nil && ms.npc != nil && ms.npc != n {
		out = append(out, ms.npc)
	}
	if fs := w.familiars[a.ServerID]; fs != nil && fs.npc != nil && fs.npc != n {
		out = append(out, fs.npc)
	}
	return out
}

// followSlotLocked is the place n takes for slot s round a in a's formation
// as it stands: false with none it can walk to (it then follows on the
// line). Must be called with the lock held.
func (w *World) followSlotLocked(n *NPC, a *Player, s followSlot) (x, z float32, ok bool) {
	f, c := &a.follow, &n.fslot
	if c.anchor == a && c.stamp == f.stamp && c.slot == s {
		return c.x, c.z, c.ok
	}
	align := c.align && c.anchor == a
	*c = followSlotCache{anchor: a, stamp: f.stamp, slot: s, align: align}
	r := followRadiusOf(n)
	sibs := w.followSiblingsLocked(a, n)
	for _, t := range followTries {
		x, z := followSpotAt(a.X, a.Z, f.heading, followSlotAngle[s]+t.turn, r*t.share)
		if !w.followSpotFreeLocked(a, sibs, x, z) || !w.followSpotWalkableLocked(n, a, x, z) {
			continue
		}
		c.x, c.z, c.ok = x, z, true
		return x, z, true
	}
	return 0, 0, false
}

// followSpotFreeLocked reports (x, z) apart from every sibling's slot in a's
// formation now. Must be called with the lock held.
func (w *World) followSpotFreeLocked(a *Player, sibs []*NPC, x, z float32) bool {
	for _, o := range sibs {
		c := &o.fslot
		if c.anchor == a && c.stamp == a.follow.stamp && c.ok && distXZ(c.x, c.z, x, z) < followApart {
			return false
		}
	}
	return true
}

// followSpotWalkableLocked reports a way from a's place to (x, z) a pet
// walks (movement.go's legRoute: a floor all along, no fall, no wall);
// every place without a ground. Must be called with the lock held.
func (w *World) followSpotWalkableLocked(n *NPC, a *Player, x, z float32) bool {
	if w.ground == nil {
		return true
	}
	return w.legFromLocked(n, a.X, a.Y, a.Z, x, z, nil, legRoute)
}

// followLocked takes npc to its slot s round owner (the file's doc). Must be
// called with the lock held.
func (w *World) followLocked(npc *NPC, owner *Player, s followSlot, dt float32, now time.Time) {
	f := w.followFrameLocked(owner, now)
	sx, sz, ok := w.followSlotLocked(npc, owner, s)
	if !ok {
		w.followTrailLocked(npc, owner, dt, now)
		return
	}
	d := distXZ(npc.X, npc.Z, owner.X, owner.Z)
	ds := distXZ(npc.X, npc.Z, sx, sz)
	moving := f.moving(now)
	// over or under its owner on another floor while the ground has a way
	// to it (petwalk.go): it goes round, wherever it stands
	stacked := d <= petFollowStart && w.petStackedLocked(npc, owner.X, owner.Y, owner.Z, now)
	if !stacked && (ds <= followArrive && !moving || !npc.moving() && ds <= followSlack) {
		if npc.moving() {
			npc.fslot.align = true // arrived
		}
		npc.stopMoving()
		if npc.fslot.align && dt > 0 {
			npc.Facing = turnToward(npc.Facing, f.heading, npcTurnRate*dt)
			npc.fslot.align = npc.Facing != f.heading
		}
		return
	}
	npc.fslot.align = false
	top := max(owner.SpeedNow(), 3)
	gx, gz := sx, sz
	var speed float32
	if moving {
		if (npc.X-sx)*f.vx+(npc.Z-sz)*f.vz > 0 {
			// ahead of its slot as its owner comes on: it waits for it
			npc.stopMoving()
			return
		}
		speed = min(max(f.pace(now)+followGain*ds, 3), top*1.5)
		gx += f.vx * followLead
		gz += f.vz * followLead
	} else {
		speed = min(NPCWalkSpeed, top)
		if ds > followWalkFar {
			speed = top
		}
	}
	if d > petCatchUp {
		speed = max(speed, top*1.5)
	}
	w.petWalkLocked(npc, owner.ServerID, gx, gz, owner.X, owner.Y, owner.Z, speed, dt, now) // round what stands between
	if !moving && distXZ(npc.X, npc.Z, sx, sz) <= followArrive {
		npc.fslot.align = true
	}
}

// followTrailLocked keeps npc petFollow off its owner on the line from its
// owner to it (the follow before slots, for a follower whose slot has no way
// to it): from petFollowStart away, at its owner's speed, half as fast again
// past petCatchUp. Must be called with the lock held.
func (w *World) followTrailLocked(npc *NPC, owner *Player, dt float32, now time.Time) {
	d := distXZ(npc.X, npc.Z, owner.X, owner.Z)
	// beside its owner it stands; but not on the floor over or under it while
	// the ground has a way to its owner's (petwalk.go)
	stacked := d <= petFollowStart && w.petStackedLocked(npc, owner.X, owner.Y, owner.Z, now)
	if !stacked && (d <= petFollow || (npc.Loco == LocoStand && d <= petFollowStart)) {
		npc.stopMoving()
		return
	}
	speed := max(owner.SpeedNow(), 3)
	if d > petCatchUp {
		speed *= 1.5
	}
	x, z := npc.X, npc.Z
	if d > petFollow {
		x = owner.X + (npc.X-owner.X)/d*petFollow
		z = owner.Z + (npc.Z-owner.Z)/d*petFollow
	}
	w.petWalkLocked(npc, owner.ServerID, x, z, owner.X, owner.Y, owner.Z, speed, dt, now) // round what stands between
}
