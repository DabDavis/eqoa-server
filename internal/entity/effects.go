package entity

import (
	"fmt"
	"log"

	"github.com/eqoa/server/internal/session"
)

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
// channel then goes back to its 4 bytes, a count of none. It is the channel's
// slots alone (#effect clear): the spells' effects behind them go on, their
// stat changes too, until they end (ClearBuffs ends them).
func (w *World) ClearEffects(sess *session.Session) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if p := w.sessionPlayerLocked(sess); p != nil {
		p.Buffs = nil
	}
}

// ClearBuffs ends every effect on the session's player now (a GM's
// #clearbuffs: F3 20261029-011146, jib's "a GM Test command to clear all
// buffs on the character issuing it"), each as its time running out ends it
// (removeEffectLocked, as effectSystem calls it): its slot off the effects
// channel, the stat changes summed again with the hit points and power kept
// within the maximums they leave, the stat and effects channels sent on the
// next tick (StatsDirty), the run speed told (0x00F8) when it changes, and
// with the effect whatever it held: invisibility, an illusion's body, a
// ceiling on the maximum power, a heal or power over time (no last tick: its
// time is not up), a damage shield, a proc. What lasts on a player is a
// spell's (anyone's, a potion's too), a meal's or a drink's: all go, the
// harmful (curePlayerLocked's kind) with the rest, for a clean slate to
// test from: no creature casts here and no harmful spell lands on a player,
// so the only harm one carries is the price of its own spells and potions
// (a pact's stat cost, a buff's downside, the Eye line's power ceiling).
// The slots #effect put on, which no effect backs, go too. A
// logout after it keeps none (SavedEffects). It returns the names of the
// effects taken off in their order ("spell N" for one with no name), how
// many slots #effect had put on, and false when the session has no player
// in the world.
func (w *World) ClearBuffs(sess *session.Session) (names []string, slots int, ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return nil, 0, false
	}
	for len(p.Effects) > 0 {
		ae := p.Effects[0]
		w.removeEffectLocked(p, ae.SpellID)
		name := ae.Name
		if name == "" {
			name = fmt.Sprintf("spell %d", ae.SpellID)
		}
		names = append(names, name)
		log.Printf("Cast: %q taken off %s (#clearbuffs)", ae.Name, p.Name)
	}
	if slots = len(p.Buffs); slots > 0 {
		p.Buffs = nil
		p.StatsDirty = true
	}
	return names, slots, true
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
