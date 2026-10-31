package entity

import (
	"log"
	"time"

	"github.com/eqoa/server/internal/session"
)

// linkdead.go — a character that quits in a fight stays in it, and one that
// leaves keeps its hit points and power (the user, 2026-10-31: "i just
// logged out with jedd and i fully healed and mobs did not kill me"; their
// rule, chosen that day: "HP and power are saved at logout and restored at
// login. A character that leaves while creatures are fighting it stays in
// the world, link-dead, until they stop (it dies, they leash, or about 60 s
// pass). A death there is a normal death: debt, and home to the bind
// point"). No record of the live game says what SOE's servers did.
//
// What it left with comes back at its next login with what its HoT and PoT
// give back over the time away: it rested (the user's word), so one away
// long enough comes back whole.
//
// A client that drops already left its character in the world until its
// session timed out, the creatures fighting on (takeover.go); CLIENT's Quit
// (0x09B0, which disconnects at once) ended the session at the next tick and
// every creature's threat on it with it, and the next login filled the hit
// points: a quit was a way out of any fight. Now a Quit while creatures
// fight the character (FightingSession: their threat on it, or it their
// enemy) leaves it in the world, its own swing stopped, and the session ends
// once none fights it any more, or QuitLinger after the Quit; a death waits
// for its respawn first, so it goes home to its bind point, its debt paid as
// any death's. A login meanwhile takes the character over as it stands.

// QuitLinger is the longest a character that quit in a fight stays in it.
const QuitLinger = 60 * time.Second

// fightingLocked is how many creatures fight p: their threat on it, or it
// their enemy (a training dummy's fight is not one). Must be called with the
// lock held.
func (w *World) fightingLocked(p *Player) int {
	n := 0
	for _, npc := range w.NPCs {
		if npc.Dead || npc.Dummy != NotDummy {
			continue
		}
		if _, threat := npc.Threats[p.ServerID]; threat || npc.CurrentEnemy == p.ServerID {
			n++
		}
	}
	return n
}

// QuitInFight is a client's Quit (0x09B0): when creatures fight its
// character, it stays in the world, its own swing stopped, and QuitInFight
// reports how many fight it; 0 when none does (the session may end now).
func (w *World) QuitInFight(sess *session.Session) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || w.Players[p.ServerID] != p {
		return 0
	}
	n := w.fightingLocked(p)
	if n > 0 {
		p.AutoAttack = false
		p.disengage()
		log.Printf("Quit: %s quit with %d creatures fighting it: it stays in the world until they stop (%v at most)",
			p.Name, n, QuitLinger)
	}
	return n
}

// QuitStayOver reports a stay that is over for a session whose client quit
// in a fight at since: its character gone from it, or none fighting it any
// more, or QuitLinger past; never while its character is dead and its
// respawn to come (the bind point is where it then leaves the world).
func (w *World) QuitStayOver(sess *session.Session, since, now time.Time) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || w.Players[p.ServerID] != p {
		return true
	}
	if p.respawnPending() {
		return false
	}
	return now.Sub(since) >= QuitLinger || w.fightingLocked(p) == 0
}

// Vitals is the hit points and power the session's player leaves the world
// with, for the next login; keep is false for one dead or on its way to a
// respawn, which comes back whole. ok is false when the session's player is
// not the world's (another login took it, and keeps it).
func (w *World) Vitals(sess *session.Session) (hp, power int, keep, ok bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || w.Players[p.ServerID] != p {
		return 0, 0, false, false
	}
	if p.respawnPending() || p.IsDead() {
		return 0, 0, false, true
	}
	return p.HP, p.Power, true, true
}

// RestoreVitals gives the session's player the hit points and power it left
// the world with, and what its HoT and PoT give back over the time it was
// away (a tick of each every RegenInterval, regen.go: the user, "if player
// stays logged out for past their HoT/PoT would regen to full then they come
// back full health/power of course. they essentially rested"), within its
// maxima now (its gear and effects on): at a login, after its gear. It
// reports what it set.
func (w *World) RestoreVitals(sess *session.Session, hp, power int, away time.Duration) (int, int, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return 0, 0, false
	}
	top, topPower := p.MaxHPNow(), p.MaxPowerNow()
	if ticks := int64(max(away, 0) / RegenInterval); ticks > 0 {
		hp = int(min(int64(hp)+ticks*int64(p.HoTNow()), int64(top)))
		power = int(min(int64(power)+ticks*int64(p.PoTNow()), int64(topPower)))
	}
	p.HP = max(1, min(hp, top))
	p.Power = max(0, min(power, topPower))
	p.StatsDirty = true
	return p.HP, p.Power, true
}
