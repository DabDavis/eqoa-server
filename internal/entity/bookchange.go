package entity

import (
	"log"
	"time"

	"github.com/eqoa/server/internal/session"
)

// bookchange.go — the World's part in a change of a player's spell book that
// takes spells out and puts others in at once: the Spell Archivist's path
// change (handlers/archivistpaths.go; the GM Jibrish's petition 1325, "if
// they pick the other swap it"). The book itself is the handler's (the spells
// table and the client's 0x00B8 and 0x00B3); what the World holds of those
// spells is here:
//
//   - a lockout (cast.go's castLockout) is the server's own, by spell: one of
//     a spell taken out keeps running, so a spell taken out and given back is
//     not cast again before its time (two changes in a row would otherwise
//     make every recast ready), but it is shown on no ability any more (its
//     display index -1: CLIENT's Msg00CD lands on the first ability of the
//     index it is sent, which may be another spell's by then); one of a spell
//     put in is shown where the spell now is (0x00CD with what is left);
//   - an effect the player put on itself with a spell taken out ends now, as
//     its time running out ends it (removeEffectLocked, #clearbuffs' way): the
//     path it was of is not the player's any more. One another put on it
//     (another cleric's ward) is not of its book and runs on.
//
// A cast on its way is the handler's to wait for (Casting): it would land
// after the change and put back what the change ended.

// Casting reports whether sess's player has a cast on its way: begun and not
// landed (castSystem clears it at the landing).
func (w *World) Casting(sess *session.Session) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	return p != nil && p.Casting != nil
}

// BookChanged is a change of sess's player's book (the notes at the top): the
// spells gone (their ids) and the spells come, each at its display index now.
// It returns the names of the effects ended, in the order they were on the
// player, and how many lockouts it showed again.
func (w *World) BookChanged(sess *session.Session, gone []int, came map[int]int, now time.Time) (ended []string, shown int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return nil, 0
	}
	out := make(map[int]bool, len(gone))
	for _, id := range gone {
		out[id] = true
	}
	for i := range p.Lockouts {
		l := &p.Lockouts[i]
		if out[l.spellID] {
			l.displayIndex = -1
		}
		if index, ok := came[l.spellID]; ok {
			// over or not, its end is said where the spell is now
			// (castSystem), never on what took its old place
			l.displayIndex = index
			if now.Before(l.until) {
				sendCooldown(sess, index, l.until.Sub(now))
				shown++
			}
		}
	}
	for i := 0; i < len(p.Effects); {
		e := p.Effects[i]
		if !out[e.SpellID] || e.Caster != p.ServerID {
			i++
			continue
		}
		// one effect a spell (addEffectLocked replaces a recast's), so this
		// takes p.Effects[i] out and the next is at i
		w.removeEffectLocked(p, e.SpellID)
		ended = append(ended, effectLabel(e))
		log.Printf("Cast: %q taken off %s (its spell left the book)", e.Name, p.Name)
	}
	return ended, shown
}
