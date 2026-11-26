package entity

import "github.com/eqoa/server/internal/session"

// OwnsCMAwake is the first of ids (class mastery ranks, cmcatalog ids) the
// session's player owns with its masteries awake, and whether it owns one: a
// Relive's masteries sleep (marks.go), as refreshCMLocked counts none of their
// effects. An ability's older form (database.LegacyForm, handlers/cast.go)
// asks it at each cast, so a rank bought or removed counts at the next one.
func (w *World) OwnsCMAwake(sess *session.Session, ids []int) (int, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || p.Relive != nil {
		return 0, false
	}
	for _, id := range ids {
		if _, ok := p.CMs[id]; ok {
			return id, true
		}
	}
	return 0, false
}
