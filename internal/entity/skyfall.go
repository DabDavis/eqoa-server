package entity

import (
	"log"
	"time"

	"github.com/eqoa/server/internal/session"
)

// skyfall.go — a fall off the Plane of Sky's islands ends at home (Hoofbite's
// petition 1995, 2026-11-13: "When jumping off plane of sky island, you do not
// return home and continue to fall forever. Should return home after about 15
// seconds of falling from a plane of sky island").
//
// The islands float over nothing. CLIENT's walk drops a body with no floor to
// its world floor, -1000 (WorldFloorY), and leaves it standing there in the
// air for good; vanilla's walk falls on past it (Hoofbite's own report: y
// -12,350). Nothing brought either back but /stuck.
//
// A player in the Plane of Sky is LOST while its place (its 0x40s, as the
// server took them) is below the islands (skyBottom: under the lowest ground
// of the world's terrain), or over the void: the world's ground known
// (internal/ground: its terrain and what is placed on it, a bridge's deck
// too) and no floor under it at all, down to below the islands. Lost for
// skyFallAfter, unbroken, it is sent home as /stuck and Return Home send a
// player (the gate hook: where a spiritmaster bound it, else where every
// login starts), told why. A report that stands, swims or has a floor under
// it ends the count; a jump from an island's edge onto another below is never
// lost long. A GM's flight (#fly), a dead player and a world being loaded are
// left alone; the count runs on with no report (a client parked at -1000 may
// say nothing more), by the tick (skyFallSystem).

// skyWorld is the Plane of Sky's world index (CLIENT's gWorldInfo: PLANESKY).
const skyWorld = 4

// skyBottom is the height under which a player in the Plane of Sky is below
// every island: the lowest of its terrain, -304.2 (the ground bake of the
// Frontiers disc's PLANESKY, both its zones: y -304.2 to 883.6), less 50.
const skyBottom = float32(-354)

// skyFallAfter is how long a player is lost before it is sent home (the
// petition's "about 15 seconds of falling").
const skyFallAfter = 15 * time.Second

// skyFallLine is what the player is told as it is sent home (ours).
const skyFallLine = "You fall through the endless sky, and the winds carry you home."

// skyLostLocked is whether the player is lost in the Plane of Sky (the file's
// doc). Must be called with the lock held.
func (w *World) skyLostLocked(p *Player, now time.Time) bool {
	if p.World != skyWorld || p.IsDead() || p.respawnPending() || p.flyingNow(now) {
		return false
	}
	if p.Y < skyBottom {
		return true
	}
	mg, ok := w.ground.(MoveGround)
	if !ok || !mg.Knows(skyWorld) {
		return false
	}
	_, held := mg.Support(skyWorld, p.X, p.Y, p.Z, mcFloorAbove, p.Y-skyBottom)
	return !held
}

// skyFallStepLocked counts a report of the player's toward its fall, or ends
// the count. Must be called with the lock held.
func (w *World) skyFallStepLocked(p *Player, now time.Time) {
	if !w.skyLostLocked(p, now) {
		p.skyFallSince = time.Time{}
		return
	}
	if p.skyFallSince.IsZero() {
		p.skyFallSince = now
		log.Printf("Sky: %s falls at (%.1f, %.1f, %.1f): home in %v unless it lands", p.Name, p.X, p.Y, p.Z, skyFallAfter)
	}
}

// skyFallSystem sends home every player lost in the Plane of Sky for
// skyFallAfter, returning them for the gate hook (runGates). Must be called
// with the lock held.
func (w *World) skyFallSystem(now time.Time) []respawnDue {
	var due []respawnDue
	for _, p := range w.Players {
		if p.skyFallSince.IsZero() {
			continue
		}
		if !w.skyLostLocked(p, now) {
			p.skyFallSince = time.Time{}
			continue
		}
		if now.Sub(p.skyFallSince) < skyFallAfter {
			continue
		}
		var sess *session.Session
		if w.SessionManager != nil {
			sess = w.findSessionByEntityIDLocked(p.ServerID)
		}
		if sess == nil || sess.StateChannelsHeld() {
			continue // gone, or loading a world
		}
		p.skyFallSince = time.Time{}
		log.Printf("Sky: %s lost %v at (%.1f, %.1f, %.1f): sent home", p.Name, skyFallAfter, p.X, p.Y, p.Z)
		w.sendColoredLocked(sess, skyFallLine, [3]byte{255, 255, 0})
		due = append(due, respawnDue{sess: sess, p: p})
	}
	return due
}
