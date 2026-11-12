package entity

import (
	"math"
	"time"
)

// mercidle.go — a merc at ease (the owner, 2026-11-12: "also let merc after
// idle for 5 secs walk around the player ever so often, make it immersive").
//
// Once its owner has stood still mercIdleAfter (no step past followMoveSlack),
// and neither it nor its owner has fought, cast or auto-attacked for as long,
// a following merc (not one told to stay) takes its ease: every
// mercIdleEvery (a random time between its two figures) it does one thing,
// by chance:
//   - mercStrollShare: strolls at a walk (NPCWalkSpeed) to a place round its
//     owner, mercStrollNear to mercStrollFar off it, that it can walk to from
//     where it stands and from its owner (the leg a pet's way checks: a floor
//     all along, no fall, no wall), in its owner's sight (sight.go), out of
//     the water and apart (followApart) from its owner's other followers;
//     there it faces its owner, looks round, or stands as it came;
//   - mercHomeShare (once off its slot): walks back to its slot;
//   - else it looks round where it stands, and back to its owner after a
//     while (mercLookFor).
// Every turn is a creature's (npcTurnRate). Its owner's next move ends it at
// once: it follows to its slot as ever (followslot.go). A fight, a target, a
// cast of its or its owner's attack ends it too and starts the wait again; a
// cast on its way stands it where it is (mercfight.go's turn comes first).
// Pets and familiars do not idle (a familiar "usually only sit[s] there and do
// nothing": petition 1445).

const (
	mercIdleAfter      = 5 * time.Second
	mercIdleFirstLo    = 0 * time.Second
	mercIdleFirstHi    = 4 * time.Second
	mercIdleEveryLo    = 8 * time.Second
	mercIdleEveryHi    = 20 * time.Second
	mercLookForLo      = 1500 * time.Millisecond
	mercLookForHi      = 3 * time.Second
	mercStrollNear     = 3.0
	mercStrollFar      = 8.0
	mercStrollTries    = 8
	mercStrollShare    = 0.65
	mercHomeShare      = 0.20
	mercStrollStuckFor = 4 * time.Second
)

// What a merc does on coming to where it strolled.
const (
	idleThenStand byte = iota
	idleThenFaceOwner
	idleThenLook
)

// mercIdle is a merc's ease (the file's doc).
type mercIdle struct {
	// quiet is the last time it had a target or a cast on its way
	quiet time.Time
	// next is when it does its next thing; zero: not yet at ease
	next time.Time
	// off: it stands away from its slot (it strolled)
	off bool
	// walking to (x, z) since walkFrom, then doing then on arriving
	walking  bool
	x, z     float32
	walkFrom time.Time
	then     byte
	// turning to turnTo; back: when to turn back to its owner after a look
	turning bool
	turnTo  float32
	back    time.Time
	// strolls counts its strolls (tests, the rig's log)
	strolls int
}

// mercIdleNoteBusy marks the merc not at ease at now: a target, a cast.
func (ms *mercState) mercIdleNoteBusy(now time.Time) {
	q := ms.idle.quiet
	if now.After(q) {
		q = now
	}
	ms.idle = mercIdle{quiet: q, strolls: ms.idle.strolls}
}

// mercAtEaseLocked reports the merc and its owner quiet for mercIdleAfter:
// its owner standing, no fight, no cast, no auto-attack. Must be called with
// the lock held.
func (w *World) mercAtEaseLocked(ms *mercState, f *followFrame, now time.Time) bool {
	o, n := ms.owner, ms.npc
	since := func(t time.Time) bool { return t.IsZero() || !now.Before(t.Add(mercIdleAfter)) }
	return !ms.stay && ms.target == 0 && n.World == o.World && !o.Attacking() && !o.IsDead() &&
		since(ms.idle.quiet) && since(f.movedAt) && since(o.LastCombatTime) && since(n.LastCombatTime) &&
		!now.Before(ms.busy.Add(mercIdleAfter))
}

// mercIdleLocked is the merc's turn at ease: true when it has moved or
// turned it (or stands off its slot), false when it follows as ever. Must be
// called with the lock held.
func (w *World) mercIdleLocked(ms *mercState, dt float32, now time.Time) bool {
	o, n := ms.owner, ms.npc
	f := w.followFrameLocked(o, now)
	id := &ms.idle
	if !w.mercAtEaseLocked(ms, f, now) {
		if id.next != (time.Time{}) || id.off || id.walking || id.turning {
			*id = mercIdle{quiet: id.quiet, strolls: id.strolls}
		}
		return false
	}
	if id.next.IsZero() {
		id.next = now.Add(w.roamDuration(mercIdleFirstLo, mercIdleFirstHi))
	}
	switch {
	case id.walking:
		w.petWalkLocked(n, o.ServerID, id.x, id.z, o.X, o.Y, o.Z, min(NPCWalkSpeed, max(o.SpeedNow(), 3)), dt, now)
		if distXZ(n.X, n.Z, id.x, id.z) <= followArrive {
			n.stopMoving()
			id.walking = false
			w.mercIdleArrivedLocked(ms, now)
		} else if !n.moving() && now.Sub(id.walkFrom) > mercStrollStuckFor {
			id.walking = false // its way is gone: it stands where it got to
		}
		return true
	case id.turning:
		n.stopMoving()
		n.Facing = turnToward(n.Facing, id.turnTo, npcTurnRate*dt)
		if n.Facing == id.turnTo {
			id.turning = false
		}
		return true
	case !id.back.IsZero() && !now.Before(id.back):
		id.back = time.Time{}
		id.turning, id.turnTo = true, headingTo(o.X-n.X, o.Z-n.Z)
		return true
	case !now.Before(id.next):
		id.next = now.Add(w.roamDuration(mercIdleEveryLo, mercIdleEveryHi))
		w.mercIdleChooseLocked(ms, now)
		return id.walking || id.turning || id.off
	}
	if id.off {
		n.stopMoving()
		return true
	}
	return false
}

