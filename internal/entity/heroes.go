package entity

import (
	"fmt"
	"log"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// heroes.go — the Testing Heroes, the World's half (handlers/heroes.go is the
// vendor's). Petition 1471, the GM Jibrish, 2026-11-10: "For all mobs that give
// the player XP, add a new faction "Testing Heroes". This starts at max
// negative for all players. Each kill that gives XP also grants +1 to this
// faction, and +5 for raid bosses. Add a vendor in freeport near the well and
// dabrish that allows you to buy items from him with that faction. The items
// should cost 25, 100, 250, 500 and 1000 faction and reduce their faction by
// that amount. If it would take the player below max negative, they do not
// have enough to buy the item."
//
// world.testing_heroes turns all of it on (SetTestingHeroes: off unless set;
// the Heirloom Test Server sets it). Off, nothing here does anything: no kill
// is credited, no line is said, the faction window leaves the faction out and
// the vendor has nothing to say.
//
//   - The faction is a row of table factions like any other ("Testing Heroes",
//     starting at FactionMin, -2000: tools/seed_testing_heroes.py writes it), so
//     a character's standing is kept, shown in its faction window, set by a
//     GM's #faction and edited in the panel as every faction's is. Without the
//     row nothing is credited and the vendor is closed (said once in the log):
//     a faction the table lacks would read as standing 0, 2000 to spend.
//   - The credit is the kill's experience's: every character a kill gives
//     experience to (awardKillXPLocked: the killer and its group's members
//     near, each whose share is above nothing; a creature too low for a member
//     gives it none, and so no credit) gains HeroesKill with the faction,
//     HeroesBossKill for a raid boss, once a kill each. A kill that gives no
//     experience credits no one: a guards' kill, a duel with Metal Moogie, a
//     GM's #kill.
//   - A raid boss is the raid loot's (handlers/raidloot.go, "RAID BOSS"): a
//     creature whose class template gives it RaidBossHPPct or more of its
//     level's hit points, whose loot categories' profile is "boss", or one of
//     whose loot tables is a "boss ..." one. The loot's two are read whole
//     (database.RaidBossNames) at the start and every minute, as the Ascended
//     records are: a kill is credited under the World's lock, which asks the
//     database nothing.
//   - Each change is told as a faction's is (factionLine: "-1999 faction with
//     Testing Heroes (+1)"), unless the client's Faction Messages are off, and
//     the standing stays within FactionMin..FactionMax.
//   - What a character may spend is its standing above the floor (HeroesBalance:
//     standing - FactionMin); a purchase takes its cost off the standing, and
//     is refused where that would go below the floor (HeroesSpend).

// HeroesFaction is the Testing Heroes' faction (table factions).
const HeroesFaction = database.HeroesFaction

// HeroesKill and HeroesBossKill are what a kill that gives experience is
// worth with the Testing Heroes: a creature's, a raid boss's (petition 1471).
const (
	HeroesKill     = 1
	HeroesBossKill = 5
)

// RaidBossHPPct is the class template hit points (of its level's, in percent)
// from which a creature is a raid boss: the raid loot's rule
// (handlers/raidloot.go; every tuned raid boss is 1,008% to 10,417%).
const RaidBossHPPct = 1000

// heroesState is the World's Testing Heroes: the switch, the loot's raid
// bosses by NPCClassKey, and whether the missing faction was said.
type heroesState struct {
	on     bool
	bosses map[string]bool
	warned bool
}

// SetTestingHeroes turns the Testing Heroes on or off (world.testing_heroes).
func (w *World) SetTestingHeroes(on bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.heroes.on = on
	w.heroes.warned = false
}

// TestingHeroes reports world.testing_heroes.
func (w *World) TestingHeroes() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.heroes.on
}

// SetRaidBossNames names the creatures the loot's tables make raid bosses
// (database.RaidBossNames): at the start and every minute after. It says how
// many names it keeps.
func (w *World) SetRaidBossNames(names []string) int {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		if k := database.NPCClassKey(n); k != "" {
			m[k] = true
		}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.heroes.bosses = m
	return len(m)
}

// LoadRaidBossNames reads the loot's raid bosses off the database
// (SetRaidBossNames). Call it without the database held.
func (w *World) LoadRaidBossNames(db *database.SQLiteDB) (int, error) {
	names, err := db.RaidBossNames()
	if err != nil {
		return 0, fmt.Errorf("the loot's raid bosses: %w", err)
	}
	return w.SetRaidBossNames(names), nil
}

