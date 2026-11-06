package entity

import (
	"log"
	"time"

	"github.com/eqoa/server/internal/session"
)

// LandOnSelf lands a spell on the session's player as if it had cast the spell on itself, with no
// casting, power or lockout: its visual, then its effects, as a beneficial spell's (a town's
// board's gift, handlers/boardinvis.go). It reports whether it landed (false: no living player).
func (w *World) LandOnSelf(sess *session.Session, spell CastSpell, now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || p.IsDead() {
		return false
	}
	w.broadcastCastLocked(p, p.ServerID, 0, spell.visual())
	w.landOnPlayerLocked(&castState{spell: spell, caster: p.ServerID, targets: []EntityID{p.ServerID}}, p, sess, p, now)
	log.Printf("Cast: %s is given %q", p.Name, spell.Name)
	return true
}
