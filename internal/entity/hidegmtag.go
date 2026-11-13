package entity

import "github.com/eqoa/server/internal/session"

// hidegmtag.go — a GM's [GM] tag hidden (Player.HideGMTag, /hide gmtag; the
// owner, 2026-11-13: "for gm accounts let them be able to hide GM tag from
// their chars"). Its name goes bare on the entity channel (ShownName), so
// every client draws it as a player's at the next pass; it is a GM as before.

// SetGMTagHidden hides (on) or shows the session's player's [GM] tag. It
// reports whether the player is a GM (only a GM has a tag); false for none.
func (w *World) SetGMTagHidden(sess *session.Session, on bool) (gm bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || !p.GM {
		return false
	}
	p.HideGMTag = on
	return true
}