// mercIdleChooseLocked picks the merc's next thing at ease (the file's doc).
// Must be called with the lock held.
func (w *World) mercIdleChooseLocked(ms *mercState, now time.Time) {
	o, n, id := ms.owner, ms.npc, &ms.idle
	r := w.roamRand().Float32()
	if r < mercStrollShare {
		if x, z, ok := w.mercStrollSpotLocked(ms); ok {
			id.walking, id.x, id.z, id.walkFrom, id.off = true, x, z, now, true
			id.strolls++
			switch t := w.roamRand().Float32(); {
			case t < 0.5:
				id.then = idleThenFaceOwner
			case t < 0.8:
				id.then = idleThenLook
			default:
				id.then = idleThenStand
			}
			return
		}
	} else if r < mercStrollShare+mercHomeShare && id.off {
		if x, z, ok := w.followSlotLocked(n, o, slotMerc); ok {
			id.walking, id.x, id.z, id.walkFrom, id.off, id.then = true, x, z, now, false, idleThenStand
			id.strolls++
			return
		}
	}
	w.mercIdleLookLocked(ms, now)
}

// mercIdleArrivedLocked is what the merc does where it strolled to. Must be
// called with the lock held.
func (w *World) mercIdleArrivedLocked(ms *mercState, now time.Time) {
	o, n, id := ms.owner, ms.npc, &ms.idle
	switch id.then {
	case idleThenFaceOwner:
		id.turning, id.turnTo = true, headingTo(o.X-n.X, o.Z-n.Z)
	case idleThenLook:
		w.mercIdleLookLocked(ms, now)
	default:
		if !id.off {
			n.fslot.align = true // back in its slot: the formation's heading
		}
	}
}

// mercIdleLookLocked has the merc look round: a turn of 35 to 90 degrees to
// one side, and back to its owner after mercLookFor. Must be called with the
// lock held.
func (w *World) mercIdleLookLocked(ms *mercState, now time.Time) {
	n, id := ms.npc, &ms.idle
	rnd := w.roamRand()
	a := float32(35+rnd.Intn(56)) * math.Pi / 180
	if rnd.Intn(2) == 0 {
		a = -a
	}
	h := math.Mod(float64(n.Facing+a), 2*math.Pi)
	if h < 0 {
		h += 2 * math.Pi
	}
	id.turning, id.turnTo = true, float32(h)
	id.back = now.Add(w.roamDuration(mercLookForLo, mercLookForHi))
}

// mercStrollSpotLocked is a place the merc may stroll to (the file's doc),
// false with none found in mercStrollTries. Must be called with the lock
// held.
func (w *World) mercStrollSpotLocked(ms *mercState) (x, z float32, ok bool) {
	o, n := ms.owner, ms.npc
	rnd := w.roamRand()
	sibs := w.followSiblingsLocked(o, n)
	for range mercStrollTries {
		a := rnd.Float64() * 2 * math.Pi
		r := mercStrollNear + rnd.Float32()*(mercStrollFar-mercStrollNear)
		x = o.X + r*float32(math.Sin(a))
		z = o.Z + r*float32(math.Cos(a))
		if distXZ(x, z, n.X, n.Z) < followSlack {
			continue // no stroll at all
		}
		if !w.followSpotFreeLocked(o, sibs, x, z) {
			continue
		}
		near := false
		for _, s := range sibs {
			if distXZ(s.X, s.Z, x, z) < followApart {
				near = true
				break
			}
		}
		if near {
			continue
		}
		if w.ground != nil {
			if !w.legFromLocked(n, n.X, n.Y, n.Z, x, z, nil, legRoute) || !w.legFromLocked(n, o.X, o.Y, o.Z, x, z, nil, legRoute) {
				continue
			}
			y := o.Y
			if f, ok := w.ground.Floor(o.World, x, o.Y, z); ok {
				y = f
			}
			if !w.inSightLocked(o.World, o.X, o.Y, o.Z, x, y, z) || w.waterAtLocked(o.World, x, y+bodyAbove, z) {
				continue
			}
		}
		return x, z, true
	}
	return 0, 0, false
}
