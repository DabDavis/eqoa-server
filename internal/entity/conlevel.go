// conlevel.go — what /con says of a player's target (handlers/conlevel.go,
// world.con_level; Live petition 5130, approved by the owner 2026-11-15).

package entity

import (
	"strings"

	"github.com/eqoa/server/internal/session"
)

// Considered is sess's target as /con tells it: its name as players see it (an
// NPC's source '$' off, as mob info has it), its level (the one its con colour
// and the entity channel's byte 140 read) and that colour to sess's player. ok
// is false with no target, or one no longer in sess's view.
func (w *World) Considered(sess *session.Session) (name string, level int, con byte, ok bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || p.TargetObjectID == 0 || p.TargetObjectID == NoAssist {
		return "", 0, 0, false
	}
	e, found := w.findEntityByObjectIDLocked(sess, p.TargetObjectID)
	if !found {
		return "", 0, 0, false
	}
	switch t := e.(type) {
	case *Player:
		name, level = t.Name, t.Level
	case *NPC:
		name, level = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(t.Name), "$")), t.Level
	default:
		return "", 0, 0, false
	}
	return name, level, ConColor(p.Level, level), true
}
