package entity

import "github.com/eqoa/server/internal/session"

// AddEffect puts an active effect after the session's player's others, for
// buffChannelSyncSystem to send on channel 0x43; false with no player or with
// MaxEffects already on it (the client keeps no more).
func (w *World) AddEffect(sess *session.Session, e BuffSlot) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || len(p.Buffs) >= MaxEffects {
		return false
	}
	p.Buffs = append(p.Buffs, e)
	return true
}

// ClearEffects takes every active effect off the session's player; the
// channel then goes back to its 4 bytes, a count of none.
func (w *World) ClearEffects(sess *session.Session) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if p := w.sessionPlayerLocked(sess); p != nil {
		p.Buffs = nil
	}
}

// PlayerEffects is a copy of the session's player's active effects.
func (w *World) PlayerEffects(sess *session.Session) []BuffSlot {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if p := w.sessionPlayerLocked(sess); p != nil {
		return append([]BuffSlot(nil), p.Buffs...)
	}
	return nil
}