// raidBossLocked is the raid loot's rule on npc (the file's doc). Must be
// called with the lock held (a read lock will do).
func (w *World) raidBossLocked(npc *NPC) bool {
	if npc == nil {
		return false
	}
	if npc.tmpl != nil && npc.tmpl.HPPct >= RaidBossHPPct {
		return true
	}
	return w.heroes.bosses[database.NPCClassKey(npc.Name)]
}

// IsRaidBoss reports the raid loot's rule on the NPC of that id.
func (w *World) IsRaidBoss(id EntityID) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.raidBossLocked(w.NPCs[id])
}

// heroesOpenLocked is whether the Testing Heroes are on and their faction is
// one of the table's. Must be called with the lock held.
func (w *World) heroesOpenLocked() bool {
	if !w.heroes.on {
		return false
	}
	w.factions.ensure()
	if _, ok := w.factions.byName[HeroesFaction]; !ok {
		if !w.heroes.warned {
			w.heroes.warned = true
			log.Printf("Heroes: world.testing_heroes is on and table factions has no %q: nothing is credited and the vendor is closed (tools/seed_testing_heroes.py writes it)",
				HeroesFaction)
		}
		return false
	}
	return true
}

// HeroesOpen reports whether the Testing Heroes are on and their faction is
// one of the table's.
func (w *World) HeroesOpen() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.heroesOpenLocked()
}

// heroesCreditLocked credits p with the Testing Heroes for a kill of npc that
// gave it experience (awardKillXPLocked), and tells it (sess nil: no one to
// tell). Must be called with the lock held.
func (w *World) heroesCreditLocked(sess *session.Session, p *Player, npc *NPC) {
	if p == nil || npc == nil || !w.heroesOpenLocked() {
		return
	}
	delta := HeroesKill
	if w.raidBossLocked(npc) {
		delta = HeroesBossKill
	}
	old, now := w.heroesMoveLocked(p, delta)
	log.Printf("Heroes: %s's kill of %q (level %d): %s %d -> %d (%+d)", p.Name, npc.Name, npc.Level, HeroesFaction, old, now, delta)
	if sess != nil && !sess.FactionMessagesOff() {
		w.sendChatToSession(sess, factionLine(HeroesFaction, delta, now))
	}
}

// heroesMoveLocked changes p's standing with the Testing Heroes by delta, held
// within FactionMin..FactionMax: the standing before and after. Must be called
// with the lock held.
func (w *World) heroesMoveLocked(p *Player, delta int) (old, now int) {
	old = w.standingLocked(p, HeroesFaction)
	now = clampFaction(old + delta)
	if now != old {
		if p.Factions == nil {
			p.Factions = make(map[string]int)
		}
		p.Factions[HeroesFaction] = now
	}
	return old, now
}

// HeroesBalance is what sess's player may spend with the Testing Heroes, its
// standing above the floor, and the standing itself; false with no player in
// the world, or with the Testing Heroes off or without their faction.
func (w *World) HeroesBalance(sess *session.Session) (balance, standing int, ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || !w.heroesOpenLocked() {
		return 0, 0, false
	}
	standing = w.standingLocked(p, HeroesFaction)
	return standing - FactionMin, standing, true
}

// HeroesSpend takes cost off sess's player's standing with the Testing Heroes
// and tells it as a faction's change is told: the standing before and after.
// False, nothing taken, when that would go below the floor (FactionMin), with
// no player, or with the Testing Heroes closed.
func (w *World) HeroesSpend(sess *session.Session, cost int) (before, after int, ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || cost <= 0 || !w.heroesOpenLocked() {
		return 0, 0, false
	}
	before = w.standingLocked(p, HeroesFaction)
	if before-cost < FactionMin {
		return before, before, false
	}
	_, after = w.heroesMoveLocked(p, -cost)
	if !sess.FactionMessagesOff() {
		w.sendChatToSession(sess, factionLine(HeroesFaction, -cost, after))
	}
	return before, after, true
}

// HeroesRefund gives back what HeroesSpend took (a purchase whose item could
// not be given after all), told the same way: the standing after.
func (w *World) HeroesRefund(sess *session.Session, cost int) (after int, ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || cost <= 0 {
		return 0, false
	}
	_, after = w.heroesMoveLocked(p, cost)
	if !sess.FactionMessagesOff() {
		w.sendChatToSession(sess, factionLine(HeroesFaction, cost, after))
	}
	return after, true
}
