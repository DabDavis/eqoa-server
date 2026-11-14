package entity

import (
	"log"
	"time"

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
	MoogieLineFoe  = "Mr. Moogie will not help you while you fight him."
)

// MoogieBusy is why sess's player may not have Mr. Moogie's help now ("" when it may): he, moogie,
// will not help one he fights (the GM Jibrish's petition 1251 made him attackable), one flagged for
// PvP or one in a duel.
func (w *World) MoogieBusy(sess *session.Session, moogie EntityID) string {
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
	case w.metalDuelOfLocked(p.ServerID):
		return MetalLineDueling // metalmoogie.go
	}
	if npc := w.NPCs[moogie]; npc != nil {
		if _, foe := npc.Threats[p.ServerID]; foe || npc.CurrentEnemy == p.ServerID {
			return MoogieLineFoe
		}
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
		if outlivesDeath(ae.SpellID) || ae.Rules.Kept { // the Boon, a GM's reward, his own Duelist's Favor (metalmoogie.go)
			continue
		}
		w.removeEffectLocked(p, ae.SpellID)
		n++
	}
	p.StatsDirty = true
	log.Printf("Moogie: %s's buffs cleared (%d effects)", p.Name, n)
	return p.Name, n, true
}

// MoogieResetID is the lockout of Mr. Moogie's reset itself (the GM Jibrish's petition 1444: "allow the
// player to reset all of their ability cool downs ... can only be used on a player once every 10
// minutes"): a lockout of no spell (an id no ability has), shown on no ability, so it is kept as every
// lockout is over a logout (charlockouts: lockoutsave.go) and a restart that keeps the sessions (keep.go),
// on the wall clock; MoogieResetEvery is how often a character may have it.
const (
	MoogieResetID    = -1444
	MoogieResetEvery = 10 * time.Minute
)

// MoogieReset ends every lockout of sess's player's abilities still running (cast.go: each spell's and
// art's, and the arts' shared cooldown) as their time running out ends them: each one's badge told 0x00CD
// with 0, as castSystem tells a lockout over. Not its pet's, nor anyone else's; not its items' cooldowns.
// Once in MoogieResetEvery a character (its own lockout, MoogieResetID): within it nothing is ended and
// wait is what is left of it. It returns the player's name and how many lockouts it ended; ok false
// with no living player, or within its ten minutes. The PvP flag and a duel are MoogieBusy's, asked
// first.
func (w *World) MoogieReset(sess *session.Session, now time.Time) (name string, n int, wait time.Duration, ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || p.IsDead() {
		return "", 0, 0, false
	}
	for _, l := range p.Lockouts {
		if l.spellID == MoogieResetID && now.Before(l.until) {
			return p.Name, 0, l.until.Sub(now), false
		}
	}
	for _, l := range p.Lockouts {
		if l.spellID == MoogieResetID {
			continue // its own, over: another below
		}
		if now.Before(l.until) {
			n++
		}
		sendCooldown(sess, l.displayIndex, 0) // a badge of no ability (-1) is sent nothing
	}
	if now.Before(p.artReady) {
		n++
	}
	p.artReady = time.Time{}
	p.Lockouts = append(p.Lockouts[:0], castLockout{spellID: MoogieResetID, displayIndex: -1, until: now.Add(MoogieResetEvery)})
	log.Printf("Moogie: %s's ability lockouts reset (%d running); again from %s", p.Name, n, now.Add(MoogieResetEvery).UTC().Format("15:04:05"))
	return p.Name, n, 0, true
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
