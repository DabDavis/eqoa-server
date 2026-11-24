package entity

import (
	"github.com/eqoa/server/internal/session"
)

// factionspend.go — a standing spent as a currency, the shops paid in faction
// (handlers/tokenshop.go, a row whose token is "faction:<name>"; Live petition
// 5575, a player, 2026-11-24: "The stroyteller Mercants should sale items for
// Storyteller faction"). The Storyteller's points were "redeemable for special
// rewards" (the players' guide of 2010, eqoa.forumotion.com t306): what a
// purchase costs comes off the standing, which never goes below the floor the
// shop names (0 for the Storyteller: the points earned). The Testing Heroes'
// vendor spends his faction the same way from FactionMin (heroes.go).

// FactionKnown is whether table factions has the faction (a standing with a
// faction it lacks would read as 0).
func (w *World) FactionKnown(faction string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.factions.ensure()
	_, ok := w.factions.byName[faction]
	return ok
}

// moveFactionLocked changes p's standing with a faction by delta, held within
// FactionMin..FactionMax: the standing before and after. Must be called with
// the lock held.
func (w *World) moveFactionLocked(p *Player, faction string, delta int) (old, now int) {
	old = w.ownStandingLocked(p, faction)
	now = clampFaction(old + delta)
	if now != old {
		if p.Factions == nil {
			p.Factions = make(map[string]int)
		}
		p.Factions[faction] = now
	}
	return old, now
}

// SpendFaction takes cost off sess's player's standing with a faction and
// tells it as a faction's change is told: the standing before and after.
// False, nothing taken, when that would go below floor, the faction is not
// one of the table's, or there is no player.
func (w *World) SpendFaction(sess *session.Session, faction string, floor, cost int) (before, after int, ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || cost <= 0 {
		return 0, 0, false
	}
	w.factions.ensure()
	if _, known := w.factions.byName[faction]; !known {
		return 0, 0, false
	}
	before = w.ownStandingLocked(p, faction)
	if before-cost < floor {
		return before, before, false
	}
	_, after = w.moveFactionLocked(p, faction, -cost)
	if !sess.FactionMessagesOff() {
		w.sendChatToSession(sess, factionLine(faction, -cost, after))
	}
	return before, after, true
}

// RefundFaction gives back what SpendFaction took (a purchase whose item could
// not be given after all), told the same way: the standing after.
func (w *World) RefundFaction(sess *session.Session, faction string, cost int) (after int, ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || cost <= 0 {
		return 0, false
	}
	_, after = w.moveFactionLocked(p, faction, cost)
	if !sess.FactionMessagesOff() {
		w.sendChatToSession(sess, factionLine(faction, cost, after))
	}
	return after, true
}
