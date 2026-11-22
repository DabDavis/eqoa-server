package entity

import (
	"fmt"
	"log"
	"time"

	"github.com/eqoa/server/internal/session"
)

// merccamp.go — /merc camp (a GM's ask, board entry 1657: "This sets where the
// player is standing to a camp. The mercenaries stand there, and do not follow
// the player unless the player moves 125 meters away, then the camp will break
// and they follow. Hitting the command again untoggles it. Mercs should behave
// as normal when the player attacks under /camp, run toward and engage.").
//
// The camp is the owner's spot, world and facing at the command; each merc
// holds its follow slot round it (followPlaceLocked), walks back to it after a
// fight and faces the owner's way, takes no ease there, and fights as ever,
// looking for its fights round the camp as well as round its owner
// (mercAroundLocked, mercGuardsLocked). The owner past MercCampBreak of the
// spot, or in another world, breaks it (mercCampLeftLine), as do /merc follow
// and /merc stay; a merc breaking camp runs back to its owner instead of being
// put beside it (mercLostLocked: MercCampBreak + petLost while camped or on
// the way back, until within petCatchUp), with no ease on the way. A keep
// restart carries it (keepmerc.go: KeptMerc.Camp); a logout ends it.

// MercCampBreak is how far from its camp's spot an owner may go before the
// camp breaks (a unit is a meter, as petAssist's 30).
const MercCampBreak float32 = 125

// mercCampLeftLine tells the owner a camp broke by its going (ours).
const mercCampLeftLine = "You left your camp: %s follows you."

// mercCamp is a merc's camp: its owner's spot and facing at the command, in
// its world, and the merc's own place there.
type mercCamp struct {
	World         uint8
	X, Y, Z, Face float32
	At            [3]float32
}

// ToggleMercCamp is /merc camp for the session's player's merc of the slot (0:
// every one out): when any of them holds a camp, those break it (camped
// false), else each makes one where its owner stands. n is how many took it.
func (w *World) ToggleMercCamp(sess *session.Session, slot int) (n int, camped bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	list := w.mercPickLocked(p, slot)
	if len(list) == 0 {
		return 0, false
	}
	var held []*mercState
	for _, ms := range list {
		if ms.camp != nil {
			held = append(held, ms)
		}
	}
	if len(held) > 0 {
		for _, ms := range held {
			w.breakMercCampLocked(ms)
		}
		return len(held), false
	}
	for _, ms := range list {
		x, y, z := w.followPlaceLocked(ms.npc, p, mercFollowSlot(ms.slot))
		ms.camp = &mercCamp{World: p.World, X: p.X, Y: p.Y, Z: p.Z, Face: p.Facing, At: [3]float32{x, y, z}}
		ms.stay, ms.campBack = false, false
		ms.idle = mercIdle{quiet: ms.idle.quiet, strolls: ms.idle.strolls}
		log.Printf("Merc: %s's %q makes camp at (%.1f, %.1f) in world %d", p.Name, ms.npc.Name, p.X, p.Z, p.World)
	}
	return len(list), true
}

// breakMercCampLocked ends the merc's camp, if it holds one: it runs back to
// its owner. Must be called with the lock held.
func (w *World) breakMercCampLocked(ms *mercState) {
	if ms.camp == nil {
		return
	}
	ms.camp, ms.campBack = nil, true
	log.Printf("Merc: %s's %q breaks camp", ms.owner.Name, ms.npc.Name)
}

// mercCampHoldsLocked reports a camp its owner is still at: in its world,
// within MercCampBreak of its spot.
func (w *World) mercCampHoldsLocked(ms *mercState) bool {
	c, o := ms.camp, ms.owner
	return c != nil && o.World == c.World && distXZ(o.X, o.Z, c.X, c.Z) <= MercCampBreak
}

// mercCampUpkeepLocked breaks a camp its owner has left (telling the owner),
// and ends a run back once the merc is within petCatchUp of its owner in its
// world. Must be called with the lock held.
func (w *World) mercCampUpkeepLocked(ms *mercState, sess *session.Session) {
	if ms.camp != nil && !w.mercCampHoldsLocked(ms) {
		w.breakMercCampLocked(ms)
		if sess != nil {
			w.sendChatToSession(sess, fmt.Sprintf(mercCampLeftLine, ms.npc.Name))
		}
	}
	n, o := ms.npc, ms.owner
	if ms.campBack && n.World == o.World && distXZ(n.X, n.Z, o.X, o.Z) <= petCatchUp {
		ms.campBack = false
	}
}

// mercLostLocked is how far from its owner the merc may be before it is put
// beside it: farther while it holds a camp or runs back from one.
func (w *World) mercLostLocked(ms *mercState) float32 {
	if ms.campBack || w.mercCampHoldsLocked(ms) {
		return MercCampBreak + petLost
	}
	return petLost
}

// mercCampHomeLocked is the merc at its camp out of a fight: walking to its
// place round what stands between, then facing the owner's way. Must be
// called with the lock held.
func (w *World) mercCampHomeLocked(ms *mercState, dt float32, now time.Time) {
	n, c := ms.npc, ms.camp
	if distXZ(n.X, n.Z, c.At[0], c.At[2]) <= 1 {
		n.stopMoving()
		n.Facing = turnToward(n.Facing, c.Face, npcTurnRate*dt)
		return
	}
	w.petWalkLocked(n, n.ServerID, c.At[0], c.At[2], c.At[0], c.At[1], c.At[2], max(ms.owner.SpeedNow(), 3), dt, now)
}

// mercGuardsLocked reports a place within r of what the merc looks after: its
// owner, or the camp it holds (in its world). Not camped, it is the owner's
// check alone.
func (w *World) mercGuardsLocked(ms *mercState, x, z, r float32) bool {
	o := ms.owner
	if distXZ(o.X, o.Z, x, z) <= r {
		return true
	}
	c := ms.camp
	return w.mercCampHoldsLocked(ms) && ms.npc.World == c.World && distXZ(c.X, c.Z, x, z) <= r
}

// mercAroundLocked is every entity within r of the merc's owner and, camped,
// of its camp's spot, each once. Not camped, it is the owner's query alone.
func (w *World) mercAroundLocked(ms *mercState, r float32) []EntityID {
	o := ms.owner
	ids := w.SpatialIndex.QueryRadius(o.X, o.Z, r)
	c := ms.camp
	if !w.mercCampHoldsLocked(ms) || ms.npc.World != c.World || (c.X == o.X && c.Z == o.Z) {
		return ids
	}
	seen := make(map[EntityID]bool, len(ids))
	for _, id := range ids {
		seen[id] = true
	}
	for _, id := range w.SpatialIndex.QueryRadius(c.X, c.Z, r) {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}
