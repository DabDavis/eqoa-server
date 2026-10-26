package entity

import (
	"fmt"
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
// nothing, and every one after heals npcCalmHealPct of its maximum, at least
// 1, until it is whole. Any threat on it (addThreatLocked: a hit, a spell,
// its own notice of a player, a friend's call for help) starts the count
// again, so one fought again between two ticks waits its two ticks anew. Out
// of a fight is no enemy: on its way home after its leash it heals already.
// Not a pet (pet.go keeps its own), a charmed creature, a dummy (dummy.go), a
// corpse. The numbers are jib's account of the live game, not measured.

// npcCalmTicks is how many of the regen's ticks out of a fight give a
// creature nothing.
const npcCalmTicks = 2

// npcCalmHealPct is the share of its maximum hit points a creature out of a
// fight heals each tick after npcCalmTicks.
const npcCalmHealPct = 15

// npcRegenSystem is the creatures' healing out of a fight (the file's doc).
// Every RegenInterval; must be called with the lock held.
func (w *World) npcRegenSystem(now time.Time) {
	if w.TickCount%uint64(RegenInterval/(100*time.Millisecond)) != 0 {
		return
	}
	for _, npc := range w.NPCs {
		if npc.Dead || npc.Owner != 0 || npc.Dummy != NotDummy {
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
		npc.HP = min(npc.HP+max(1, npc.MaxHP*npcCalmHealPct/100), npc.MaxHP)
	}
}

// NPCRegenSummary is a line for the server's start.
func NPCRegenSummary() string {
	return fmt.Sprintf("out of a fight a creature heals %d%% of its hit points every %v from the %s tick on",
		npcCalmHealPct, RegenInterval, ordinal(npcCalmTicks+1))
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
