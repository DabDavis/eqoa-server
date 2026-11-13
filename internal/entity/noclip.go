package entity

// noclip.go — a GM's /noclip (the GM Jibrish's petition 1904, 2026-11-13: "Add a
// high level GM only command (Just for jibrish and dabs account) that is /noclip -
// this lets the player move through walls vertical or horizontal and default turns
// on /fly. Make it a toggle, hitting it again turns it off").
//
// Noclip is a flight (fly.go) through everything: on, it turns the flight on with
// it; off, it turns the flight off; and a flight's end (#fly off, a new login)
// ends it, since a body that walks needs its floors. The server's own walls are
// the movement check's (movecheck.go), which judges none of a flyer's moves: so a
// GM in noclip is never moved back for a wall, a floor or a ceiling crossed. The
// walls the client meets are its own (vanilla's walker.rs collides even in
// flight): handlers/noclip.go tells a client that flies (protocol.OpcodeNoclip),
// and a client that knows the opcode passes through them; one that does not flies
// as #fly does. Who may is the handlers' (world.noclip_accounts).

import (
	"log"
	"time"

	"github.com/eqoa/server/internal/session"
)

// SetNoclip turns sess's character's noclip on or off, its flight with it; false
// with no character. Its client is the caller's to tell (handlers/noclip.go).
func (w *World) SetNoclip(sess *session.Session, on bool, now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return false
	}
	w.setFlyingLocked(p, on, now)
	p.Noclip = on
	log.Printf("Noclip: %s %s at (%.1f, %.1f, %.1f) world %d", p.Name, map[bool]string{true: "on", false: "off"}[on], p.X, p.Y, p.Z, p.World)
	return true
}

// Noclip is whether sess's character is in noclip; ok false with no character.
func (w *World) Noclip(sess *session.Session) (on, ok bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return false, false
	}
	return p.Noclip, true
}
