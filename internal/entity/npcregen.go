package entity

import (
	"fmt"
	"log"
	"time"
)

// Creatures heal fast once a fight is over (F3 20261025-215107, jib: "When
// mobs leash / deagro they should begin regenerating HP rapidly, 15% per
// tick. The regen shouldn't kick in for 2 ticks out of combat."). Until then
// a creature never healed: one a fight left at a sliver (its enemy dead, gone
// or past its leash) stayed at a sliver until something killed it.
//
// On the regen's six-second clock (regen.go's RegenInterval, the players'
// own), an NPC out of a fight counts the ticks: the first npcCalmTicks give
// nothing, and every one after heals its level's regen_pct of its maximum
// (Balance by level; npcCalmHealPct, jib's 15, with no rows), at least 1,
// until it is whole. Any threat on it (addThreatLocked: a hit, a spell, its
// own notice of a player, a friend's call for help) starts the count again,
// so one fought again between two ticks waits its two ticks anew. Out of a
// fight is no enemy: on its way home after its leash it heals already. Not a
// dummy (dummy.go) or a corpse. The two calm ticks are jib's account of the
// live game, not measured.
//
// The user, 2026-10-29: "pets will have HoT, PoT per tick when charmed or a
// summoned pet. Mobs regen slower". So a pet, summoned or charmed, heals and
// gains power every tick, in a fight or out of one, as a player does
// (regen.go): a 50 of its maximum hit points (power), at least 1 (petRegen);
// its power is 28 a level (CalculateMaxPower with no stats: the pet has
// none), given at its summoning or charming (petPowerLocked). No pet spends
// power yet (the necromancer pet's tap is on its own clock, pettap.go); its
// seed hit points from level 15 are seven tenths of the first tuning's so the
// pet classes win by the con as before (database/balanceseed.go). And
// the seed rows make a creature out of a fight heal 5% a tick, a third of
// jib's 15 (claude-notes/balance-20261029.md: a creature left empty is whole
// in about 2 min 10 s, not 50 s).

// npcCalmTicks is how many of the regen's ticks out of a fight give a
// creature nothing.
const npcCalmTicks = 2

// npcCalmHealPct is the share of its maximum hit points a creature out of a
// fight heals each tick after npcCalmTicks with no balance rows (jib's; the
// rows' regen_pct otherwise).
const npcCalmHealPct = 15

// npcRegenSystem is the creatures' healing out of a fight (the file's doc).
// Every RegenInterval; must be called with the lock held.
func (w *World) npcRegenSystem(now time.Time) {
	if w.TickCount%uint64(RegenInterval/(100*time.Millisecond)) != 0 {
		return
	}
	for _, npc := range w.NPCs {
		if npc.Dead || npc.Dummy != NotDummy {
			continue
		}
		if npc.Owner != 0 {
			npc.petRegen() // a pet, as a player (the file's doc)
			continue
		}
		if npc.CurrentEnemy != 0 {
			npc.calmTicks = 0
			continue
		}
		if npc.calmTicks <= npcCalmTicks {
			npc.calmTicks++
		}
		if npc.calmTicks <= npcCalmTicks || npc.HP >= npc.MaxHP {
			continue
		}
		was := npc.HP
		npc.HP = min(npc.HP+max(1, int(float64(npc.MaxHP)*w.balance.RegenPct(npc.Level)/100)), npc.MaxHP)
		if !npc.healing { // its first heal since its fight: one line, and one when whole
			log.Printf("Regen: %s heals out of its fight (%d -> %d of %d)", npc.Name, was, npc.HP, npc.MaxHP)
		}
		npc.healing = npc.HP < npc.MaxHP
		if !npc.healing {
			log.Printf("Regen: %s is whole again", npc.Name)
		}
	}
}

// petRegen is a pet's tick: its HoT and PoT as a player's base (regen.go),
// a 50 of each maximum, at least 1, within the maximum.
func (n *NPC) petRegen() {
	if n.HP > 0 && n.HP < n.MaxHP {
		n.HP = min(n.HP+max(1, n.MaxHP/50), n.MaxHP)
	}
	if n.MaxPower > 0 && n.Power < n.MaxPower {
		n.Power = min(n.Power+max(1, n.MaxPower/50), n.MaxPower)
	}
}

// petPowerLocked gives a new pet (summoned or charmed) its power, full: 28 a
// level (the file's doc).
func petPowerLocked(pet *NPC) {
	pet.MaxPower = CalculateMaxPower(pet.Level, classWarrior, 0, 0, 0, 0, 0, 0, 0)
	pet.Power = pet.MaxPower
}

// NPCRegenSummary is a line for the server's start: the rate of a creature
// of level 1 and of 60 (the balance's regen_pct, by level).
func (w *World) NPCRegenSummary() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return fmt.Sprintf("out of a fight a creature heals %.3g%% (level 1) to %.3g%% (60) of its hit points every %v from the %s tick on; "+
		"a pet heals and gains power every tick as a player does", w.balance.RegenPct(1), w.balance.RegenPct(60), RegenInterval, ordinal(npcCalmTicks+1))
}

func ordinal(n int) string {
	switch n {
	case 1:
		return "first"
	case 2:
		return "second"
	case 3:
		return "third"
	}
	return fmt.Sprintf("%dth", n)
}
