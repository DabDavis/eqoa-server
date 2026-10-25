package entity

import (
	"time"

	"github.com/eqoa/server/internal/session"
)

// playvisual.go — a spell visual played by hand: a GM's /fx and the GM
// panel's play_visual, to see what a visual (0x00B1's spell effect DictID,
// a 0xC200 record of SPELLFX.CSF) looks like before a spell is given it.

// MaxVisualCast is the longest cast time a visual is played with by hand.
const MaxVisualCast = 30 * time.Second

// PlayVisual plays a spell visual now, from sess's player onto its target
// (itself when it has none, or one it no longer sees), as a cast's 0x00B1
// begins one: to the player and to everyone who sees it, castTime the cast
// the visual's casting part waits for (0 to MaxVisualCast). Nothing else of
// a cast happens: no power, no lockout, nothing lands. It reports whom it was
// played on ("" and false when sess has no player in the world).
func (w *World) PlayVisual(sess *session.Session, fx uint32, castTime time.Duration) (string, bool) {
	castTime = max(0, min(castTime, MaxVisualCast))
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || !sess.CharacterInWorld() {
		return "", false
	}
	target, name := p.ServerID, p.Name
	if id := p.TargetObjectID; id != 0 && id != 0xFFFFFFFF && id != sess.PlayerObjectID() {
		if e, ok := w.findEntityByObjectIDLocked(sess, id); ok {
			target = e.GetEntityID()
			switch t := e.(type) {
			case *Player:
				name = t.Name
			case *NPC:
				name = t.Name
			}
		}
	}
	w.broadcastCastLocked(p, target, castTime, fx)
	return name, true
}
