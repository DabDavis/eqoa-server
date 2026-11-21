package entity

import (
	"log"
	"time"
)

// followstuck.go — a pet stuck on its way to whom it follows is put beside it
// (petition 2592: a merc's charmed lava centipede in Lavastorm, following its
// master over a bridge, "is getting stuck on rock").
//
// A summoned pet farther than petLost from its owner, or in another world, is
// put beside it at once (pet.go); a charmed one past petLost runs to its master
// through anything (charm.go's run). Inside petLost every pet walks
// (followslot.go, round what stands between: petwalk.go), and a pet whose way
// the walk cannot find (a placed rock, a wall, a ledge) stood behind it for as
// long as its owner stayed within petLost.
//
// The rule (followUnstuckLocked), for a player's class pet or charmed creature
// (petSystem), a stacking charm's animal (stackcharm.go) and a merc's pet,
// summoned or charmed (mercpet.go), each in its follow turn alone:
//   - only while it follows: not fighting, not in a duel's or a PvP fight, not
//     casting its template's heals and buffs, not on a charm's run, not standing
//     with its merc stood down. Any of those, and its watch lapses (a turn not
//     seen for followStuckLapse starts it again), so the window counts only
//     unbroken following;
//   - only with whom it follows within reach: in its world and within petLost
//     (past that the lost rule or the charm's run has it);
//   - only far from where it heads: more than followStuckFar (12) off its slot
//     (its owner's place when it has none). A follower near its slot stands by
//     design (followHolds, its owner walking up to it), and 12 is past every
//     slot's radius (3.5 to 6.5) and followWalkFar (8), the farthest a follower
//     walks back at a walk, so a pet that far off is one that should be moving;
//   - no progress for followStuckWindow (5 s): progress is either its own
//     advance toward whom it follows (how much nearer to where its owner stands
//     NOW it is than the place it stood at the window's start: a measure its
//     owner's own movement does not touch, so an owner running away, the gap
//     growing, does not count against a pet that runs after him), or the gap
//     itself closed (its owner coming back to a pet waiting ahead of its slot),
//     each by followStuckProgress (1) at least. Either restarts the window.
//     A pet that follows advances 3 or more a second (its walk), 15 or more in
//     a window; one sliding along a wall or stepping back and forth behind a
//     rock does not.
//
// Then it is put beside its owner as the lost rule puts it
// (followPlaceLocked(pet, owner, slot): its slot, else one turned round its
// owner it can walk to, else its owner's place), stopped, its way forgotten,
// the spatial index told, and the server's log says so once.
//
// Nothing else is changed: a pet with no stay of its own (the 0x0041 commands
// are stances, dismiss, back off and attack) follows; a merc told to stay
// (/merc stay) holds its place by mercHomeLocked and is no pet here, and its pet
// follows it, not the hirer; a back off (heldUntil) holds only its fights. The
// watch is the NPC's own, in memory: a keep restart makes a pet again
// (keeppet.go) with a fresh one.

const (
	// followStuckFar is how far off its slot a following pet must be to be
	// watched
	followStuckFar float32 = 12
	// followStuckProgress is the least advance (or closing of the gap) that
	// counts as progress
	followStuckProgress float32 = 1
	// followStuckWindow is how long a far pet may go without progress
	followStuckWindow = 5 * time.Second
	// followStuckLapse is the longest between two follow turns of one watch:
	// a fight, a cast, a run between starts the watch again
	followStuckLapse = time.Second
)

// followStuck is a pet's watch on its way to whom it follows.
type followStuck struct {
	anchor *Player
	seen   time.Time // its last follow turn
	since  time.Time // the window's start
	x, z   float32   // where it stood then
	gap    float32   // how far off its anchor then
}

// petFollowSlotLocked is a pet's follow turn: put beside a when stuck
// (followUnstuckLocked), else to its slot s round a (followLocked). Must be
// called with the lock held.
func (w *World) petFollowSlotLocked(n *NPC, a *Player, s followSlot, dt float32, now time.Time) {
	if w.followUnstuckLocked(n, a, s, now) {
		return
	}
	w.followLocked(n, a, s, dt, now)
}

// followUnstuckLocked puts n beside a when it has been stuck on its way to its
// slot s round a (the file's doc), and reports whether it did. It looks at n
// where it stands before its step, so a pet held in place is seen as held.
// Must be called with the lock held.
func (w *World) followUnstuckLocked(n *NPC, a *Player, s followSlot, now time.Time) bool {
	st := &n.fstuck
	gap := distXZ(n.X, n.Z, a.X, a.Z)
	tx, tz := a.X, a.Z
	if followSlots {
		w.followFrameLocked(a, now)
		if sx, sz, ok := w.followSlotLocked(n, a, s); ok {
			tx, tz = sx, sz
		}
	}
	restart := func() {
		*st = followStuck{anchor: a, seen: now, since: now, x: n.X, z: n.Z, gap: gap}
	}
	if n.World != a.World || gap > petLost || distXZ(n.X, n.Z, tx, tz) <= followStuckFar {
		*st = followStuck{} // not watched: its window starts at its next far turn
		return false
	}
	if st.anchor != a || now.Sub(st.seen) > followStuckLapse {
		restart()
		return false
	}
	st.seen = now
	if distXZ(st.x, st.z, a.X, a.Z)-gap >= followStuckProgress || st.gap-gap >= followStuckProgress {
		restart() // it got on
		return false
	}
	if now.Sub(st.since) < followStuckWindow {
		return false
	}
	oldX, oldZ := n.X, n.Z
	n.X, n.Y, n.Z = w.followPlaceLocked(n, a, s)
	n.stopMoving()
	n.way = petWay{}
	if w.SpatialIndex != nil {
		w.SpatialIndex.Update(n.ServerID, oldX, oldZ, n.X, n.Z)
	}
	log.Printf("Pet: %q stuck %.1f off %s for %s at (%.0f, %.0f) in world %d: put beside it",
		n.Name, gap, a.Name, now.Sub(st.since).Round(time.Second), oldX, oldZ, n.World)
	*st = followStuck{}
	return true
}
