package entity

import (
	"cmp"
	"fmt"
	"log"
	"slices"
	"time"
)

// monkeythrow.go — the monkeys throw snowballs (the owner, 2026-11-16: "make
// [a player]'s monkeys throw snowballs at players").
//
// Each monkey (monkeys.go) throws now and then: at its own time, a random
// monkeyThrowMin to monkeyThrowMax after it came out or last tried, it throws
// a snowball at a player picked at random among those it may hit, and tries
// again at its next time whether it threw or not. It may hit another player
// (never its owner, never an NPC) alive and in its owner's world and instance,
// within SnowballRange of it and in its sight (inSightLocked: no wall
// between), not in a fight (monkeyFightingLocked), not invisible, not a GM
// hidden by Toggle Invis, not loading a world (noticeableSeenLocked), and not hit by any of its owner's monkeys within
// monkeyThrowGap (all five together throw at one player once in that time at
// most). Nothing is thrown while its owner fights.
//
// A throw is the snowball's (moogieitems.go): SnowballFX, Frost Shot's
// projectile, from the monkey to the player for all who see it, and 1 off the
// player's hit points, never below 1 (the snowball's rule), a -1 over the
// player told to the player alone; the player is told "Bongo hits you with a
// snowball!" and the owner "Bongo hits Zed with a snowball!". Nothing else:
// no PvP flag, no fight, no hate, no hit on any list (combatrecord, a duel),
// and nothing answers it.

const (
	// monkeyThrowMin, monkeyThrowMax: a monkey's time between two tries
	monkeyThrowMin = 20 * time.Second
	monkeyThrowMax = 40 * time.Second
	// monkeyThrowGap is the least time between two snowballs at one player
	// from one owner's monkeys
	monkeyThrowGap = 30 * time.Second
	// monkeyFightWindow is how long after a blow a player counts as fighting
	// (fishing's window and more)
	monkeyFightWindow = 10 * time.Second
	// monkeyThrowLine is the owner's line
	monkeyThrowLine = "%s hits %s with a snowball!"
)

// monkeyNextThrow is a monkey's next try after now: monkeyThrowMin to
// monkeyThrowMax on.
func monkeyNextThrow(now time.Time) time.Time {
	span := int(monkeyThrowMax-monkeyThrowMin) / int(time.Millisecond)
	return now.Add(monkeyThrowMin + time.Duration(monkeyPick(span+1))*time.Millisecond)
}

// monkeyFightingLocked reports p in a fight: its auto-attack on, a blow given
// or taken within monkeyFightWindow, flagged for PvP, in a duel or Mr.
// Moogie's challenge. Must be called with the lock held.
func (w *World) monkeyFightingLocked(p *Player, now time.Time) bool {
	return p.Attacking() || !p.LastCombatTime.IsZero() && now.Sub(p.LastCombatTime) < monkeyFightWindow || w.ownFightLocked(p)
}

// monkeyThrowsLocked is each of owner's monkeys whose time has come trying a
// throw (the file's doc). Must be called with the lock held.
func (w *World) monkeyThrowsLocked(owner *Player, out []*monkeyState, now time.Time) {
	for _, ms := range out {
		if ms.nextThrow.IsZero() {
			ms.nextThrow = monkeyNextThrow(now)
		}
		if now.Before(ms.nextThrow) {
			continue
		}
		ms.nextThrow = monkeyNextThrow(now)
		if w.monkeyFightingLocked(owner, now) || owner.IsDead() {
			continue
		}
		if t := w.monkeyTargetLocked(owner, ms.npc, now); t != nil {
			w.monkeyThrowLocked(owner, ms.npc, t, now)
		}
	}
}

// monkeyTargetLocked is a player at random a monkey n of owner's may hit now
// (the file's doc), nil for none. Must be called with the lock held.
func (w *World) monkeyTargetLocked(owner *Player, n *NPC, now time.Time) *Player {
	thrown := w.monkeyThrown[owner.ServerID]
	var can []*Player
	for id, t := range w.Players {
		if t == owner || t.IsDead() || t.respawnPending() || t.GMHidden || t.Invisible() || !SamePlace(owner, t) {
			continue
		}
		if at, ok := thrown[id]; ok && now.Sub(at) < monkeyThrowGap {
			continue
		}
		if dist3(n.X, n.Y, n.Z, t.X, t.Y, t.Z) > SnowballRange || w.monkeyFightingLocked(t, now) {
			continue
		}
		if !w.noticeableSeenLocked(t) { // loading a world, or one creatures leave alone (aggro.go)
			continue
		}
		if !w.inSightLocked(n.World, n.X, n.Y, n.Z, t.X, t.Y, t.Z) {
			continue
		}
		can = append(can, t)
	}
	if len(can) == 0 {
		return nil
	}
	// w.Players is a map: its order is no order, so pick off a sorted list
	slices.SortFunc(can, func(a, b *Player) int { return cmp.Compare(a.ServerID, b.ServerID) })
	return can[monkeyPick(len(can))]
}

// monkeyThrowLocked is monkey n's snowball landing on t (the file's doc). Must
// be called with the lock held.
func (w *World) monkeyThrowLocked(owner *Player, n *NPC, t *Player, now time.Time) {
	if w.monkeyThrown == nil {
		w.monkeyThrown = make(map[EntityID]map[EntityID]time.Time)
	}
	thrown := w.monkeyThrown[owner.ServerID]
	if thrown == nil {
		thrown = make(map[EntityID]time.Time)
		w.monkeyThrown[owner.ServerID] = thrown
	}
	for id, at := range thrown {
		if now.Sub(at) >= monkeyThrowGap {
			delete(thrown, id) // spent
		}
	}
	thrown[t.ServerID] = now
	n.Facing = headingTo(t.X-n.X, t.Z-n.Z)
	w.broadcastNPCCastLocked(n, t.ServerID, 0, SnowballFX)
	took := 0
	if t.HP > 1 {
		took = 1
		t.HP--
		t.StatsDirty = true
	}
	if ts := w.findSessionByEntityIDLocked(t.ServerID); ts != nil {
		if cid := w.objectIDSeenByLocked(ts, n.ServerID); cid != 0 {
			w.sendDamageIndication(ts, cid, -took, ts.PlayerObjectID(), 0)
		}
		w.sendChatToSession(ts, fmt.Sprintf(snowballHitLine, n.Name))
	}
	if os := w.findSessionByEntityIDLocked(owner.ServerID); os != nil {
		w.sendChatToSession(os, fmt.Sprintf(monkeyThrowLine, n.Name, t.Name))
	}
	log.Printf("Monkeys: %s's %q hits %s with a snowball (HP %d/%d, %d taken)", owner.Name, n.Name, t.Name, t.HP, t.MaxHPNow(), took)
}
