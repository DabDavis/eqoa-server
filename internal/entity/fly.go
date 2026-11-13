package entity

// fly.go — a GM's flight (#fly, handlers/fly.go; the user, 2026-11-07: "lets add
// a gm fly command its like swimming you can look up/down to go in that
// direction, but running, standing like normal and its a on/off toggle").
//
// The flight itself is the client's walk (vanilla's walker.rs: CLIENT's swim
// path with no gravity). What the server has to do is leave the player alone:
//
//   - the movement check (movecheck.go) judges none of its moves and logs none
//     of them: a body held up by nothing, rising, is what it is for;
//   - its 0x40's walker flags are the place's own, so nothing here takes a flyer
//     for a swimmer (breath runs down only under real water, as anyone's) and it
//     is not standing unless it touches a floor. A logout saves where it last
//     stood (SavePlace), so no login begins in mid-air, and flight ends there: a
//     new login is a new Player, or takes the live one over without its flight
//     (handedOverLocked);
//   - a creature's floor and a pet's are their own (groundNPCLocked,
//     petStepLocked: the terrain under the owner's height at the highest), but a
//     creature's reach is measured across the ground (chaseLocked), which would
//     have one stand under a flyer and hit it 100 up (flownOutOfReach);
//   - a restart that keeps the players keeps the flight (restoreFlyingLocked).

import (
	"log"
	"math"
	"time"

	"github.com/eqoa/server/internal/session"
)

// flyOffGrace is how long after #fly off the check goes on leaving the player
// alone: the client hears a moment later and is still in the air, with its
// velocity to lose, for its first reports after it. Then it judges as after a
// server move: from wherever the player is, the fall from there.
const flyOffGrace = 2 * time.Second

// flyingNow is whether the check leaves the player's moves alone at now.
func (p *Player) flyingNow(now time.Time) bool {
	return p.Flying || now.Before(p.flyGrace)
}

// flownOutOfReach is whether a creature's enemy is flown up or down out of its
// reach, though it stands beside it on the map. Everyone else is on the
// creature's floor or near it, and the chase asks only how far across.
func flownOutOfReach(npc *NPC, p *Player) bool {
	return p.Flying && float32(math.Abs(float64(p.Y-npc.Y))) > npcReach(npc)
}

// SetFlying turns sess's character's flight on or off; false with no character.
// Its client is the caller's to tell (handlers/fly.go).
func (w *World) SetFlying(sess *session.Session, on bool, now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return false
	}
	w.setFlyingLocked(p, on, now)
	return true
}

// setFlyingLocked is SetFlying on p. Must be called with the lock held.
func (w *World) setFlyingLocked(p *Player, on bool, now time.Time) {
	if on == p.Flying {
		return
	}
	p.Flying = on
	if !on {
		p.Noclip = false // no flight, no noclip: a body that walks needs its floors (noclip.go)
	}
	p.mv.init = false
	p.flyGrace = time.Time{}
	if !on {
		p.flyGrace = now.Add(flyOffGrace)
	}
	log.Printf("Fly: %s %s at (%.1f, %.1f, %.1f) world %d", p.Name, map[bool]string{true: "takes off", false: "stops flying"}[on], p.X, p.Y, p.Z, p.World)
}

// Flying is whether sess's character flies; ok false with no character.
func (w *World) Flying(sess *session.Session) (on, ok bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return false, false
	}
	return p.Flying, true
}

// restoreFlyingLocked gives a character built anew from its row, after a
// restart, its flight, and the place it flew at: a shutdown's save writes where
// a body in the air last stood (SavePlace), and a flyer hovering sends no 0x40 to
// correct it, so its dump would have put it back there. The row keeps that place
// (a login never begins in mid-air); StoodX and the rest stay the row's. Must be
// called with the lock held.
func (w *World) restoreFlyingLocked(p *Player, k KeptPlayer) {
	p.Flying = k.Flying
	p.Noclip = k.Flying && k.Noclip // noclip.go
	if !k.Flying || k.World != p.World {
		return
	}
	oldX, oldZ := p.X, p.Z
	p.X, p.Y, p.Z = k.Place[0], k.Place[1], k.Place[2]
	if w.SpatialIndex != nil {
		w.SpatialIndex.Update(p.ServerID, oldX, oldZ, p.X, p.Z)
	}
}
