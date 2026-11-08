package entity

import (
	"log"

	"github.com/eqoa/server/internal/session"
)

// moogie.go — Mr. Moogie (petition 1246, Dwarfrish; the user: build it): an NPC who clears a
// player's buffs or heals it whole, for anyone not flagged for PvP and not in a duel (handlers'
// moogie.go asks; this does). "Clear my buffs" ends every effect on the player as a GM's
// #clearbuffs ends them (ClearBuffs), but the town's Boon (boon.go): a blessing for playing, not a
// spell's buff. "Fully heal me" brings its hit points and power to their most.

// MoogieName is his name: an NPC of it is Mr. Moogie.
const MoogieName = "Mr. Moogie"

// Why Mr. Moogie will not help (ours).
const (
	MoogieLinePvP  = "Mr. Moogie will not help you while you are flagged for PvP."
	MoogieLineDuel = "Mr. Moogie will not help you while you are in a duel."
	MoogieLineDead = "Mr. Moogie cannot help you now."
)

// MoogieBusy is why sess's player may not have Mr. Moogie's help now ("" when it may).
func (w *World) MoogieBusy(sess *session.Session) string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	switch {
	case p == nil || p.IsDead():
		return MoogieLineDead
	case p.pvp.on:
		return MoogieLinePvP
	case w.duelOnLocked(p.ServerID) != nil:
		return MoogieLineDuel
	}
	return ""
}

// MoogieClear ends every effect on sess's player but the Boon, each as its time running out ends it
// (removeEffectLocked). It returns the player's name and how many it ended; ok false with no player.
func (w *World) MoogieClear(sess *session.Session) (name string, n int, ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return "", 0, false
	}
	for _, ae := range append([]*ActiveEffect(nil), p.Effects...) {
		if ae.SpellID == BoonEffectID {
			continue
		}
		w.removeEffectLocked(p, ae.SpellID)
		n++
	}
	p.StatsDirty = true
	log.Printf("Moogie: %s's buffs cleared (%d effects)", p.Name, n)
	return p.Name, n, true
}

// MoogieHeal brings sess's player's hit points and power to their most. It returns the player's
// name; ok false with no player.
func (w *World) MoogieHeal(sess *session.Session) (name string, ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || p.IsDead() {
		return "", false
	}
	p.HP, p.Power = p.MaxHPNow(), p.MaxPowerNow()
	p.StatsDirty = true
	log.Printf("Moogie: %s healed whole (%d HP, %d power)", p.Name, p.HP, p.Power)
	return p.Name, true
}
